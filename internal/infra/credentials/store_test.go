package credentials

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAccountAuthJSONBoundaries(t *testing.T) {
	store, err := NewStore(Config{RootDir: filepath.Join(t.TempDir(), "credentials"), ActiveCodexDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := store.ReadAccountAuthJSON(ctx, "codex", "../outside"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ReadAccountAuthJSON(cancelled, "codex", "account"); err != context.Canceled {
		t.Fatalf("cancelled read = %v", err)
	}
	accountDir, err := store.AccountCodexDir("codex", "account")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(accountDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"valid", "{\"future\":true}\n", true},
		{"empty", "{}", false},
		{"multiple-values", "{\"future\":true} {}", false},
		{"oversized-valid-prefix", "{\"future\":true}" + strings.Repeat(" ", 2*1024*1024), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(accountDir, authFileName), []byte(tc.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := store.ReadAccountAuthJSON(ctx, "codex", "account")
			if tc.valid && (err != nil || string(got) != tc.payload) {
				t.Fatal("valid credential content must remain unchanged")
			}
			if !tc.valid && (err == nil || got != nil) {
				t.Fatal("invalid or oversized credentials must not be returned")
			}
		})
	}
}

func TestStoreImportsActivatesAndRemovesAuth(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	sourceDir := filepath.Join(tempDir, "source")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatalf("create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, authFileName), []byte(`{"tokens":{"access_token":"secret"}}`), 0o600); err != nil {
		t.Fatalf("write source auth: %v", err)
	}

	store, err := NewStore(Config{
		RootDir:        filepath.Join(tempDir, "credentials"),
		ActiveCodexDir: activeDir,
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := store.ImportFromCodexDir(context.Background(), "codex", "storage-1", sourceDir); err != nil {
		t.Fatalf("import auth: %v", err)
	}
	accountDir, err := store.AccountCodexDir("codex", "storage-1")
	if err != nil {
		t.Fatalf("account dir: %v", err)
	}
	assertFileMode(t, filepath.Join(accountDir, authFileName), 0o600)

	runtimeDir := filepath.Join(tempDir, "runtime")
	if err := store.ExportToCodexDir(context.Background(), "codex", "storage-1", runtimeDir); err != nil {
		t.Fatalf("export auth: %v", err)
	}
	assertFileMode(t, filepath.Join(runtimeDir, authFileName), 0o600)

	if err := store.ActivateAccount(context.Background(), "codex", "storage-1"); err != nil {
		t.Fatalf("activate account: %v", err)
	}
	assertFileMode(t, filepath.Join(activeDir, authFileName), 0o600)

	if err := store.RemoveAccount(context.Background(), "codex", "storage-1"); err != nil {
		t.Fatalf("remove account: %v", err)
	}
	if _, err := os.Stat(accountDir); !os.IsNotExist(err) {
		t.Fatalf("account dir still exists or stat failed: %v", err)
	}
}

func TestStoreImportsRawAuthJSON(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(Config{
		RootDir:        filepath.Join(tempDir, "credentials"),
		ActiveCodexDir: filepath.Join(tempDir, "active"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	authJSON := []byte(`{"tokens":{"access_token":"secret"}}`)
	codexDir := filepath.Join(tempDir, "runtime")
	if err := store.ImportAuthJSONToCodexDir(context.Background(), codexDir, authJSON); err != nil {
		t.Fatalf("import raw auth json: %v", err)
	}
	authPath := filepath.Join(codexDir, authFileName)
	assertFileMode(t, authPath, 0o600)
	content, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth json: %v", err)
	}
	if string(content) != string(authJSON) {
		t.Fatalf("auth content = %s, want %s", content, authJSON)
	}
}

func TestStoreRejectsInvalidRawAuthJSON(t *testing.T) {
	store, err := NewStore(Config{
		RootDir:        filepath.Join(t.TempDir(), "credentials"),
		ActiveCodexDir: filepath.Join(t.TempDir(), "active"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := store.ImportAuthJSONToCodexDir(context.Background(), filepath.Join(t.TempDir(), "runtime"), []byte(`not-json`)); err == nil {
		t.Fatal("ImportAuthJSONToCodexDir accepted invalid JSON")
	}
}

func TestStoreRejectsInvalidSegments(t *testing.T) {
	store, err := NewStore(Config{
		RootDir:        filepath.Join(t.TempDir(), "credentials"),
		ActiveCodexDir: filepath.Join(t.TempDir(), "active"),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	_, err = store.AccountCodexDir("codex", "../bad")
	if err == nil {
		t.Fatal("AccountCodexDir accepted path traversal segment")
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
