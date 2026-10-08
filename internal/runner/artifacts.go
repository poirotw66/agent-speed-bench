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

func retainPatch(ctx context.Context, workspace, artifacts string, paths []string, baseline string) error {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	indexDir, err := os.MkdirTemp("", "agentspeedbench-patch-index-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(indexDir)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(indexDir, "index")}
	var diagnostics limitedBuffer
	run := func(args []string, output io.Writer) error {
		result := RunProcess(bounded, adapters.Command{Path: "git", Args: args, Env: env}, workspace, output, &diagnostics)
		if result.Err != nil {
			return fmt.Errorf("submitted patch capture failed: %w", result.Err)
		}
		return nil
	}
	// A private index compares current files with the original baseline, regardless
	// of agent commits, index removal, staged deletions or newly created files.
	if err := run([]string{"read-tree", baseline}, io.Discard); err != nil {
		return err
	}
	args := append([]string{"--literal-pathspecs", "add", "-A", "-f", "--"}, paths...)
	if err := run(args, io.Discard); err != nil {
		return err
	}
	var patch patchBuffer
	args = append([]string{"--no-pager", "--literal-pathspecs", "diff", "--cached", "--no-ext-diff", "--no-textconv", "--binary", baseline, "--"}, paths...)
	if err := run(args, &patch); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(artifacts, "candidate.patch.txt"), []byte(patch.String()), 0600)
}
