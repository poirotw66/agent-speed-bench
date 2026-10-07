package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/poirotw66/agent-speed-bench/internal/adapters"
)

// Retained files are allowlisted and must resolve inside the prepared workspace.
func retainFiles(workspace, artifactDir string, paths []string) error {
	manifest := []SourceArtifact{}
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
		sum := sha256.Sum256(data)
		manifest = append(manifest, SourceArtifact{Path: path, SHA256: hex.EncodeToString(sum[:]), Bytes: len(data)})
	}
	if len(manifest) > 0 {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(artifactDir, "source_manifest.json"), data, 0600)
	}
	return nil
}

type SourceArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type patchBuffer struct{ strings.Builder }

func (b *patchBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxLineBytes {
		return 0, fmt.Errorf("submitted patch exceeds artifact limit")
	}
	return b.Builder.Write(p)
}

func retainPatch(ctx context.Context, workspace, artifacts string, paths []string) error {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"--no-pager", "--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD", "--"}
	args = append(args, paths...)
	var patch patchBuffer
	var diagnostics limitedBuffer
	result := RunProcess(bounded, adapters.Command{Path: "git", Args: args}, workspace, &patch, &diagnostics)
	if result.Err != nil {
		return fmt.Errorf("submitted patch capture failed: %w", result.Err)
	}
	for _, path := range paths {
		tracked := RunProcess(bounded, adapters.Command{Path: "git", Args: []string{"--literal-pathspecs", "ls-files", "--error-unmatch", "--", path}}, workspace, &limitedBuffer{}, &limitedBuffer{})
		if tracked.Err == nil {
			continue
		}
		addition := RunProcess(bounded, adapters.Command{Path: "git", Args: []string{"--no-pager", "--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--binary", "--no-index", "--", "/dev/null", path}}, workspace, &patch, &diagnostics)
		if addition.ExitCode == nil || (*addition.ExitCode != 0 && *addition.ExitCode != 1) {
			return fmt.Errorf("new submitted file patch capture failed")
		}
	}
	return os.WriteFile(filepath.Join(artifacts, "candidate.patch.txt"), []byte(patch.String()), 0600)
}
