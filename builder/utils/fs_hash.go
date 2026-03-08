package utils

import (
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zeebo/xxh3"
)

func HashDirsFast(dirs []string) (string, error) {
	h := xxh3.New()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(h, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano()); err != nil {
				return fmt.Errorf("failed to write to hash: %w", err)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	sum := h.Sum128()
	b := sum.Bytes()
	return hex.EncodeToString(b[:]), nil
}
