package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
