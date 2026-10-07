package adapters

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type agyParser struct {
	agent    benchmark.Agent
	terminal bool
}

func (p *agyParser) BuildCommand(prompt, workdir string) (Command, error) {
	path := p.agent.Command
	if path == "" {
		path = "agy"
	}
	args := []string{"-p", prompt, "--output-format", "stream-json", "--dangerously-skip-permissions"}
	if p.agent.Model != "" {
		args = append(args, "--model", p.agent.Model)
	}
	if p.agent.ReasoningEffort != "" {
		args = append(args, "--effort", p.agent.ReasoningEffort)
	}
	if p.agent.IsolateConfig {
		args = append(args, "--disable-slash-commands")
	}
	return Command{Path: path, Args: args}, nil
}
func (p *agyParser) RequiresTerminal() bool { return true }
func (p *agyParser) TerminalSeen() bool     { return p.terminal }
func (p *agyParser) Capabilities() Capabilities {
	return Capabilities{StructuredOutput: true, TokenUsage: true, ToolIntervals: true, Notes: "Native stream-json deltas and authoritative terminal usage; reported output includes thinking. Tool intervals remain runner receipt gaps."}
}
func (p *agyParser) ParseEvent(line []byte) ([]telemetry.Event, error) {
	var w struct {
		Event string `json:"event"`
		Init  struct {
			Model  string `json:"model"`
			Effort string `json:"effort"`
		} `json:"init"`
		Step struct {
			Index    *int   `json:"step_index"`
			State    string `json:"state"`
			Type     string `json:"step_type"`
			Text     string `json:"text_delta"`
			ToolName string `json:"tool_name"`
		} `json:"step_update"`
		Result struct {
			Status   string                     `json:"status"`
			Response string                     `json:"response"`
			Error    json.RawMessage            `json:"error"`
			Duration *float64                   `json:"duration_seconds"`
			Usage    map[string]json.RawMessage `json:"usage"`
		} `json:"result"`
	}
	if err := json.Unmarshal(line, &w); err != nil {
		return nil, err
	}
	switch w.Event {
	case "init":
		return []telemetry.Event{{Type: "agent_metadata", Model: w.Init.Model, ReasoningEffort: w.Init.Effort}}, nil
	case "step_update":
		s := w.Step
		if s.Type == "agent_response" && s.Text != "" && (s.State == "ACTIVE" || s.State == "DONE") {
			return []telemetry.Event{{Type: "assistant_output", Text: s.Text, TimingBasis: "text_delta_receipt"}}, nil
		}
		if s.Type == "tool" && s.Index != nil && *s.Index >= 0 && (s.State == "ACTIVE" || s.State == "DONE") {
			kind := "tool_started"
			if s.State == "DONE" {
				kind = "tool_finished"
			}
			return []telemetry.Event{{Type: kind, ToolID: strconv.Itoa(*s.Index), ToolName: s.ToolName}}, nil
		}
	case "result":
		if p.terminal {
			return nil, nil
		}
		if w.Result.Status != "SUCCESS" {
			f := ClassifyFailure(w.Result.Error)
			if len(w.Result.Error) == 0 || string(w.Result.Error) == "null" {
				f = ClassifyFailure([]byte("agy terminal status: " + w.Result.Status))
			}
			return []telemetry.Event{{Type: "agent_error", Text: f.Message, Failure: f}}, nil
		}
		u, err := usage(w.Result.Usage)
		if err != nil {
			return nil, err
		}
		if d := w.Result.Duration; d != nil && (math.IsNaN(*d) || math.IsInf(*d, 0) || *d < 0) {
			return nil, fmt.Errorf("invalid agy duration_seconds")
		}
		u.OutputTokenAccounting = "includes_thinking"
		p.terminal = true
		return []telemetry.Event{{Type: "final_output", Text: w.Result.Response, TimingBasis: "complete_message_receipt"}, {Type: "usage_total", Usage: &u}, {Type: "agent_completed", ReportedDurationSeconds: w.Result.Duration, ReportedDurationSource: "agy.result.duration_seconds"}}, nil
	}
	return nil, nil
}
