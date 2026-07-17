package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientCallCorrelatesResponsesAndIgnoresNotifications(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, Config{
		Bin: executable,
		Env: []string{"AICAM_FAKE_APPSERVER=1"},
	})
	if err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() {
		if err := client.Close(context.Background()); err != nil {
			t.Fatalf("close client: %v", err)
		}
	}()

	var response struct {
		Value string `json:"value"`
	}
	if err := client.Call(ctx, "test/echo", map[string]string{"value": "ok"}, &response); err != nil {
		t.Fatalf("call echo: %v", err)
	}
	if response.Value != "ok" {
		t.Fatalf("response value = %q, want ok", response.Value)
	}
}

func TestClientReturnsRPCError(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, Config{
		Bin: executable,
		Env: []string{"AICAM_FAKE_APPSERVER=1"},
	})
	if err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	err = client.Call(ctx, "test/error", nil, nil)
	if err == nil {
		t.Fatal("call error succeeded, want error")
	}
}

func TestClientExtractsUpstreamErrorFromRPCError(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, Config{
		Bin: executable,
		Env: []string{"AICAM_FAKE_APPSERVER=1"},
	})
	if err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	err = client.Call(ctx, "test/upstream-error", nil, nil)
	if err == nil {
		t.Fatal("call error succeeded, want error")
	}
	upstream, ok := UpstreamErrorFrom(err)
	if !ok {
		t.Fatalf("UpstreamErrorFrom(%v) = false, want true", err)
	}
	if upstream.Code != "token_invalidated" {
		t.Fatalf("upstream code = %q, want token_invalidated", upstream.Code)
	}
	if upstream.Message != "Your authentication token has been invalidated. Please try signing in again." {
		t.Fatalf("upstream message = %q", upstream.Message)
	}
}

func TestUpstreamErrorFromMessageExtractsTokenRevoked(t *testing.T) {
	message := `GET https://chatgpt.com/backend-api/wham/usage failed: 401 Unauthorized; body={
  "error": {
    "message": "Encountered invalidated oauth token for user, failing request",
    "code": "token_revoked"
  },
  "status": 401
} *errors.errorString`

	upstream := upstreamErrorFromMessage(message)
	if upstream == nil {
		t.Fatal("upstream error is nil")
	}
	if upstream.Code != "token_revoked" {
		t.Fatalf("upstream code = %q, want token_revoked", upstream.Code)
	}
	if upstream.Message != "Encountered invalidated oauth token for user, failing request" {
		t.Fatalf("upstream message = %q", upstream.Message)
	}
}

func TestStartUsesTemporarySQLiteHomeForCodexHome(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	codexHome := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, Config{
		Bin:       executable,
		CodexHome: codexHome,
		Env:       []string{"AICAM_FAKE_APPSERVER=1"},
	})
	if err != nil {
		t.Fatalf("start client: %v", err)
	}

	env := envMap(client.cmd.Env)
	if env["CODEX_HOME"] != codexHome {
		t.Fatalf("CODEX_HOME = %q, want %q", env["CODEX_HOME"], codexHome)
	}
	sqliteHome := env["CODEX_SQLITE_HOME"]
	if sqliteHome == "" {
		t.Fatal("CODEX_SQLITE_HOME is empty")
	}
	if sqliteHome == codexHome {
		t.Fatal("CODEX_SQLITE_HOME should not reuse CODEX_HOME")
	}
	if _, err := os.Stat(sqliteHome); err != nil {
		t.Fatalf("stat sqlite home: %v", err)
	}

	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("close client: %v", err)
	}
	if _, err := os.Stat(sqliteHome); !os.IsNotExist(err) {
		t.Fatalf("sqlite home still exists or stat failed: %v", err)
	}
}

func TestStartIncludesStderrWhenInitializationFails(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Start(ctx, Config{
		Bin: executable,
		Env: []string{"AICAM_FAKE_APPSERVER=stderr-exit"},
	})
	if err == nil {
		t.Fatal("Start() succeeded, want initialization error")
	}
	if !strings.Contains(err.Error(), "app-server stderr: fake app-server stderr detail") {
		t.Fatalf("error = %v, want captured stderr", err)
	}
}

func envMap(env []string) map[string]string {
	values := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("AICAM_FAKE_APPSERVER"); mode != "" {
		runFakeAppServer(mode)
		return
	}
	os.Exit(m.Run())
}

func runFakeAppServer(mode string) {
	if mode == "stderr-exit" {
		_, _ = fmt.Fprintln(os.Stderr, "fake app-server stderr detail")
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var message map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			os.Exit(2)
		}
		id, hasID := message["id"]
		if !hasID {
			continue
		}
		var method string
		_ = json.Unmarshal(message["method"], &method)

		switch method {
		case "initialize":
			writeFakeResponse(encoder, id, map[string]string{"userAgent": "fake"})
		case "test/echo":
			_ = encoder.Encode(map[string]any{
				"method": "account/rateLimits/updated",
				"params": map[string]any{},
			})
			writeFakeResponse(encoder, id, map[string]string{"value": "ok"})
		case "test/error":
			_ = encoder.Encode(map[string]any{
				"id": id,
				"error": map[string]any{
					"code":    -32601,
					"message": "method missing",
				},
			})
		case "test/upstream-error":
			_ = encoder.Encode(map[string]any{
				"id": id,
				"error": map[string]any{
					"code":    -32000,
					"message": "GET https://chatgpt.com/backend-api/wham/usage failed: 401 Unauthorized; body={\n  \"error\": {\n    \"message\": \"Your authentication token has been invalidated. Please try signing in again.\",\n    \"code\": \"token_invalidated\"\n  },\n  \"status\": 401\n} *errors.errorString",
				},
			})
		default:
			_ = encoder.Encode(map[string]any{
				"id": id,
				"error": map[string]any{
					"code":    -32601,
					"message": fmt.Sprintf("unknown method %s", method),
				},
			})
		}
	}
	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read fake app-server input: %v\n", err)
		os.Exit(2)
	}
}

func writeFakeResponse(encoder *json.Encoder, id json.RawMessage, result any) {
	_ = encoder.Encode(map[string]any{
		"id":     id,
		"result": result,
	})
}
