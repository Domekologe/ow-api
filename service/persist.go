package service

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileReplacing writes data to path atomically.
//
// The temp file is created in the same directory as the target so the rename
// stays within one filesystem. On Unix os.Rename replaces the destination
// atomically. On Windows it fails when the destination exists, so we fall back
// to delete-then-rename -- but only after the rename actually failed, which
// keeps the vulnerable window as small as the platform allows.
func writeFileReplacing(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any failure path.
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	// Flush to disk before the rename, otherwise a crash can leave a renamed but
	// empty file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err == nil {
		tmpName = "" // Renamed successfully, so nothing to clean up.
		return nil
	}

	// Windows: the destination must not exist. Remove it and retry.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	tmpName = ""
	return nil
}
