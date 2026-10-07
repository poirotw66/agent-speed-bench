package adapters

import (
	"strings"
	"testing"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
)

func TestNestedFailureClassification(t *testing.T) {
	for _, raw := range []string{
		`{"type":"error","message":"{\"type\":\"error\",\"status\":400,\"error\":{\"message\":\"The model is not supported with this account\"}}"}`,
		`{"type":"turn.failed","error":{"message":"The model is not supported with this account"}}`,
	} {
		f := ClassifyFailure([]byte(raw))
		if f.Category != "configuration" || f.Scope != "agent" || f.Retryable || f.Message != "The model is not supported with this account" {
			t.Fatal(f)
		}
	}
	f := ClassifyFailure([]byte(`{"error":{"code":"invalid_api_key","message":"Bad credential"},"status":401}`))
	if f.Category != "authentication" || f.Retryable {
		t.Fatal(f)
	}
	f = ClassifyFailure([]byte(`{"status":429,"error":{"message":"Busy"}}`))
	if f.Category != "service" || !f.Retryable || f.Scope == "agent" {
		t.Fatal(f)
	}
	f = ClassifyFailure([]byte(`{"error":{"message":"Task failed"}}`))
	if f.Scope == "agent" || f.Message != "Task failed" {
		t.Fatal(f)
	}
}

func TestReasoningEffortPassedAsCLIOverride(t *testing.T) {
	p, _ := New(benchmark.Agent{Adapter: "codex", ReasoningEffort: "high"})
	command, err := p.BuildCommand("prompt", "workspace")
	if err != nil || !strings.Contains(strings.Join(command.Args, "|"), `--config|model_reasoning_effort="high"`) {
		t.Fatal(command, err)
	}
}
