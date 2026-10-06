package adapters

import (
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func TestCodexDocumentedTrace(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "codex"})
	lines := []string{
		`{"type":"thread.started","thread_id":"fixture"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"ls","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","status":"completed","exit_code":0}}`,
		`{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Repo contains docs."}}`,
		`{"type":"turn.completed","usage":{"input_tokens":24763,"cached_input_tokens":24448,"output_tokens":122}}`,
	}
	var kinds []string
	for _, line := range lines {
		events, err := p.ParseEvent([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			kinds = append(kinds, e.Type)
			if e.Usage != nil && *e.Usage.OutputTokens != 122 {
				t.Fatal(e.Usage)
			}
		}
	}
	if got := strings.Join(kinds, ","); got != "tool_started,tool_finished,assistant_output,usage_reported,agent_completed" {
		t.Fatal(got)
	}
	if !p.TerminalSeen() {
		t.Fatal("missing terminal")
	}
	if _, err := p.ParseEvent([]byte(`{"type":"turn.started"}`)); err != nil {
		t.Fatal(err)
	}
	if p.TerminalSeen() {
		t.Fatal("new turn must require its own completion")
	}
}

func TestClaudeDeltasDoNotDoubleCountMessages(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "claude"})
	lines := []string{
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"t1","name":"Read"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"file"}]}}`,
		`{"type":"result","subtype":"success","result":"hello","usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":5}}`,
	}
	var text string
	var tools int
	for _, line := range lines {
		events, err := p.ParseEvent([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.Type == "assistant_output" {
				text += e.Text
			}
			if e.Type == "tool_started" || e.Type == "tool_finished" {
				tools++
			}
			if e.Usage != nil && (e.Usage.OutputTokens == nil || *e.Usage.OutputTokens != 0 || *e.Usage.CachedTokens != 5) {
				t.Fatal(e.Usage)
			}
		}
	}
	if text != "hello" || tools != 2 || !p.TerminalSeen() {
		t.Fatalf("text=%q tools=%d", text, tools)
	}
}

func TestCursorDuplicateFlushesAndUnknownUsage(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "cursor"})
	lines := []string{
		`{"type":"assistant","timestamp_ms":10,"message":{"content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","timestamp_ms":11,"model_call_id":"m1","message":{"content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"tool_call","subtype":"started","call_id":"t1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}`,
		`{"type":"tool_call","subtype":"completed","call_id":"t1","tool_call":{"readToolCall":{"result":{"success":{}}}}}`,
		`{"type":"result","subtype":"success","result":"hello"}`,
	}
	var text string
	var tools int
	for _, line := range lines {
		events, err := p.ParseEvent([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.Type == "assistant_output" {
				text += e.Text
			}
			if e.Type == "tool_started" || e.Type == "tool_finished" {
				tools++
			}
			if e.Usage != nil {
				t.Fatal("Cursor should not invent usage")
			}
		}
	}
	if text != "hello" || tools != 2 || !p.TerminalSeen() {
		t.Fatalf("text=%q tools=%d", text, tools)
	}
}

func TestMalformedAndFailureEvents(t *testing.T) {
	for _, adapter := range []string{"codex", "claude", "cursor", "generic"} {
		t.Run(adapter, func(t *testing.T) {
			p, _ := New(benchmark.Agent{Adapter: adapter})
			if _, err := p.ParseEvent([]byte(`{"type":`)); err == nil {
				t.Fatal("malformed JSON accepted")
			}
		})
	}
	for _, tc := range []struct{ adapter, line string }{
		{"codex", `{"type":"turn.completed","usage":{"output_tokens":-1}}`},
		{"claude", `{"type":"result","subtype":"success","usage":{"output_tokens":2.5}}`},
		{"generic", `{"type":"usage_reported","usage":{"output_tokens":-1}}`},
	} {
		p, _ := New(benchmark.Agent{Adapter: tc.adapter})
		if _, err := p.ParseEvent([]byte(tc.line)); err == nil {
			t.Fatal(tc)
		}
	}
	p, _ := New(benchmark.Agent{Adapter: "claude"})
	events, err := p.ParseEvent([]byte(`{"type":"result","subtype":"error_max_turns","is_error":true}`))
	if err != nil || len(events) != 1 || events[0].Type != "agent_error" || p.TerminalSeen() {
		t.Fatal(events, err)
	}
}

func TestCommandsPreservePromptAsData(t *testing.T) {
	prompt := "--dangerously-bypass-approvals-and-sandbox $(touch /tmp/nope)\nhello"
	for _, adapter := range []string{"codex", "claude", "cursor"} {
		p, _ := New(benchmark.Agent{Adapter: adapter, Model: "test-model"})
		c, err := p.BuildCommand(prompt, "/workspace")
		if err != nil {
			t.Fatal(err)
		}
		if adapter == "cursor" {
			if c.Args[len(c.Args)-2] != "--" || c.Args[len(c.Args)-1] != prompt {
				t.Fatal(c)
			}
		} else if c.Stdin != prompt {
			t.Fatal(c)
		}
		for _, arg := range c.Args {
			if arg == "--dangerously-skip-permissions" || arg == "--force" || arg == "--dangerously-bypass-approvals-and-sandbox" {
				t.Fatal("unexpected permission bypass")
			}
		}
	}
}

func TestGenericCanonicalAndPlainText(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "generic", Command: "custom", Args: []string{"{prompt}", "{workdir}", "{model}"}, Model: "m"})
	c, _ := p.BuildCommand("hello world", "/a b")
	if c.Args[0] != "hello world" || c.Args[1] != "/a b" {
		t.Fatal(c)
	}
	events, err := p.ParseEvent([]byte("hello"))
	if err != nil || events[0].Text != "hello\n" {
		t.Fatal(events, err)
	}
	events, err = p.ParseEvent([]byte(`{"type":"usage_reported","usage":{"output_tokens":0}}`))
	if err != nil || events[0].Usage.OutputTokens == nil {
		t.Fatal(events, err)
	}
}
