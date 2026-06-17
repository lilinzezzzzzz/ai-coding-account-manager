package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
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

func TestMain(m *testing.M) {
	if os.Getenv("AICAM_FAKE_APPSERVER") == "1" {
		runFakeAppServer()
		return
	}
	os.Exit(m.Run())
}

func runFakeAppServer() {
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
}

func writeFakeResponse(encoder *json.Encoder, id json.RawMessage, result any) {
	_ = encoder.Encode(map[string]any{
		"id":     id,
		"result": result,
	})
}
