package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type responsesParser struct {
	agent    benchmark.Agent
	terminal bool
}

func (p *responsesParser) BuildCommand(prompt, workdir string) (Command, error) {
	path, err := os.Executable()
	if err != nil {
		return Command{}, err
	}
	if p.agent.API == nil {
		return Command{}, fmt.Errorf("missing API configuration")
	}
	args := []string{"__responses-agent", "--model", p.agent.Model, "--endpoint", p.agent.API.Endpoint, "--key-env", p.agent.API.KeyEnv, "--max-output-tokens", strconv.Itoa(p.agent.API.MaxOutputTokens)}
	if p.agent.ReasoningEffort != "" {
		args = append(args, "--effort", p.agent.ReasoningEffort)
	}
	return Command{Path: path, Args: args, Stdin: prompt}, nil
}
func (p *responsesParser) RequiresTerminal() bool { return true }
func (p *responsesParser) TerminalSeen() bool     { return p.terminal }
func (p *responsesParser) Capabilities() Capabilities {
	return Capabilities{StructuredOutput: true, TokenUsage: true, Notes: "Response-only HTTP SSE relay; deltas are client receipts, not decoding intervals. No coding tools."}
}
func (p *responsesParser) ParseEvent(line []byte) ([]telemetry.Event, error) {
	var w struct {
		Type     string          `json:"type"`
		Delta    string          `json:"delta"`
		Error    json.RawMessage `json:"error"`
		Response struct {
			Status string                     `json:"status"`
			Model  string                     `json:"model"`
			Error  json.RawMessage            `json:"error"`
			Usage  map[string]json.RawMessage `json:"usage"`
			Output []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal(line, &w); err != nil {
		return nil, err
	}
	switch w.Type {
	case "response.created":
		return []telemetry.Event{{Type: "agent_metadata", Model: w.Response.Model}}, nil
	case "response.output_text.delta":
		if w.Delta != "" {
			return []telemetry.Event{{Type: "assistant_output", Text: w.Delta, TimingBasis: "text_delta_receipt", StreamTransport: "responses_http_sse_relay"}}, nil
		}
	case "response.completed":
		if w.Response.Status != "completed" {
			return nil, fmt.Errorf("invalid completed Response status")
		}
		u, err := usage(w.Response.Usage)
		if err != nil {
			return nil, err
		}
		var details map[string]json.RawMessage
		if raw := w.Response.Usage["output_tokens_details"]; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &details); err != nil {
				return nil, err
			}
			if value, ok := details["reasoning_tokens"]; ok {
				thinking, err := usage(map[string]json.RawMessage{"thinking_tokens": value})
				if err != nil {
					return nil, err
				}
				u.ThinkingTokens = thinking.ThinkingTokens
			}
		}
		if raw := w.Response.Usage["input_tokens_details"]; len(raw) > 0 && string(raw) != "null" {
			details = nil
			if err := json.Unmarshal(raw, &details); err != nil {
				return nil, err
			}
			if value, ok := details["cached_tokens"]; ok {
				cached, err := usage(map[string]json.RawMessage{"cached_input_tokens": value})
				if err != nil {
					return nil, err
				}
				u.CachedTokens = cached.CachedTokens
			}
		}
		u.OutputTokenAccounting = "includes_thinking"
		var answer strings.Builder
		for _, item := range w.Response.Output {
			if item.Type == "message" {
				for _, part := range item.Content {
					if part.Type == "output_text" {
						answer.WriteString(part.Text)
					}
				}
			}
		}
		p.terminal = true
		return []telemetry.Event{{Type: "final_output", Text: answer.String(), TimingBasis: "complete_message_receipt"}, {Type: "usage_total", Usage: &u}, {Type: "agent_metadata", Model: w.Response.Model}, {Type: "agent_completed"}}, nil
	case "response.failed", "response.incomplete", "error":
		raw := w.Error
		if len(raw) == 0 {
			raw = w.Response.Error
		}
		if len(raw) == 0 || string(raw) == "null" {
			raw = []byte(`{"message":"Responses request did not complete"}`)
		}
		return []telemetry.Event{{Type: "agent_error", Failure: ClassifyFailure(raw)}}, nil
	}
	return nil, nil
}
