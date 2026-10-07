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
	if c.Path != "agy" || c.Args[1] != "$(touch nope)" || strings.Join(c.Args[2:], " ") != "--output-format stream-json --model gemini-3.8-flash-high --effort high" {
		t.Fatal(c)
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
