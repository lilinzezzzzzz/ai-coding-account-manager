package codexruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolverUsesConfiguredRuntimeFirst(t *testing.T) {
	wantPath := filepath.Join(t.TempDir(), "codex")
	validator := &stubValidator{}
	resolver := NewResolver(Config{
		ConfiguredBin: wantPath,
		Validator:     validator,
	})

	runtime, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if runtime.Path != wantPath || runtime.Source != SourceConfig {
		t.Fatalf("runtime = %+v, want configured path", runtime)
	}
}

func TestResolverFindsVSCodeExtensionRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	homeDir := t.TempDir()
	runtimePath := filepath.Join(homeDir, ".vscode", "extensions", "openai.codex-test", "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	resolver := NewResolver(Config{
		HomeDir:   homeDir,
		Validator: &stubValidator{},
	})
	runtime, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if runtime.Path != runtimePath || runtime.Source != SourceVSCodeExtension {
		t.Fatalf("runtime = %+v, want VS Code extension runtime", runtime)
	}
}

func TestResolverIgnoresCursorExtensionRuntime(t *testing.T) {
	t.Setenv("PATH", "")
	homeDir := t.TempDir()
	runtimePath := filepath.Join(homeDir, ".cursor", "extensions", "openai.codex-test", "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(runtimePath), 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	if err := os.WriteFile(runtimePath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	resolver := NewResolver(Config{
		HomeDir:   homeDir,
		Validator: &stubValidator{},
	})
	_, err := resolver.Resolve(context.Background())
	if err == nil {
		t.Fatal("Resolve() error = nil, want error")
	}
}

type stubValidator struct{}

func (stub *stubValidator) Validate(context.Context, string) error {
	return nil
}
