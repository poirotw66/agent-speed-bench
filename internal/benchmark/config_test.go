package benchmark

import (
	"os"
	"path/filepath"
	"testing"
)

func loadText(t *testing.T, text string) (Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bench.yaml")
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(p)
}

const validConfig = `name: test
agents:
  - name: custom
    adapter: generic
    command: ./agent
cases:
  - name: task
    prompt: hello
`

func TestDefaultsAndRelativePaths(t *testing.T) {
	c, err := loadText(t, validConfig)
	if err != nil {
		t.Fatal(err)
	}
	if c.Jobs != 1 || c.Repeats != 1 || c.Cases[0].TimeoutSeconds != 300 || c.Cases[0].Verify.TimeoutSeconds != 60 || !filepath.IsAbs(c.Agents[0].Command) {
		t.Fatal(c)
	}
}
func TestConfigRejectsInvalidInput(t *testing.T) {
	for name, tail := range map[string]string{"unknown-field": "oops: true\n", "second-document": "---\nname: another\n", "negative-jobs": "jobs: -1\n", "bad-case": "cases:\n  - name: ../bad\n    prompt: hello\n", "unsafe-seed": "cases:\n  - name: task\n    prompt: hello\n    files: {../outside: bad}\n"} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadText(t, validConfig+tail); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}
func TestSafeSeedPaths(t *testing.T) {
	for _, p := range []string{"", ".", "..", "../out", "/tmp/out", ".git/config", "nested/.git", "nested/.git/config"} {
		if SafePath(p) {
			t.Fatalf("accepted %q", p)
		}
	}
	for _, p := range []string{"README.md", "src/main.go"} {
		if !SafePath(p) {
			t.Fatalf("rejected %q", p)
		}
	}
}

func TestReasoningEffortAndSequenceValidation(t *testing.T) {
	cases := []string{
		"name: test\nagents:\n - name: a\n   adapter: codex\n   reasoning_effort: unsupported\ncases:\n - name: x\n   prompt: hi\n",
		"name: test\nagents:\n - name: a\n   adapter: claude\n   reasoning_effort: high\ncases:\n - name: x\n   prompt: hi\n",
		"name: test\nagents:\n - name: a\n   adapter: generic\n   command: sh\ncases:\n - name: x\n   prompt: hi\n   verify:\n     integer_sequence: {start: -9223372036854775808, end: 9223372036854775807, end_marker: DONE}\n",
	}
	for _, text := range cases {
		if _, err := loadText(t, text); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	text := "name: test\nagents:\n - name: a\n   adapter: codex\n   reasoning_effort: high\ncases:\n - name: x\n   prompt: hi\n   verify:\n     integer_sequence: {start: 1, end: 100, end_marker: DONE}\n"
	cfg, err := loadText(t, text)
	if err != nil || cfg.Agents[0].ReasoningEffort != "high" || cfg.Cases[0].Verify.IntegerSequence.End != 100 {
		t.Fatal(cfg, err)
	}
}

func TestNativeOptionsAndExamples(t *testing.T) {
	for _, agent := range []string{"adapter: cursor\n   trust_workspace: true", "adapter: codex\n   service_tier: fast", "adapter: agy\n   reasoning_effort: medium"} {
		if _, err := loadText(t, "name: native\nagents:\n - name: a\n   "+agent+"\ncases:\n - name: task\n   prompt: hi\n"); err != nil {
			t.Fatal(err)
		}
	}
	for _, agent := range []string{"adapter: codex\n   trust_workspace: true", "adapter: cursor\n   service_tier: fast", "adapter: codex\n   service_tier: turbo", "adapter: agy\n   reasoning_effort: minimal"} {
		if _, err := loadText(t, "name: native\nagents:\n - name: a\n   "+agent+"\ncases:\n - name: task\n   prompt: hi\n"); err == nil {
			t.Fatal(agent)
		}
	}
	for _, path := range []string{"../../benchmarks/antigravity.example.yaml", "../../benchmarks/native-comparison.yaml"} {
		if _, err := Load(path); err != nil {
			t.Fatal(err)
		}
	}
}
