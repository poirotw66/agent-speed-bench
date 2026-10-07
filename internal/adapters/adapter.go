package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/benchmark"
	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

type Command struct {
	Path  string
	Args  []string
	Stdin string
	Env   []string
}
type Capabilities struct {
	StructuredOutput bool   `json:"structured_output"`
	TokenUsage       bool   `json:"token_usage"`
	ToolIntervals    bool   `json:"tool_intervals"`
	GenerationTiming bool   `json:"generation_timing"`
	Notes            string `json:"notes"`
}
type Adapter interface {
	BuildCommand(prompt, workdir string) (Command, error)
	ParseEvent(line []byte) ([]telemetry.Event, error)
	Capabilities() Capabilities
	RequiresTerminal() bool
	TerminalSeen() bool
}

type parser struct {
	agent    benchmark.Agent
	terminal bool
	streamed bool
}

func New(a benchmark.Agent) (Adapter, error) {
	switch a.Adapter {
	case "openai-responses":
		return &responsesParser{agent: a}, nil
	case "agy":
		return &agyParser{agent: a}, nil
	case "codex", "claude", "cursor", "generic", "demo":
		return &parser{agent: a}, nil
	default:
		return nil, fmt.Errorf("unsupported adapter: %s", a.Adapter)
	}
}

func (p *parser) BuildCommand(prompt, workdir string) (Command, error) {
	c := Command{Path: p.agent.Command}
	switch p.agent.Adapter {
	case "codex":
		if c.Path == "" {
			c.Path = "codex"
		}
		c.Args = []string{"exec", "--json", "--ephemeral", "--yolo", "--skip-git-repo-check", "--color", "never"}
		if p.agent.IsolateConfig {
			c.Args = append(c.Args, "--ignore-user-config", "--ignore-rules")
			c.Args = append(c.Args, "--config", "project_doc_max_bytes=0")
			for _, feature := range []string{"memories", "plugins", "apps", "browser_use", "computer_use"} {
				c.Args = append(c.Args, "--disable", feature)
			}
		}
		if p.agent.Model != "" {
			c.Args = append(c.Args, "--model", p.agent.Model)
		}
		if p.agent.ReasoningEffort != "" {
			c.Args = append(c.Args, "--config", "model_reasoning_effort="+strconv.Quote(p.agent.ReasoningEffort))
		}
		if p.agent.ServiceTier != "" {
			c.Args = append(c.Args, "--config", "service_tier="+strconv.Quote(p.agent.ServiceTier))
			if p.agent.ServiceTier == "fast" {
				c.Args = append(c.Args, "--enable", "fast_mode")
			}
		}
		c.Args = append(c.Args, "-")
		c.Stdin = prompt
	case "claude":
		if c.Path == "" {
			c.Path = "claude"
		}
		c.Args = []string{"--print", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--dangerously-skip-permissions"}
		if p.agent.Model != "" {
			c.Args = append(c.Args, "--model", p.agent.Model)
		}
		c.Stdin = prompt
	case "cursor":
		if c.Path == "" {
			c.Path = "agent"
		}
		c.Args = []string{"--print", "--output-format", "stream-json", "--stream-partial-output", "--yolo", "--sandbox", "disabled"}
		if p.agent.Model != "" {
			c.Args = append(c.Args, "--model", p.agent.Model)
		}
		if p.agent.TrustWorkspace {
			c.Args = append(c.Args, "--trust")
		}
		c.Args = append(c.Args, "--", prompt)
	case "demo":
		var err error
		c.Path, err = os.Executable()
		if err != nil {
			return c, err
		}
		c.Args = []string{"__demo-agent"}
		c.Stdin = prompt
	case "generic":
		c.Args = make([]string, len(p.agent.Args))
		r := strings.NewReplacer("{prompt}", prompt, "{model}", p.agent.Model, "{workdir}", workdir)
		for i, arg := range p.agent.Args {
			c.Args[i] = r.Replace(arg)
		}
		c.Stdin = prompt
	}
	return c, nil
}

func (p *parser) Capabilities() Capabilities {
	switch p.agent.Adapter {
	case "codex":
		return Capabilities{StructuredOutput: true, TokenUsage: true, ToolIntervals: true, Notes: "Completed message items are buffered; tool intervals are runner receipt gaps, not execution durations."}
	case "claude":
		return Capabilities{StructuredOutput: true, TokenUsage: true, ToolIntervals: true, Notes: "Partial text deltas; tool_use to tool_result receipt gaps do not establish runtime. Final result usage is authoritative."}
	case "cursor":
		return Capabilities{StructuredOutput: true, TokenUsage: true, ToolIntervals: true, Notes: "Partial text deltas with duplicate flush filtering; final result usage is authoritative when supplied."}
	case "demo":
		return Capabilities{StructuredOutput: true, TokenUsage: true, ToolIntervals: true, Notes: "Synthetic events and tokens; never compare demo results with real agents."}
	default:
		return Capabilities{StructuredOutput: true, Notes: "Canonical JSONL telemetry is optional. Plain stdout is captured but provides no token usage."}
	}
}
func (p *parser) RequiresTerminal() bool { return p.agent.Adapter != "generic" }
func (p *parser) TerminalSeen() bool     { return p.terminal }

// These structs intentionally ignore unknown fields for forward compatibility.
type wire struct {
	DurationMS  *float64                   `json:"duration_ms"`
	ServiceTier string                     `json:"service_tier"`
	Model       string                     `json:"model"`
	Type        string                     `json:"type"`
	Subtype     string                     `json:"subtype"`
	IsError     bool                       `json:"is_error"`
	Message     json.RawMessage            `json:"message"`
	Result      string                     `json:"result"`
	Usage       map[string]json.RawMessage `json:"usage"`
	Item        struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Text    string `json:"text"`
		Command string `json:"command"`
	} `json:"item"`
	Event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"`
	CallID          string                     `json:"call_id"`
	ToolCall        map[string]json.RawMessage `json:"tool_call"`
	TimestampMS     *int64                     `json:"timestamp_ms"`
	ModelCallID     string                     `json:"model_call_id"`
	ParentToolUseID *string                    `json:"parent_tool_use_id"`
}
type content struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
}

func (p *parser) ParseEvent(line []byte) ([]telemetry.Event, error) {
	if p.agent.Adapter == "generic" || p.agent.Adapter == "demo" {
		var e telemetry.Event
		if err := json.Unmarshal(line, &e); err != nil {
			if p.agent.Adapter == "generic" && !bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
				return []telemetry.Event{{Type: "assistant_output", Text: string(line) + "\n", TimingBasis: "stdout_line_receipt"}}, nil
			}
			return nil, err
		}
		switch e.Type {
		case "assistant_output", "tool_started", "tool_finished", "usage_reported", "usage_total", "agent_ready", "agent_metadata", "assistant_message_receipt":
			if d := e.ReportedDurationSeconds; d != nil && (*d < 0 || math.IsNaN(*d) || math.IsInf(*d, 0)) {
				return nil, fmt.Errorf("invalid reported duration")
			}
			if e.Usage != nil {
				for _, v := range []*int64{e.Usage.InputTokens, e.Usage.OutputTokens, e.Usage.CachedTokens, e.Usage.ThinkingTokens, e.Usage.CacheWriteTokens} {
					if v != nil && *v < 0 {
						return nil, fmt.Errorf("negative usage")
					}
				}
			}
			if e.Type == "assistant_output" && e.TimingBasis != "text_delta_receipt" && e.TimingBasis != "complete_message_receipt" {
				e.TimingBasis = "stdout_line_receipt"
			}
			if e.Type == "tool_started" || e.Type == "tool_finished" {
				e.TimingBasis = "tool_start_receipt"
			}
			return []telemetry.Event{e}, nil
		case "agent_completed":
			p.terminal = true
			return []telemetry.Event{{Type: "agent_completed"}}, nil
		case "agent_error":
			f := ClassifyFailure([]byte(e.Text))
			return []telemetry.Event{{Type: "agent_error", Text: f.Message, Failure: f}}, nil
		default:
			return nil, nil
		}
	}
	var w wire
	if err := json.Unmarshal(line, &w); err != nil {
		return nil, err
	}
	if w.Type == "error" || w.Type == "turn.failed" || w.IsError {
		f := ClassifyFailure(line)
		return []telemetry.Event{{Type: "agent_error", Text: f.Message, Failure: f}}, nil
	}
	var events []telemetry.Event
	if w.ServiceTier != "" {
		events = append(events, telemetry.Event{Type: "agent_metadata", ServiceTier: w.ServiceTier})
	}
	if w.Model != "" {
		events = append(events, telemetry.Event{Type: "agent_metadata", Model: w.Model})
	}
	switch p.agent.Adapter {
	case "codex":
		switch w.Type {
		case "turn.started":
			p.terminal = false
		case "item.started", "item.completed":
			switch w.Item.Type {
			case "agent_message":
				if w.Type == "item.completed" && w.Item.Text != "" {
					events = append(events, telemetry.Event{Type: "assistant_output", Text: w.Item.Text, TimingBasis: "complete_message_receipt"})
				}
			case "command_execution", "mcp_tool_call", "web_search":
				t := "tool_started"
				if w.Type == "item.completed" {
					t = "tool_finished"
				}
				events = append(events, telemetry.Event{Type: t, TimingBasis: "tool_start_receipt", ToolID: w.Item.ID, ToolName: w.Item.Type})
			}
		case "turn.completed":
			p.terminal = true
			u, err := usage(w.Usage)
			if err != nil {
				return nil, err
			}
			events = append(events, telemetry.Event{Type: "usage_reported", Usage: &u}, telemetry.Event{Type: "agent_completed"})
		}
	case "claude", "cursor":
		if w.ParentToolUseID != nil {
			return nil, nil
		}
		switch w.Type {
		case "stream_event":
			if w.Event.Type == "content_block_delta" && w.Event.Delta.Type == "text_delta" && w.Event.Delta.Text != "" {
				p.streamed = true
				events = append(events, telemetry.Event{Type: "assistant_output", Text: w.Event.Delta.Text, TimingBasis: "text_delta_receipt"})
			}
		case "assistant", "user":
			var msg struct {
				Content []content `json:"content"`
				Model   string    `json:"model"`
			}
			if len(w.Message) > 0 {
				if err := json.Unmarshal(w.Message, &msg); err != nil {
					return nil, err
				}
			}
			if msg.Model != "" {
				events = append(events, telemetry.Event{Type: "agent_metadata", Model: msg.Model})
			}
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					if w.Type == "assistant" && p.agent.Adapter == "claude" && block.Text != "" {
						events = append(events, telemetry.Event{Type: "assistant_message_receipt", TimingBasis: "complete_message_receipt"})
					}
					valid := w.Type == "assistant" && !p.streamed
					if p.agent.Adapter == "cursor" {
						valid = w.Type == "assistant" && w.TimestampMS != nil && w.ModelCallID == ""
					}
					if valid && block.Text != "" {
						basis := "complete_message_receipt"
						if p.agent.Adapter == "cursor" {
							basis = "text_delta_receipt"
						}
						events = append(events, telemetry.Event{Type: "assistant_output", Text: block.Text, TimingBasis: basis})
					}
				case "tool_use":
					if p.agent.Adapter == "claude" {
						events = append(events, telemetry.Event{Type: "tool_started", ToolID: block.ID, ToolName: block.Name})
					}
				case "tool_result":
					if p.agent.Adapter == "claude" {
						events = append(events, telemetry.Event{Type: "tool_finished", ToolID: block.ToolUseID})
					}
				}
			}
		case "tool_call":
			if p.agent.Adapter == "cursor" && (w.Subtype == "started" || w.Subtype == "completed") {
				t := "tool_started"
				if w.Subtype == "completed" {
					t = "tool_finished"
				}
				name := "tool"
				for k := range w.ToolCall {
					name = k
					break
				}
				events = append(events, telemetry.Event{Type: t, TimingBasis: "tool_start_receipt", ToolID: w.CallID, ToolName: name})
			}
		case "result":
			if w.Subtype != "success" {
				f := ClassifyFailure(line)
				if w.Result != "" {
					f = ClassifyFailure([]byte(w.Result))
				}
				return []telemetry.Event{{Type: "agent_error", Text: f.Message, Failure: f}}, nil
			}
			if w.DurationMS != nil && *w.DurationMS < 0 {
				return nil, fmt.Errorf("invalid duration_ms")
			}
			p.terminal = true
			// Final result text is an authoritative transcript for grading, not a new delta.
			events = append(events, telemetry.Event{Type: "final_output", Text: w.Result}, telemetry.Event{Type: "agent_completed"})
			if w.DurationMS != nil {
				seconds := *w.DurationMS / 1000
				events = append(events, telemetry.Event{Type: "agent_duration_reported", ReportedDurationSeconds: &seconds, ReportedDurationSource: p.agent.Adapter + ".result.duration_ms"})
			}
			if w.Usage != nil {
				if p.agent.Adapter == "cursor" {
					w.Usage = cursorUsage(w.Usage)
				}
				u, err := usage(w.Usage)
				if err != nil {
					return nil, err
				}
				events = append(events, telemetry.Event{Type: "usage_total", Usage: &u})
			}
		}
	}
	return events, nil
}

func usage(m map[string]json.RawMessage) (telemetry.Usage, error) {
	var u telemetry.Usage
	for _, field := range []struct {
		name string
		dst  **int64
	}{{"reasoning_output_tokens", &u.ThinkingTokens}, {"cache_write_input_tokens", &u.CacheWriteTokens}, {"thinking_tokens", &u.ThinkingTokens}, {"cache_write_tokens", &u.CacheWriteTokens}, {"cache_read_tokens", &u.CachedTokens}, {"input_tokens", &u.InputTokens}, {"output_tokens", &u.OutputTokens}, {"cached_input_tokens", &u.CachedTokens}, {"cache_read_input_tokens", &u.CachedTokens}} {
		raw, ok := m[field.name]
		if !ok || string(raw) == "null" {
			continue
		}
		var v int64
		if err := json.Unmarshal(raw, &v); err != nil || v < 0 {
			return u, fmt.Errorf("invalid usage field %s", field.name)
		}
		if *field.dst != nil && **field.dst != v {
			return u, fmt.Errorf("conflicting usage alias %s", field.name)
		}
		*field.dst = &v
	}
	return u, nil
}

// Cursor uses camelCase names in its authoritative terminal usage object.
func cursorUsage(m map[string]json.RawMessage) map[string]json.RawMessage {
	n := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		n[k] = v
	}
	for from, to := range map[string]string{"inputTokens": "input_tokens", "outputTokens": "output_tokens", "cacheReadTokens": "cached_input_tokens", "cacheWriteTokens": "cache_write_tokens"} {
		if v, ok := m[from]; ok {
			n[to] = v
		}
	}
	return n
}

// DiagnosticFailure recognizes an actionable stderr blocker, not arbitrary warnings.
func DiagnosticFailure(a Adapter, line []byte) *telemetry.Failure {
	message := strings.ToLower(strings.TrimSpace(string(line)))
	if _, ok := a.(*agyParser); ok && strings.HasPrefix(message, "jetski: no output produced") && strings.Contains(message, "permission") && strings.Contains(message, "auto-denied") {
		return &telemetry.Failure{Category: "permission", Code: "headless_tool_permission_denied", Scope: "agent", Retryable: false, Message: "agy headless tool permission was denied; configure sandbox permissions before benchmarking coding tasks"}
	}
	if strings.HasPrefix(message, "error: authentication required") {
		return &telemetry.Failure{Category: "authentication", Code: "authentication_required", Scope: "agent", Retryable: false, Message: "CLI authentication is required"}
	}
	p, ok := a.(*parser)
	if ok && p.agent.Adapter == "cursor" && strings.TrimSpace(string(line)) == "⚠ Workspace Trust Required" {
		return &telemetry.Failure{Category: "permission", Code: "workspace_trust_required", Scope: "agent", Retryable: false, Message: "Cursor workspace trust is required; enable trust_workspace only for an authorized benchmark workspace"}
	}
	return nil
}
