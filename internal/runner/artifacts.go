package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Retained files are allowlisted and must resolve inside the prepared workspace.
func retainFiles(workspace, artifactDir string, paths []string) error {
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	for _, path := range paths {
		source, err := filepath.EvalSymlinks(filepath.Join(workspace, path))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, source)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("retained file escapes workspace: %s", path)
		}
		f, err := os.Open(source)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxLineBytes {
			f.Close()
			return fmt.Errorf("retained file must be regular and at most %d bytes: %s", maxLineBytes, path)
		}
		data, err := io.ReadAll(io.LimitReader(f, maxLineBytes+1))
		f.Close()
		if err != nil {
			return err
		}
		if len(data) > maxLineBytes {
			return fmt.Errorf("retained file too large: %s", path)
		}
		dest := filepath.Join(artifactDir, "candidate", path+".txt")
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0600); err != nil {
			return err
		}
	}
	return nil
}
