package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic flushes a complete replacement before publishing it. A failed
// write cannot leave a partial fact, index, or cursor in the destination file.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".memory-*")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing temporary file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replacing file: %w", err)
	}
	return nil
}
