package benchmark

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Agent struct {
	TrustWorkspace  bool     `yaml:"trust_workspace,omitempty" json:"trust_workspace,omitempty"`
	ServiceTier     string   `yaml:"service_tier,omitempty" json:"service_tier,omitempty"`
	Name            string   `yaml:"name" json:"name"`
	Adapter         string   `yaml:"adapter" json:"adapter"`
	Model           string   `yaml:"model,omitempty" json:"model,omitempty"`
	ReasoningEffort string   `yaml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	Command         string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args            []string `yaml:"args,omitempty" json:"args,omitempty"`
	VersionArgs     []string `yaml:"version_args,omitempty" json:"version_args,omitempty"`
}
type Repo struct {
	Path   string `yaml:"path" json:"path"`
	Commit string `yaml:"commit" json:"commit"`
}
type Check struct {
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
}

type Verify struct {
	CoreTests       []Check          `yaml:"core_tests,omitempty" json:"core_tests,omitempty"`
	RegressionTests []Check          `yaml:"regression_tests,omitempty" json:"regression_tests,omitempty"`
	Command         string           `yaml:"command,omitempty" json:"command,omitempty"`
	Args            []string         `yaml:"args,omitempty" json:"args,omitempty"`
	OutputContains  string           `yaml:"output_contains,omitempty" json:"output_contains,omitempty"`
	OutputEquals    *string          `yaml:"output_equals,omitempty" json:"output_equals,omitempty"`
	IntegerSequence *IntegerSequence `yaml:"integer_sequence,omitempty" json:"integer_sequence,omitempty"`
	TimeoutSeconds  int              `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}
type IntegerSequence struct {
	Start     int    `yaml:"start" json:"start"`
	End       int    `yaml:"end" json:"end"`
	EndMarker string `yaml:"end_marker" json:"end_marker"`
}
type Case struct {
	RetainFiles    []string          `yaml:"retain_files,omitempty" json:"retain_files,omitempty"`
	Name           string            `yaml:"name" json:"name"`
	Prompt         string            `yaml:"prompt" json:"prompt"`
	TimeoutSeconds int               `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	Repo           *Repo             `yaml:"repo,omitempty" json:"repo,omitempty"`
	Files          map[string]string `yaml:"files,omitempty" json:"files,omitempty"`
	Verify         Verify            `yaml:"verify,omitempty" json:"verify,omitempty"`
}
type Config struct {
	WarmupRepeats  int     `yaml:"warmup_repeats,omitempty" json:"warmup_repeats,omitempty"`
	Name           string  `yaml:"name" json:"name"`
	Repeats        int     `yaml:"repeats" json:"repeats"`
	Jobs           int     `yaml:"jobs" json:"jobs"`
	TimeoutSeconds int     `yaml:"timeout_seconds" json:"timeout_seconds"`
	Agents         []Agent `yaml:"agents" json:"agents"`
	Cases          []Case  `yaml:"cases" json:"cases"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,79}$`)

func SafePath(path string) bool {
	p := filepath.Clean(path)
	if path == "" || p == "." || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
		return false
	}
	for _, part := range strings.Split(p, string(filepath.Separator)) {
		if part == ".git" {
			return false
		}
	}
	return true
}

func Load(path string) (Config, error) {
	var cfg Config
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	d := yaml.NewDecoder(f)
	d.KnownFields(true)
	if err := d.Decode(&cfg); err != nil {
		return cfg, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return cfg, errors.New("configuration must contain exactly one YAML document")
	}
	if cfg.Repeats == 0 {
		cfg.Repeats = 1
	}
	if cfg.Jobs == 0 {
		cfg.Jobs = 1
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 300
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return cfg, err
	}
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		if a.Adapter == "" {
			a.Adapter = a.Name
		}
		if strings.ContainsRune(a.Command, filepath.Separator) && !filepath.IsAbs(a.Command) {
			a.Command = filepath.Join(base, a.Command)
		}
	}
	for i := range cfg.Cases {
		c := &cfg.Cases[i]
		if c.TimeoutSeconds == 0 {
			c.TimeoutSeconds = cfg.TimeoutSeconds
		}
		if c.Verify.TimeoutSeconds == 0 {
			c.Verify.TimeoutSeconds = 60
		}
		for _, checks := range [][]Check{c.Verify.CoreTests, c.Verify.RegressionTests} {
			for j := range checks {
				if strings.ContainsRune(checks[j].Command, filepath.Separator) && !filepath.IsAbs(checks[j].Command) {
					checks[j].Command = filepath.Join(base, checks[j].Command)
				}
			}
		}
		if c.Repo != nil && !filepath.IsAbs(c.Repo.Path) {
			c.Repo.Path = filepath.Join(base, c.Repo.Path)
		}
		if strings.ContainsRune(c.Verify.Command, filepath.Separator) && !filepath.IsAbs(c.Verify.Command) {
			c.Verify.Command = filepath.Join(base, c.Verify.Command)
		}
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if !safeName.MatchString(c.Name) {
		return errors.New("name must be a safe identifier of 1-80 characters")
	}
	if c.WarmupRepeats < 0 || c.WarmupRepeats > 100 {
		return errors.New("warmup_repeats must be 0-100")
	}
	if c.Repeats < 1 || c.Repeats > 1000 || c.Jobs < 1 || c.Jobs > 64 || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 86400 {
		return errors.New("repeats must be 1-1000, jobs 1-64, timeout_seconds 1-86400")
	}
	if len(c.Agents) == 0 || len(c.Cases) == 0 {
		return errors.New("at least one agent and case are required")
	}
	seen := map[string]bool{}
	for _, a := range c.Agents {
		if !safeName.MatchString(a.Name) || seen[a.Name] {
			return fmt.Errorf("invalid or duplicate agent name: %q", a.Name)
		}
		seen[a.Name] = true
		switch a.Adapter {
		case "codex", "claude", "cursor", "agy", "demo", "generic":
		default:
			return fmt.Errorf("unknown adapter %q; use generic for custom CLIs", a.Adapter)
		}
		if a.Adapter == "generic" && a.Command == "" {
			return fmt.Errorf("generic agent %s requires command", a.Name)
		}
		if a.Adapter != "generic" && len(a.Args) > 0 {
			return fmt.Errorf("agent %s: args are only allowed for generic adapters", a.Name)
		}
		if a.TrustWorkspace && a.Adapter != "cursor" {
			return fmt.Errorf("agent %s: trust_workspace is only supported for cursor", a.Name)
		}
		if a.ServiceTier != "" && (a.Adapter != "codex" || (a.ServiceTier != "default" && a.ServiceTier != "fast")) {
			return fmt.Errorf("agent %s: service_tier requires codex and default or fast", a.Name)
		}
		if a.Adapter == "agy" && a.ReasoningEffort == "minimal" {
			return fmt.Errorf("agent %s: agy does not support minimal effort", a.Name)
		}
		if a.ReasoningEffort != "" {
			if a.Adapter != "codex" && a.Adapter != "agy" {
				return fmt.Errorf("agent %s: reasoning_effort is supported only for codex and agy", a.Name)
			}
			switch a.ReasoningEffort {
			case "minimal", "low", "medium", "high", "xhigh", "max":
			default:
				return fmt.Errorf("agent %s: invalid reasoning_effort", a.Name)
			}
		}
	}
	seen = map[string]bool{}
	for _, task := range c.Cases {
		if !safeName.MatchString(task.Name) || seen[task.Name] {
			return fmt.Errorf("invalid or duplicate case name: %q", task.Name)
		}
		seen[task.Name] = true
		if strings.TrimSpace(task.Prompt) == "" {
			return fmt.Errorf("case %s requires a prompt", task.Name)
		}
		if task.TimeoutSeconds < 1 || task.TimeoutSeconds > 86400 || task.Verify.TimeoutSeconds < 1 || task.Verify.TimeoutSeconds > 86400 {
			return fmt.Errorf("case %s has an invalid timeout", task.Name)
		}
		if task.Repo != nil && (task.Repo.Path == "" || task.Repo.Commit == "" || strings.HasPrefix(task.Repo.Commit, "-")) {
			return fmt.Errorf("case %s repo requires path and commit", task.Name)
		}
		if task.Repo != nil && len(task.Files) > 0 {
			return fmt.Errorf("case %s cannot combine repo and seed files", task.Name)
		}
		for _, path := range task.RetainFiles {
			if !SafePath(path) {
				return fmt.Errorf("unsafe retained path: %q", path)
			}
		}
		for path := range task.Files {
			if !SafePath(path) {
				return fmt.Errorf("unsafe seed path: %q", path)
			}
		}
		for _, checks := range [][]Check{task.Verify.CoreTests, task.Verify.RegressionTests} {
			for _, check := range checks {
				if strings.TrimSpace(check.Command) == "" {
					return fmt.Errorf("case %s: scoring command is required", task.Name)
				}
			}
		}
		if task.Verify.Command == "" && len(task.Verify.Args) > 0 {
			return fmt.Errorf("case %s verifier args require command", task.Name)
		}
		if seq := task.Verify.IntegerSequence; seq != nil {
			if seq.Start < -1000000 || seq.End > 1000000 || seq.End < seq.Start || int64(seq.End)-int64(seq.Start) > 10000 || strings.TrimSpace(seq.EndMarker) == "" || strings.ContainsAny(seq.EndMarker, "\r\n") {
				return fmt.Errorf("case %s has invalid integer_sequence", task.Name)
			}
		}
	}
	return nil
}
