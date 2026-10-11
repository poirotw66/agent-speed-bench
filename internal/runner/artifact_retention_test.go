package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func TestRetainFilesPreservesPartialEvidenceAndRejectsEscapes(t *testing.T) {
	workspace, artifacts := t.TempDir(), t.TempDir()
	for _, name := range []string{"first.go", "last.go"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape.go")); err != nil {
		t.Fatal(err)
	}
	err := retainFiles(workspace, artifacts, []string{"first.go", "missing.go", "escape.go", "last.go", "also-missing.go"})
	if err == nil {
		t.Fatal("missing or escaped files were accepted")
	}
	for _, want := range []string{"missing.go", "also-missing.go", "escapes workspace"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(artifacts, "source_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []SourceArtifact
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest) != 2 {
		t.Fatal(manifest, err)
	}
	for i, name := range []string{"first.go", "last.go"} {
		data, err := os.ReadFile(filepath.Join(artifacts, "candidate", name+".txt"))
		sum := sha256.Sum256(data)
		if err != nil || string(data) != name || manifest[i].Path != name || manifest[i].Bytes != len(data) || manifest[i].SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal(manifest, err)
		}
	}
	if _, err := os.Stat(filepath.Join(artifacts, "candidate", "escape.go.txt")); !os.IsNotExist(err) {
		t.Fatal("escaped data was retained", err)
	}
}

func TestRetainFilesWritesEmptyManifestWhenAllRequestedFilesFail(t *testing.T) {
	artifacts := t.TempDir()
	if err := retainFiles(t.TempDir(), artifacts, []string{"missing.go", "also-missing.go"}); err == nil {
		t.Fatal("missing files accepted")
	}
	data, err := os.ReadFile(filepath.Join(artifacts, "source_manifest.json"))
	if err != nil || string(data) != "[]" {
		t.Fatal(string(data), err)
	}
}

func TestPartialCaptureFailurePreservesPassingGrade(t *testing.T) {
	task := fixtureTask()
	task.Files = map[string]string{"first.go": "first", "last.go": "last"}
	task.RetainFiles = []string{"first.go", "missing.go", "last.go"}
	task.Verify.OutputContains = "Done"
	r, err := executeOne(context.Background(), benchmark.Agent{Name: "fixture", Adapter: "generic", Command: "sh", Args: []string{"-c", "printf Done"}}, task, 1, "test", t.TempDir())
	if err != nil || r.Status != "completed" || r.Success == nil || !*r.Success || len(r.ArtifactErrors) != 1 {
		t.Fatal(r, err)
	}
	for _, name := range []string{"first.go", "last.go"} {
		if _, err := os.Stat(filepath.Join(r.ArtifactDir, "candidate", name+".txt")); err != nil {
			t.Fatal(err)
		}
	}
}
