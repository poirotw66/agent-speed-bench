package adapters

import (
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

func TestResponsesUsageAndTranscript(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "openai-responses"})
	events, err := p.ParseEvent([]byte(`{"type":"response.completed","response":{"status":"completed","model":"test","output":[{"type":"message","content":[{"type":"output_text","text":"Done"}]}],"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":5}}}}`))
	if err != nil || !p.TerminalSeen() {
		t.Fatal(events, err)
	}
	m := telemetry.Calculate(events, 2, nil)
	if m.Usage.ThinkingTokens == nil || *m.Usage.ThinkingTokens != 5 || m.Usage.CachedTokens == nil || *m.Usage.CachedTokens != 0 || m.GenerationTPS != nil {
		t.Fatal(m)
	}
	p, _ = New(benchmark.Agent{Adapter: "openai-responses"})
	events, err = p.ParseEvent([]byte(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens_details":{}}}}`))
	if err != nil || telemetry.Calculate(events, 1, nil).Usage.OutputTokens != nil {
		t.Fatal(events, err)
	}
	if _, err = p.ParseEvent([]byte(`{"type":"response.completed","response":{"status":"completed","usage":{"output_tokens_details":{"reasoning_tokens":-1}}}}`)); err == nil {
		t.Fatal("negative reasoning accepted")
	}
}
