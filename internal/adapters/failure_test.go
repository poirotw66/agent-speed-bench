package adapters

import (
	"strconv"
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

func TestQuotaIsUngradedAndStopsFutureCalls(t *testing.T) {
	for _, raw := range []string{`{"message":"You have hit your usage limit. Try again later."}`, `{"status":429,"error":{"code":"insufficient_quota"}}`} {
		f := ClassifyFailure([]byte(raw))
		if f.Category != "quota" || (f.Code != "usage_limit_reached" && f.Code != "insufficient_quota") || f.Scope != "agent" || f.Retryable {
			t.Fatal(f)
		}
	}
}

func TestAgyHeadlessPermissionDiagnostic(t *testing.T) {
	agy, _ := New(benchmark.Agent{Adapter: "agy"})
	line := []byte(`jetski: no output produced — a tool required the "command" permission that headless mode cannot prompt for, so it was auto-denied.`)
	f := DiagnosticFailure(agy, line)
	if f == nil || f.Category != "permission" || f.Scope != "agent" || f.Retryable {
		t.Fatal(f)
	}
	if DiagnosticFailure(agy, []byte("A tool permission was configured")) != nil {
		t.Fatal("ordinary warning classified as blocker")
	}
}

func TestExplicitNetworkFailuresRemainUngradedAndRetryable(t *testing.T) {
	for _, message := range []string{"There was a network issue connecting to the server, please try again.", "Reconnecting... (stream disconnected before completion: Connection reset by peer)", "API error: read tcp: connection reset by peer"} {
		f := ClassifyFailure([]byte(strconv.Quote(message)))
		if f.Category != "network" || f.Scope != "attempt" || !f.Retryable {
			t.Fatal(f)
		}
	}
	if f := ClassifyFailure([]byte(`{"message":"Task needs network configuration changes"}`)); f.Category == "network" {
		t.Fatal("ordinary task failure treated as a transport error")
	}
}
