package telemetry

import "time"

// Usage preserves missing fields rather than interpreting them as zero.
type Usage struct {
	ThinkingTokens        *int64 `json:"thinking_tokens"`
	CacheWriteTokens      *int64 `json:"cache_write_tokens"`
	OutputTokenAccounting string `json:"output_token_accounting,omitempty"`
	InputTokens           *int64 `json:"input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	CachedTokens          *int64 `json:"cached_input_tokens"`
}

type Event struct {
	ServiceTier             string   `json:"service_tier,omitempty"`
	ReportedDurationSeconds *float64 `json:"reported_duration_seconds,omitempty"`
	ReportedDurationSource  string   `json:"reported_duration_source,omitempty"`
	RunID                   string   `json:"run_id,omitempty"`
	Agent                   string   `json:"agent,omitempty"`
	TimestampNS             int64    `json:"timestamp_ns"`
	ElapsedNS               int64    `json:"elapsed_ns"`
	Type                    string   `json:"type"`
	Text                    string   `json:"text,omitempty"`
	ToolID                  string   `json:"tool_id,omitempty"`
	ToolName                string   `json:"tool_name,omitempty"`
	Usage                   *Usage   `json:"usage,omitempty"`
	TimingBasis             string   `json:"timing_basis,omitempty"`
	Failure                 *Failure `json:"failure,omitempty"`
	Model                   string   `json:"model,omitempty"`
	ReasoningEffort         string   `json:"reasoning_effort,omitempty"`
}

// Failure separates availability and infrastructure from task correctness.
type Failure struct {
	Category   string `json:"category"`
	Code       string `json:"code"`
	Scope      string `json:"scope"`
	Retryable  bool   `json:"retryable"`
	Message    string `json:"message"`
	HTTPStatus *int   `json:"http_status,omitempty"`
}

type Settings struct {
	RequestedServiceTier      string  `json:"requested_service_tier,omitempty"`
	ConfiguredServiceTier     *string `json:"configured_service_tier"`
	ObservedServiceTier       *string `json:"observed_service_tier"`
	ConfiguredFastMode        *bool   `json:"configured_fast_mode"`
	RequestedModel            string  `json:"requested_model,omitempty"`
	ConfiguredModel           *string `json:"configured_model"`
	ObservedModel             *string `json:"observed_model"`
	RequestedReasoningEffort  string  `json:"requested_reasoning_effort,omitempty"`
	ConfiguredReasoningEffort *string `json:"configured_reasoning_effort"`
	ObservedReasoningEffort   *string `json:"observed_reasoning_effort"`
	ConfigSource              string  `json:"config_source,omitempty"`
	ConfigStatus              string  `json:"config_status"`
}

type RawLine struct {
	TimestampNS int64  `json:"timestamp_ns"`
	ElapsedNS   int64  `json:"elapsed_ns"`
	Stream      string `json:"stream"`
	Line        string `json:"line"`
}

type Metrics struct {
	ReportedDurationSeconds        *float64 `json:"reported_duration_seconds"`
	ReportedDurationSource         string   `json:"reported_duration_source,omitempty"`
	SchemaVersion                  int      `json:"schema_version"`
	WallSeconds                    float64  `json:"wall_seconds"`
	FirstStdoutSeconds             *float64 `json:"first_stdout_line_seconds"`
	TTFASeconds                    *float64 `json:"ttfa_seconds"`
	TTFABasis                      string   `json:"ttfa_timing_basis"`
	FirstTextDeltaSeconds          *float64 `json:"first_text_delta_seconds"`
	FirstCompleteMessageSeconds    *float64 `json:"first_complete_message_seconds"`
	FirstToolActionSeconds         *float64 `json:"first_tool_action_seconds"`
	EffectiveOutputTPS             *float64 `json:"effective_output_tokens_per_second"`
	GenerationTPS                  *float64 `json:"generation_tokens_per_second"`
	ModelActiveTPS                 *float64 `json:"model_active_tokens_per_second"`
	Usage                          Usage    `json:"usage"`
	ToolCalls                      int      `json:"tool_calls"`
	MatchedToolCalls               int      `json:"matched_tool_calls"`
	ToolLatencyMeanSeconds         *float64 `json:"tool_latency_mean_seconds"`
	ToolLatencyP50Seconds          *float64 `json:"tool_latency_p50_seconds"`
	ToolLatencyP95Seconds          *float64 `json:"tool_latency_p95_seconds"`
	ToolTimingBasis                string   `json:"tool_timing_basis"`
	ToolTimingConfidence           string   `json:"tool_timing_confidence"`
	ToolReceiptIntervalMeanSeconds *float64 `json:"tool_receipt_interval_mean_seconds"`
	ToolReceiptIntervalP50Seconds  *float64 `json:"tool_receipt_interval_p50_seconds"`
	ToolReceiptIntervalP95Seconds  *float64 `json:"tool_receipt_interval_p95_seconds"`
	Warnings                       []string `json:"warnings,omitempty"`
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
	Failure      *Failure  `json:"failure,omitempty"`
	Settings     Settings  `json:"settings"`
	Commit       string    `json:"commit,omitempty"`
	ArtifactDir  string    `json:"artifact_dir"`
	Metrics      Metrics   `json:"metrics"`
}
