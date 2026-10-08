package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

type Provenance struct {
	InputSHA256    map[string]string `json:"input_sha256,omitempty"`
	BinarySHA256   string            `json:"binary_sha256"`
	SourceRevision string            `json:"source_revision"`
	SourceModified *bool             `json:"source_modified"`
	ConfigSHA256   string            `json:"config_sha256"`
	VerifierSHA256 map[string]string `json:"verifier_sha256"`
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func provenance(cfg benchmark.Config) (Provenance, error) {
	p := Provenance{SourceRevision: "unknown", VerifierSHA256: map[string]string{}}
	path, err := os.Executable()
	if err != nil {
		return p, err
	}
	p.BinarySHA256, err = fileHash(path)
	if err != nil {
		return p, err
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				p.SourceRevision = setting.Value
			case "vcs.modified":
				modified, err := strconv.ParseBool(setting.Value)
				if err == nil {
					p.SourceModified = &modified
				}
			}
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return p, err
	}
	sum := sha256.Sum256(data)
	p.ConfigSHA256 = hex.EncodeToString(sum[:])
	for _, task := range cfg.Cases {
		for _, input := range task.Verify.Inputs {
			if p.InputSHA256 == nil {
				p.InputSHA256 = map[string]string{}
			}
			if err := fingerprintInput(input, p.InputSHA256); err != nil {
				return p, err
			}
		}
		checks := append([]benchmark.Check(nil), task.Verify.CoreTests...)
		checks = append(checks, task.Verify.RegressionTests...)
		checks = append(checks, task.CacheWarmup...)
		if task.Verify.Command != "" {
			checks = append(checks, benchmark.Check{Command: task.Verify.Command})
		}
		for _, check := range checks {
			path, err := exec.LookPath(check.Command)
			if err != nil {
				return p, err
			}
			if _, exists := p.VerifierSHA256[path]; exists {
				continue
			}
			p.VerifierSHA256[path], err = fileHash(path)
			if err != nil {
				return p, err
			}
		}
	}
	return p, nil
}

// Explicit dependencies only: do not infer arbitrary command argument semantics.
func fingerprintInput(root string, hashes map[string]string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > maxLineBytes {
			return fmt.Errorf("verifier input must be a bounded regular file: %s", path)
		}
		if _, exists := hashes[path]; exists {
			return nil
		}
		if len(hashes) >= 10000 {
			return fmt.Errorf("too many verifier input files")
		}
		hashes[path], err = fileHash(path)
		return err
	})
}
