package adapters

import (
	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
	"strings"
	"testing"
)

func TestCursorTerminalUsage(t *testing.T) {
	for _, raw := range []string{`{"inputTokens":16505,"outputTokens":262,"cacheReadTokens":1152,"cacheWriteTokens":0}`, `{"outputTokens":null}`, `{}`} {
		p, _ := New(benchmark.Agent{Adapter: "cursor"})
		events, err := p.ParseEvent([]byte(`{"type":"result","subtype":"success","result":"Done","usage":` + raw + `}`))
		if err != nil {
			t.Fatal(err)
		}
		m := telemetry.Calculate(events, 10, nil)
		if strings.Contains(raw, "262") {
			if m.Usage.OutputTokens == nil || *m.Usage.OutputTokens != 262 || *m.Usage.CacheWriteTokens != 0 || *m.EffectiveOutputTPS != 26.2 {
				t.Fatal(m)
			}
		} else if m.Usage.OutputTokens != nil {
			t.Fatal(m)
		}
	}
	for _, v := range []string{"-1", "1.2", `"262"`} {
		p, _ := New(benchmark.Agent{Adapter: "cursor"})
		if _, err := p.ParseEvent([]byte(`{"type":"result","subtype":"success","usage":{"outputTokens":` + v + `}}`)); err == nil {
			t.Fatal("invalid usage accepted", v)
		}
	}
}
func TestNativeCommandOptions(t *testing.T) {
	for _, trust := range []bool{false, true} {
		p, _ := New(benchmark.Agent{Adapter: "cursor", TrustWorkspace: trust})
		c, _ := p.BuildCommand("--trust", "")
		found := false
		for _, arg := range c.Args[:len(c.Args)-2] {
			if arg == "--trust" {
				found = true
			}
		}
		if found != trust {
			t.Fatal(c)
		}
	}
	p, _ := New(benchmark.Agent{Adapter: "codex", ServiceTier: "fast"})
	c, _ := p.BuildCommand("hi", "")
	args := strings.Join(c.Args, " ")
	if !strings.Contains(args, `--config service_tier="fast" --enable fast_mode`) {
		t.Fatal(c)
	}
	p, _ = New(benchmark.Agent{Adapter: "agy", Model: "gemini-3.8-flash-high", ReasoningEffort: "high"})
	c, _ = p.BuildCommand("$(touch nope)", "")
	if c.Path != "agy" || c.Args[1] != "$(touch nope)" || strings.Join(c.Args[2:], " ") != "--output-format stream-json --dangerously-skip-permissions --model gemini-3.8-flash-high --effort high" {
		t.Fatal(c)
	}
}

func TestNativeCommandsUseRequestedYoloPolicy(t *testing.T) {
	for adapter, flag := range map[string]string{"codex": "--yolo", "cursor": "--yolo", "agy": "--dangerously-skip-permissions", "claude": "--dangerously-skip-permissions"} {
		for _, isolate := range []bool{false, true} {
			p, err := New(benchmark.Agent{Adapter: adapter, IsolateConfig: isolate})
			if err != nil {
				t.Fatal(err)
			}
			c, err := p.BuildCommand("fixture", "/workspace")
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for i, arg := range c.Args {
				if arg == flag {
					count++
				}
				if arg == "--auto-review" || arg == "--mode" || (arg == "--sandbox" && (adapter != "cursor" || i+1 >= len(c.Args) || c.Args[i+1] != "disabled")) {
					t.Fatal("conflicting permission policy", c)
				}
			}
			if count != 1 {
				t.Fatal("YOLO flag must appear exactly once", c)
			}
		}
	}
}

func TestCodexConfigIsolation(t *testing.T) {
	for _, isolate := range []bool{false, true} {
		p, _ := New(benchmark.Agent{Adapter: "codex", IsolateConfig: isolate})
		c, _ := p.BuildCommand("hello", "")
		args := strings.Join(c.Args, " ")
		if strings.Contains(args, "--ignore-user-config --ignore-rules") != isolate {
			t.Fatal(c)
		}
		for _, feature := range []string{"memories", "plugins", "apps", "browser_use", "computer_use"} {
			if strings.Contains(args, "--disable "+feature) != isolate {
				t.Fatal(c)
			}
		}
		if c.Stdin != "hello" || c.Args[len(c.Args)-1] != "-" {
			t.Fatal(c)
		}
	}
}
func TestAgyNativeTraceAndAuthoritativeTotals(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "agy"})
	lines := []string{
		`{"event":"init","init":{"model":"gemini-3.8-flash-high"}}`,
		`{"event":"step_update","step_update":{"step_index":1,"state":"ACTIVE","step_type":"tool","tool_name":"Read"}}`,
		`{"event":"step_update","step_update":{"step_index":1,"state":"DONE","step_type":"tool","duration_seconds":99}}`,
		`{"event":"step_update","step_update":{"step_index":3,"state":"ACTIVE","step_type":"agent_response","text_delta":"Do"}}`,
		`{"event":"step_update","step_update":{"step_index":3,"state":"DONE","step_type":"agent_response","text_delta":"ne","usage":{"output_tokens":999}}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"Done","duration_seconds":5.990406,"usage":{"input_tokens":15889,"output_tokens":510,"thinking_tokens":214,"cache_read_tokens":0}}}`,
	}
	var all []telemetry.Event
	for i, line := range lines {
		events, err := p.ParseEvent([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			e.ElapsedNS = int64(i) * 1000000000
			all = append(all, e)
		}
	}
	// Duplicate result must not add a second transcript or count tokens twice.
	extra, err := p.ParseEvent([]byte(lines[len(lines)-1]))
	if err != nil || len(extra) != 0 {
		t.Fatal(extra, err)
	}
	m := telemetry.Calculate(all, 13, nil)
	if !p.TerminalSeen() || *m.Usage.OutputTokens != 510 || *m.Usage.ThinkingTokens != 214 || *m.Usage.CachedTokens != 0 || m.Usage.OutputTokenAccounting != "includes_thinking" || *m.ReportedDurationSeconds != 5.990406 || m.ToolLatencyMeanSeconds != nil || m.MatchedToolCalls != 1 || *m.FirstTextDeltaSeconds != 3 || *m.FirstCompleteMessageSeconds != 5 {
		t.Fatal(m)
	}
	for _, line := range []string{`{"event":"result","result":{"status":"ERROR","error":{"message":"model not supported"}}}`, `{"event":"result","result":{"status":"WAITING"}}`} {
		p, _ := New(benchmark.Agent{Adapter: "agy"})
		e, err := p.ParseEvent([]byte(line))
		if err != nil || len(e) != 1 || e[0].Failure == nil || p.TerminalSeen() {
			t.Fatal(e, err)
		}
	}
	for _, line := range []string{`{"event":"result","result":{"status":"SUCCESS","duration_seconds":-1}}`, `{"event":"result","result":{"status":"SUCCESS","usage":{"thinking_tokens":-1}}}`} {
		p, _ := New(benchmark.Agent{Adapter: "agy"})
		if _, err := p.ParseEvent([]byte(line)); err == nil || p.TerminalSeen() {
			t.Fatal(line)
		}
	}
}

func TestGenericRejectsInvalidNewTelemetry(t *testing.T) {
	for _, line := range []string{`{"type":"agent_metadata","reported_duration_seconds":-1}`, `{"type":"usage_total","usage":{"thinking_tokens":-1}}`, `{"type":"usage_total","usage":{"cache_write_tokens":-1}}`} {
		p, _ := New(benchmark.Agent{Adapter: "generic"})
		if _, err := p.ParseEvent([]byte(line)); err == nil {
			t.Fatal(line)
		}
	}
}

func TestCodexReasoningAndCacheWriteAliases(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "codex"})
	e, err := p.ParseEvent([]byte(`{"type":"turn.completed","usage":{"output_tokens":222,"reasoning_output_tokens":13,"cache_write_input_tokens":0}}`))
	if err != nil || *e[0].Usage.ThinkingTokens != 13 || *e[0].Usage.CacheWriteTokens != 0 || *e[0].Usage.OutputTokens != 222 {
		t.Fatal(e, err)
	}
	for _, raw := range []string{`{"reasoning_output_tokens":-1}`, `{"cache_write_input_tokens":1.5}`, `{"thinking_tokens":2,"reasoning_output_tokens":3}`} {
		p, _ := New(benchmark.Agent{Adapter: "codex"})
		if _, err := p.ParseEvent([]byte(`{"type":"turn.completed","usage":` + raw + `}`)); err == nil {
			t.Fatal(raw)
		}
	}
}
