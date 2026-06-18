package codexruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	// SourceVSCodeExtension 表示 runtime 来自 VS Code 扩展目录。
	SourceVSCodeExtension Source = "vscode_extension"
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
	ConfiguredBin   string
	HomeDir         string
	Validator       Validator
	ValidateTimeout time.Duration
}

// Resolver 按固定顺序发现 Codex runtime。
type Resolver struct {
	configuredBin   string
	homeDir         string
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
	return &Resolver{
		configuredBin:   strings.TrimSpace(cfg.ConfiguredBin),
		homeDir:         cfg.HomeDir,
		validator:       validator,
		validateTimeout: validateTimeout,
	}
}

// Resolve 返回第一个可用 Codex runtime。
func (resolver *Resolver) Resolve(ctx context.Context) (Runtime, error) {
	candidates := resolver.candidates()
	var failures []string
	for _, candidate := range candidates {
		validateCtx, cancel := context.WithTimeout(ctx, resolver.validateTimeout)
		err := resolver.validator.Validate(validateCtx, candidate.Path)
		cancel()
		if err == nil {
			return candidate, nil
		}
		failures = append(failures, fmt.Sprintf("%s:%s", candidate.Source, candidate.Path))
	}
	if len(failures) == 0 {
		return Runtime{}, fmt.Errorf("codex runtime not found")
	}
	return Runtime{}, fmt.Errorf("codex runtime candidates failed validation: %s", strings.Join(failures, ", "))
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
	if path, err := exec.LookPath("codex"); err == nil {
		add(path, SourcePath)
	}
	for _, path := range resolver.vscodeExtensionCandidates() {
		add(path, SourceVSCodeExtension)
	}
	return candidates
}

func (resolver *Resolver) vscodeExtensionCandidates() []string {
	homeDir := resolver.homeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil
		}
	}
	roots := []string{
		filepath.Join(homeDir, ".vscode", "extensions"),
		filepath.Join(homeDir, ".vscode-server", "extensions"),
	}
	var candidates []string
	for _, root := range roots {
		candidates = append(candidates, findVSCodeCodexExecutables(root)...)
	}
	sort.Strings(candidates)
	return candidates
}

func findVSCodeCodexExecutables(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.Contains(name, "codex") && !strings.Contains(name, "openai") && !strings.Contains(name, "chatgpt") {
			continue
		}
		extensionDir := filepath.Join(root, entry.Name())
		_ = filepath.WalkDir(extensionDir, func(path string, dirEntry os.DirEntry, walkErr error) error {
			if walkErr != nil || dirEntry.IsDir() {
				return nil
			}
			base := strings.ToLower(dirEntry.Name())
			if base != "codex" && base != "codex.exe" {
				return nil
			}
			if isExecutable(path) {
				candidates = append(candidates, path)
			}
			return nil
		})
	}
	return candidates
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
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
			return fmt.Errorf("validate %s %s: %w", path, strings.Join(args, " "), err)
		}
	}
	return nil
}
