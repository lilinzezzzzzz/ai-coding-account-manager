package loginrunner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const codexConfig = `cli_auth_credentials_store = "file"
`

var (
	urlPattern      = regexp.MustCompile(`https?://[^\s]+`)
	userCodePattern = regexp.MustCompile(`\b[A-Z0-9]{4}(?:-[A-Z0-9]{4}){1,3}\b`)
)

// Mode 表示 Codex 登录方式。
type Mode string

const (
	// ModeBrowser 表示使用浏览器登录。
	ModeBrowser Mode = "browser"
	// ModeDeviceCode 表示使用 device code 登录。
	ModeDeviceCode Mode = "device_code"
)

// Input 保存一次登录运行的输入。
type Input struct {
	RuntimePath string
	CodexHome   string
	Mode        Mode
	OnProgress  func(Progress)
}

// Progress 表示从登录进程输出中提取出的可展示进度。
type Progress struct {
	LoginURL *string
	UserCode *string
}

// Result 表示 Codex 登录进程完成结果。
type Result struct {
	CodexHome string
}

// Runner 执行 Codex 登录进程。
type Runner struct{}

// Run 在隔离 CODEX_HOME 中运行 Codex 登录。
func (runner Runner) Run(ctx context.Context, input Input) (Result, error) {
	if input.RuntimePath == "" {
		return Result{}, fmt.Errorf("codex runtime path is required")
	}
	if input.CodexHome == "" {
		return Result{}, fmt.Errorf("codex home is required")
	}
	if err := os.MkdirAll(input.CodexHome, 0o700); err != nil {
		return Result{}, fmt.Errorf("create login codex home: %w", err)
	}
	configPath := filepath.Join(input.CodexHome, "config.toml")
	if err := os.WriteFile(configPath, []byte(codexConfig), 0o600); err != nil {
		return Result{}, fmt.Errorf("write login codex config: %w", err)
	}

	args := []string{"login"}
	if input.Mode == ModeDeviceCode {
		args = append(args, "--device-auth")
	}
	cmd := exec.CommandContext(ctx, input.RuntimePath, args...)
	cmd.Env = append(os.Environ(),
		"CODEX_HOME="+input.CodexHome,
		"CODEX_SQLITE_HOME="+input.CodexHome,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open codex login stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open codex login stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start codex login: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go scanOutput(stdout, input.OnProgress, &wg)
	go scanOutput(stderr, input.OnProgress, &wg)

	waitErr := cmd.Wait()
	wg.Wait()
	if waitErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		return Result{}, fmt.Errorf("codex login failed: %w", waitErr)
	}
	return Result{CodexHome: input.CodexHome}, nil
}

func scanOutput(reader io.Reader, onProgress func(Progress), wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		progress := progressFromLine(scanner.Text())
		if onProgress != nil && (progress.LoginURL != nil || progress.UserCode != nil) {
			onProgress(progress)
		}
	}
}

func progressFromLine(line string) Progress {
	var progress Progress
	if match := urlPattern.FindString(line); match != "" {
		cleaned := strings.TrimRight(match, ".,);]")
		progress.LoginURL = &cleaned
	}
	if match := userCodePattern.FindString(line); match != "" {
		progress.UserCode = &match
	}
	return progress
}
