package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

type Provenance struct {
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
