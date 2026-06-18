package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// Client 是 Codex app-server JSON-RPC client。
type Client struct {
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	lines       *bufio.Scanner
	cleanupDirs []string
	mu          sync.Mutex
	nextID      atomic.Int64
}

// Config 保存 app-server client 启动参数。
type Config struct {
	Bin       string
	CodexHome string
	Env       []string
}

// Start 启动 Codex app-server 并完成初始化握手。
func Start(ctx context.Context, cfg Config) (*Client, error) {
	bin := cfg.Bin
	if bin == "" {
		bin = "codex"
	}
	cmd := exec.CommandContext(ctx, bin, "app-server", "--stdio")
	var cleanupDirs []string
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}
	if cfg.CodexHome != "" {
		if len(cmd.Env) == 0 {
			cmd.Env = cmd.Environ()
		}
		cmd.Env = append(cmd.Env, "CODEX_HOME="+cfg.CodexHome)
		if !hasEnvKey(cmd.Env, "CODEX_SQLITE_HOME") {
			sqliteHome, err := os.MkdirTemp("", "ai-coding-account-manager-codex-sqlite-*")
			if err != nil {
				return nil, fmt.Errorf("create app-server sqlite home: %w", err)
			}
			cmd.Env = append(cmd.Env, "CODEX_SQLITE_HOME="+sqliteHome)
			cleanupDirs = append(cleanupDirs, sqliteHome)
			defer func() {
				if cmd.Process == nil {
					_ = os.RemoveAll(sqliteHome)
				}
			}()
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open app-server stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open app-server stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start app-server: %w", err)
	}
	go discard(stderr)

	client := &Client{
		cmd:         cmd,
		stdin:       stdin,
		lines:       bufio.NewScanner(stdout),
		cleanupDirs: cleanupDirs,
	}
	client.lines.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	if err := client.initialize(ctx); err != nil {
		_ = client.Close(context.Background())
		return nil, err
	}
	return client, nil
}

// Call 发送 JSON-RPC 请求并返回 result。
func (client *Client) Call(ctx context.Context, method string, params any, result any) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	id := client.nextID.Add(1)
	request := rpcRequest{
		Method: method,
		ID:     id,
		Params: params,
	}
	if err := client.write(request); err != nil {
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var message rpcMessage
		if err := client.read(&message); err != nil {
			return err
		}
		if message.ID == nil {
			continue
		}
		if message.Method != "" {
			_ = client.write(rpcErrorResponse{
				ID: message.ID,
				Error: rpcError{
					Code:    -32601,
					Message: "server requests are not supported",
				},
			})
			continue
		}
		if *message.ID != id {
			continue
		}
		if message.Error != nil {
			return fmt.Errorf("app-server %s: %s", method, message.Error.Message)
		}
		if result == nil {
			return nil
		}
		if len(message.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(message.Result, result); err != nil {
			return fmt.Errorf("decode app-server %s result: %w", method, err)
		}
		return nil
	}
}

// Notify 发送 JSON-RPC notification。
func (client *Client) Notify(method string, params any) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	return client.write(rpcNotification{
		Method: method,
		Params: params,
	})
}

// Close 关闭 app-server 子进程。
func (client *Client) Close(context.Context) error {
	if client == nil {
		return nil
	}
	if client.stdin != nil {
		_ = client.stdin.Close()
	}
	if client.cmd == nil || client.cmd.Process == nil {
		return nil
	}
	defer func() {
		for _, dir := range client.cleanupDirs {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := client.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	_ = client.cmd.Wait()
	return nil
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func (client *Client) initialize(ctx context.Context) error {
	var response initializeResponse
	err := client.Call(ctx, "initialize", initializeParams{
		ClientInfo: clientInfo{
			Name:    "ai_coding_account_manager",
			Title:   "AI Coding Account Manager",
			Version: "0.1.0",
		},
		Capabilities: initializeCapabilities{
			ExperimentalAPI: true,
		},
	}, &response)
	if err != nil {
		return fmt.Errorf("initialize app-server: %w", err)
	}
	if err := client.Notify("initialized", map[string]any{}); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}
	return nil
}

func (client *Client) write(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode app-server message: %w", err)
	}
	if _, err := client.stdin.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write app-server message: %w", err)
	}
	return nil
}

func (client *Client) read(value any) error {
	if !client.lines.Scan() {
		if err := client.lines.Err(); err != nil {
			return fmt.Errorf("read app-server message: %w", err)
		}
		return io.EOF
	}
	if err := json.Unmarshal(client.lines.Bytes(), value); err != nil {
		return fmt.Errorf("decode app-server message: %w", err)
	}
	return nil
}

func discard(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}

type rpcRequest struct {
	Method string `json:"method"`
	ID     int64  `json:"id"`
	Params any    `json:"params"`
}

type rpcNotification struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type rpcMessage struct {
	ID     *int64           `json:"id"`
	Method string           `json:"method"`
	Result json.RawMessage  `json:"result"`
	Error  *rpcError        `json:"error"`
	Params *json.RawMessage `json:"params"`
}

type rpcErrorResponse struct {
	ID    *int64   `json:"id"`
	Error rpcError `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeParams struct {
	ClientInfo   clientInfo             `json:"clientInfo"`
	Capabilities initializeCapabilities `json:"capabilities"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type initializeCapabilities struct {
	ExperimentalAPI bool `json:"experimentalApi"`
}

type initializeResponse struct {
	UserAgent string `json:"userAgent"`
}
