package telemetry

import "time"

// Usage preserves missing fields rather than interpreting them as zero.
type Usage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	CachedTokens *int64 `json:"cached_input_tokens"`
}

type Event struct {
	RunID       string `json:"run_id,omitempty"`
	Agent       string `json:"agent,omitempty"`
	TimestampNS int64  `json:"timestamp_ns"`
	ElapsedNS   int64  `json:"elapsed_ns"`
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	ToolID      string `json:"tool_id,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	Usage       *Usage `json:"usage,omitempty"`
}

type RawLine struct {
	TimestampNS int64  `json:"timestamp_ns"`
	ElapsedNS   int64  `json:"elapsed_ns"`
	Stream      string `json:"stream"`
	Line        string `json:"line"`
}

type Metrics struct {
	WallSeconds            float64  `json:"wall_seconds"`
	FirstStdoutSeconds     *float64 `json:"first_stdout_line_seconds"`
	TTFASeconds            *float64 `json:"ttfa_seconds"`
	EffectiveOutputTPS     *float64 `json:"effective_output_tokens_per_second"`
	GenerationTPS          *float64 `json:"generation_tokens_per_second"`
	ModelActiveTPS         *float64 `json:"model_active_tokens_per_second"`
	Usage                  Usage    `json:"usage"`
	ToolCalls              int      `json:"tool_calls"`
	MatchedToolCalls       int      `json:"matched_tool_calls"`
	ToolLatencyMeanSeconds *float64 `json:"tool_latency_mean_seconds"`
	ToolLatencyP50Seconds  *float64 `json:"tool_latency_p50_seconds"`
	ToolLatencyP95Seconds  *float64 `json:"tool_latency_p95_seconds"`
	Warnings               []string `json:"warnings,omitempty"`
}

type Run struct {
	ID           string    `json:"id"`
	ExperimentID string    `json:"experiment_id"`
	Case         string    `json:"case"`
	Agent        string    `json:"agent"`
	Adapter      string    `json:"adapter"`
	Model        string    `json:"model,omitempty"`
	Repeat       int       `json:"repeat"`
	StartedAt    time.Time `json:"started_at"`
	Status       string    `json:"status"`
	ExitCode     *int      `json:"exit_code"`
	Success      *bool     `json:"success"`
	Error        string    `json:"error,omitempty"`
	Commit       string    `json:"commit,omitempty"`
	ArtifactDir  string    `json:"artifact_dir"`
	Metrics      Metrics   `json:"metrics"`
}
