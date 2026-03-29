package service

import (
	"os"
	"runtime"
)

// writeFileReplacing writes data by replacing path atomically (Unix) or
// replace-after-delete (Windows) so a crash cannot leave a truncated file.
func writeFileReplacing(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
