// Package localfile provides atomic, owner-only application-state writes.
package localfile

import (
	"errors"
	"os"
	"path/filepath"
)

// WritePrivate replaces path atomically using a temporary file in the same
// directory. A failed write leaves the previous file intact.
func WritePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".yati-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }() // Best-effort cleanup; after rename this path no longer exists.
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
