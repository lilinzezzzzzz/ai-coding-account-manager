package logging

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDailyWriterRotatesAcrossDayBoundary(t *testing.T) {
	currentTime := time.Date(2026, 7, 3, 23, 59, 0, 0, time.Local)
	path := filepath.Join(t.TempDir(), "server.log")
	writer, err := newDailyWriter(path, func() time.Time { return currentTime })
	if err != nil {
		t.Fatalf("newDailyWriter() error = %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("day one\n")); err != nil {
		t.Fatalf("Write(day one) error = %v", err)
	}
	currentTime = currentTime.Add(2 * time.Minute)
	if _, err := writer.Write([]byte("day two\n")); err != nil {
		t.Fatalf("Write(day two) error = %v", err)
	}

	assertFileContent(t, filepath.Join(filepath.Dir(path), "server-2026-07-03.log"), "day one\n")
	assertFileContent(t, path, "day two\n")
}

func TestDailyWriterDoesNotOverwriteExistingArchive(t *testing.T) {
	currentTime := time.Date(2026, 7, 3, 23, 59, 0, 0, time.Local)
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")
	writer, err := newDailyWriter(path, func() time.Time { return currentTime })
	if err != nil {
		t.Fatalf("newDailyWriter() error = %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	if err := os.WriteFile(filepath.Join(dir, "server-2026-07-03.log"), []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("seed archive error = %v", err)
	}
	if _, err := writer.Write([]byte("new archive\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	currentTime = currentTime.Add(2 * time.Minute)
	if _, err := writer.Write([]byte("current\n")); err != nil {
		t.Fatalf("Write(after rotation) error = %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "server-2026-07-03.log"), "existing\n")
	assertFileContent(t, filepath.Join(dir, "server-2026-07-03.1.log"), "new archive\n")
}

func TestDailyWriterArchivesStaleFileOnStartup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")
	previousDay := time.Date(2026, 7, 3, 12, 0, 0, 0, time.Local)
	if err := os.WriteFile(path, []byte("previous process\n"), 0o600); err != nil {
		t.Fatalf("seed current log error = %v", err)
	}
	if err := os.Chtimes(path, previousDay, previousDay); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	writer, err := newDailyWriter(path, func() time.Time { return previousDay.Add(24 * time.Hour) })
	if err != nil {
		t.Fatalf("newDailyWriter() error = %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Write([]byte("new process\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "server-2026-07-03.log"), "previous process\n")
	assertFileContent(t, path, "new process\n")
}

func TestDailyWriterSerializesConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	writer, err := NewDailyWriter(path)
	if err != nil {
		t.Fatalf("NewDailyWriter() error = %v", err)
	}

	const writes = 32
	var waitGroup sync.WaitGroup
	for range writes {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if _, err := writer.Write([]byte("entry\n")); err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}()
	}
	waitGroup.Wait()
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := len(content), writes*len("entry\n"); got != want {
		t.Fatalf("log size = %d, want %d", got, want)
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if got := string(content); got != want {
		t.Fatalf("content of %s = %q, want %q", path, got, want)
	}
}
