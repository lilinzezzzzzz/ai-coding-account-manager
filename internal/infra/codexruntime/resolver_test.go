package codexruntime

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolverSelectsRuntimeInOrder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured bool
		pathCLI    bool
		wantSource Source
	}{
		{"explicit configuration wins", true, true, SourceConfig},
		{"desktop wins over standalone CLI", false, true, SourceDesktopApp},
		{"desktop works without CLI on PATH", false, false, SourceDesktopApp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			apps := filepath.Join(root, "Applications")
			desktop := writeTestCLI(t, filepath.Join(apps, "ChatGPT.app", "Contents", "Resources", "codex-cli", "CodexCLI.app", "Contents", "MacOS", "codex"))
			want := desktop
			t.Setenv("PATH", "")
			if tc.pathCLI {
				standalone := writeTestCLI(t, filepath.Join(root, "bin", "codex"))
				t.Setenv("PATH", filepath.Dir(standalone))
			}
			configured := ""
			if tc.configured {
				configured = writeTestCLI(t, filepath.Join(root, "configured-codex"))
				want = configured
			}
			resolver := NewResolver(Config{ConfiguredBin: configured, ApplicationDirs: []string{apps}, Validator: validatorFunc(func(context.Context, string) error { return nil })})
			got, err := resolver.Resolve(context.Background())
			if err != nil || got.Path != want || got.Source != tc.wantSource {
				t.Fatalf("runtime = %+v, error = %v, want %s from %s", got, err, want, tc.wantSource)
			}
		})
	}
}

func TestResolverFallsBackToStandaloneCLI(t *testing.T) {
	for _, scenario := range []string{"desktop missing", "desktop validation failed"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			standalone := writeTestCLI(t, filepath.Join(root, "bin", "codex"))
			t.Setenv("PATH", filepath.Dir(standalone))
			apps := filepath.Join(root, "Applications")
			if scenario == "desktop validation failed" {
				desktop := writeTestCLI(t, filepath.Join(apps, "Codex.app", "Contents", "Resources", "codex"))
				if err := os.WriteFile(desktop, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			resolver := NewResolver(Config{ApplicationDirs: []string{apps}})
			got, err := resolver.Resolve(context.Background())
			if err != nil || got.Path != standalone || got.Source != SourcePath {
				t.Fatalf("standalone fallback = %+v, error = %v", got, err)
			}
		})
	}
}

func TestResolverFindsDesktopApplicationLayouts(t *testing.T) {
	t.Setenv("PATH", "")
	for _, relative := range []string{
		"Codex.app/Contents/Resources/codex",
		"ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex",
	} {
		t.Run(relative, func(t *testing.T) {
			root := t.TempDir()
			userApps := filepath.Join(root, "user", "Applications")
			want := writeTestCLI(t, filepath.Join(userApps, filepath.FromSlash(relative)))
			resolver := NewResolver(Config{ApplicationDirs: []string{filepath.Join(root, "system", "Applications"), userApps}})
			got, err := resolver.Resolve(context.Background())
			if err != nil || got.Path != want || got.Source != SourceDesktopApp {
				t.Fatalf("runtime = %+v, error = %v", got, err)
			}
		})
	}
}

func TestResolverDefaultApplicationDirectories(t *testing.T) {
	home := t.TempDir()
	resolver := NewResolver(Config{HomeDir: home})
	if runtime.GOOS == "darwin" {
		if len(resolver.applicationDirs) != 2 || resolver.applicationDirs[0] != "/Applications" || resolver.applicationDirs[1] != filepath.Join(home, "Applications") {
			t.Fatalf("default application dirs = %v", resolver.applicationDirs)
		}
	} else if len(resolver.applicationDirs) != 0 {
		t.Fatalf("unexpected macOS application dirs: %v", resolver.applicationDirs)
	}
}

func TestResolverLogsFailureAndSelectedFallback(t *testing.T) {
	t.Setenv("PATH", "")
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	root := t.TempDir()
	broken := filepath.Join(root, "removed-codex")
	apps := filepath.Join(root, "Applications")
	fallback := writeTestCLI(t, filepath.Join(apps, "Codex.app", "Contents", "Resources", "codex"))
	resolver := NewResolver(Config{ConfiguredBin: broken, ApplicationDirs: []string{apps}})
	got, err := resolver.Resolve(context.Background())
	if err != nil || got.Path != fallback {
		t.Fatalf("fallback = %+v, error = %v", got, err)
	}
	for _, want := range []string{"codex runtime validation failed", broken, "--version", "no such file or directory", "codex runtime selected", fallback, "desktop_app"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs missing %q: %s", want, logs.String())
		}
	}
}

func TestResolverPreservesValidationFailure(t *testing.T) {
	t.Setenv("PATH", "")
	cause := errors.New("app-server check failed")
	resolver := NewResolver(Config{ConfiguredBin: filepath.Join(t.TempDir(), "broken-codex"), ApplicationDirs: []string{}, Validator: validatorFunc(func(context.Context, string) error { return cause })})
	_, err := resolver.Resolve(context.Background())
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "app-server check failed") {
		t.Fatalf("validation cause lost: %v", err)
	}
}

func TestResolverCancellationStopsDiscovery(t *testing.T) {
	t.Setenv("PATH", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	apps := t.TempDir()
	writeTestCLI(t, filepath.Join(apps, "Codex.app", "Contents", "Resources", "codex"))
	calls := 0
	resolver := NewResolver(Config{ConfiguredBin: filepath.Join(t.TempDir(), "configured"), ApplicationDirs: []string{apps}, Validator: validatorFunc(func(context.Context, string) error {
		calls++
		cancel()
		return ctx.Err()
	})})
	if _, err := resolver.Resolve(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled discovery: calls=%d error=%v", calls, err)
	}
	if _, err := resolver.Resolve(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("already cancelled discovery: calls=%d error=%v", calls, err)
	}
}

func TestResolverCandidateTimeoutAllowsFallback(t *testing.T) {
	t.Setenv("PATH", "")
	apps := t.TempDir()
	fallback := writeTestCLI(t, filepath.Join(apps, "Codex.app", "Contents", "Resources", "codex"))
	resolver := NewResolver(Config{ConfiguredBin: filepath.Join(t.TempDir(), "slow-codex"), ApplicationDirs: []string{apps}, ValidateTimeout: 10 * time.Millisecond, Validator: validatorFunc(func(ctx context.Context, path string) error {
		if path == fallback {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	})})
	got, err := resolver.Resolve(context.Background())
	if err != nil || got.Path != fallback {
		t.Fatalf("timeout fallback = %+v, %v", got, err)
	}
}

func TestResolverIgnoresEditorExtensions(t *testing.T) {
	t.Setenv("PATH", "")
	root := t.TempDir()
	for _, editor := range []string{".vscode", ".vscode-server", ".cursor"} {
		writeTestCLI(t, filepath.Join(root, editor, "extensions", "openai.codex-test", "bin", "codex"))
	}
	resolver := NewResolver(Config{HomeDir: root, ApplicationDirs: []string{filepath.Join(root, "Applications")}})
	if _, err := resolver.Resolve(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime not found") {
		t.Fatalf("editor extension must not be selected: %v", err)
	}
}

func TestExecValidatorReportsFailedCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex")
	// 只有 app-server 能力校验失败，输出内容不应进入错误或日志。
	if err := os.WriteFile(path, []byte("#!/bin/sh\ncase \"$1\" in app-server) echo private-output >&2; exit 7;; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := (execValidator{}).Validate(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "app-server --help") || !strings.Contains(err.Error(), "exit status 7") || strings.Contains(err.Error(), "private-output") {
		t.Fatalf("unexpected validation error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (execValidator{}).Validate(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

type validatorFunc func(context.Context, string) error

func (validate validatorFunc) Validate(ctx context.Context, path string) error {
	return validate(ctx, path)
}

func writeTestCLI(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
