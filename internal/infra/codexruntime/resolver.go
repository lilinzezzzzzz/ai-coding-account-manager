package codexruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultValidateTimeout = 2 * time.Second

// Source 表示 Codex runtime 的发现来源。
type Source string

const (
	// SourceConfig 表示 runtime 来自显式配置。
	SourceConfig Source = "config"
	// SourcePath 表示 runtime 来自 PATH。
	SourcePath Source = "path"
	// SourceDesktopApp 表示 runtime 来自桌面应用内置 CLI。
	SourceDesktopApp Source = "desktop_app"
)

// Runtime 描述一个已校验可用的 Codex runtime。
type Runtime struct {
	Path   string
	Source Source
}

// Validator 校验候选 runtime 是否具备所需能力。
type Validator interface {
	Validate(context.Context, string) error
}

// Config 保存 Resolver 配置。
type Config struct {
	ConfiguredBin string
	HomeDir       string
	// ApplicationDirs 覆盖 macOS 应用目录；nil 使用系统和用户 Applications，空切片禁用桌面发现。
	ApplicationDirs []string
	Validator       Validator
	ValidateTimeout time.Duration
}

// Resolver 按固定顺序发现 Codex runtime。
type Resolver struct {
	configuredBin   string
	applicationDirs []string
	validator       Validator
	validateTimeout time.Duration
}

// NewResolver 创建 Codex runtime resolver。
func NewResolver(cfg Config) *Resolver {
	validator := cfg.Validator
	if validator == nil {
		validator = execValidator{}
	}
	validateTimeout := cfg.ValidateTimeout
	if validateTimeout <= 0 {
		validateTimeout = defaultValidateTimeout
	}
	applicationDirs := cfg.ApplicationDirs
	if applicationDirs == nil && runtime.GOOS == "darwin" {
		applicationDirs = []string{"/Applications"}
		homeDir := cfg.HomeDir
		if homeDir == "" {
			homeDir, _ = os.UserHomeDir()
		}
		if homeDir != "" {
			applicationDirs = append(applicationDirs, filepath.Join(homeDir, "Applications"))
		}
	}
	return &Resolver{
		configuredBin:   strings.TrimSpace(cfg.ConfiguredBin),
		applicationDirs: append([]string(nil), applicationDirs...),
		validator:       validator,
		validateTimeout: validateTimeout,
	}
}

// Resolve 返回第一个可用 Codex runtime。
func (resolver *Resolver) Resolve(ctx context.Context) (Runtime, error) {
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	candidates := resolver.candidates()
	var failures []error
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return Runtime{}, err
		}
		validateCtx, cancel := context.WithTimeout(ctx, resolver.validateTimeout)
		err := resolver.validator.Validate(validateCtx, candidate.Path)
		cancel()
		if err == nil {
			slog.InfoContext(ctx, "codex runtime selected", "source", candidate.Source, "path", candidate.Path)
			return candidate, nil
		}
		slog.WarnContext(ctx, "codex runtime validation failed", "source", candidate.Source, "path", candidate.Path, "error", err)
		if ctx.Err() != nil {
			return Runtime{}, ctx.Err()
		}
		failures = append(failures, fmt.Errorf("%s:%s: %w", candidate.Source, candidate.Path, err))
	}
	if len(failures) == 0 {
		return Runtime{}, fmt.Errorf("codex runtime not found")
	}
	return Runtime{}, fmt.Errorf("codex runtime candidates failed validation: %w", errors.Join(failures...))
}

func (resolver *Resolver) candidates() []Runtime {
	var candidates []Runtime
	seen := map[string]bool{}
	add := func(path string, source Source) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		absPath, err := filepath.Abs(path)
		if err == nil {
			path = absPath
		}
		if seen[path] {
			return
		}
		seen[path] = true
		candidates = append(candidates, Runtime{Path: path, Source: source})
	}

	add(resolver.configuredBin, SourceConfig)
	for _, path := range resolver.desktopAppCandidates() {
		add(path, SourceDesktopApp)
	}
	if path, err := exec.LookPath("codex"); err == nil {
		add(path, SourcePath)
	}
	return candidates
}

func (resolver *Resolver) desktopAppCandidates() []string {
	// 只检查已知应用内的固定位置，不递归扫描应用或编辑器扩展目录。
	relativePaths := []string{
		filepath.Join("Codex.app", "Contents", "Resources", "codex"),
		filepath.Join("ChatGPT.app", "Contents", "Resources", "codex-cli", "CodexCLI.app", "Contents", "MacOS", "codex"),
	}
	var candidates []string
	for _, root := range resolver.applicationDirs {
		for _, relativePath := range relativePaths {
			path := filepath.Join(root, relativePath)
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				// 权限等错误留给 validator，确保记录失败原因。
				candidates = append(candidates, path)
			}
		}
	}
	return candidates
}

type execValidator struct{}

func (execValidator) Validate(ctx context.Context, path string) error {
	checks := [][]string{
		{"--version"},
		{"login", "--help"},
		{"app-server", "--help"},
	}
	for _, args := range checks {
		cmd := exec.CommandContext(ctx, path, args...)
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return fmt.Errorf("validate %s %s: %w", path, strings.Join(args, " "), err)
		}
	}
	return nil
}
