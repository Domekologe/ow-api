package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReplacingCreatesAndOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")

	if err := writeFileReplacing(path, []byte("first"), 0o644); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Fatalf("content = %q, want %q", got, "first")
	}

	// Overwriting an existing file must work on every platform.
	if err := writeFileReplacing(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("content = %q, want %q", got, "second")
	}
}

// TestWriteFileReplacingLeavesNoTempFiles guards against the temp file leaking
// into the data directory on success.
func TestWriteFileReplacingLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")

	if err := writeFileReplacing(path, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "data.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("directory contains %v, want only data.json", names)
	}
}

func TestWriteFileReplacingFailsOnMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "data.json")

	if err := writeFileReplacing(path, []byte("x"), 0o644); err == nil {
		t.Fatal("expected an error when the target directory does not exist")
	}
}
