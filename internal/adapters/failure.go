package adapters

import (
	"encoding/json"
	"strings"

	"github.com/poirotw66/agent-speed-bench/internal/telemetry"
)

// ClassifyFailure follows only error envelope fields, including JSON encoded messages.
func ClassifyFailure(data []byte) *telemetry.Failure {
	f := &telemetry.Failure{Category: "agent", Code: "unknown", Scope: "task", Retryable: true, Message: string(data)}
	var walk func(json.RawMessage, int)
	walk = func(raw json.RawMessage, depth int) {
		if depth > 8 || strings.TrimSpace(string(raw)) == "null" {
			return
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			f.Message = text
			if json.Valid([]byte(text)) {
				walk([]byte(text), depth+1)
			}
			return
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			return
		}
		for _, key := range []string{"status", "status_code"} {
			var status int
			if json.Unmarshal(obj[key], &status) == nil && status >= 400 {
				f.HTTPStatus = &status
			}
		}
		var errorType string
		if json.Unmarshal(obj["type"], &errorType) == nil {
			switch errorType {
			case "authentication_error", "invalid_api_key", "model_not_found", "model_not_supported":
				f.Code = errorType
			}
		}
		var code string
		if json.Unmarshal(obj["code"], &code) == nil && code != "" {
			f.Code = code
		}
		for _, key := range []string{"message", "error"} {
			if raw := obj[key]; len(raw) > 0 {
				walk(raw, depth+1)
			}
		}
	}
	walk(data, 0)
	msg := strings.ToLower(f.Message)
	switch {
	case telemetry.IsUsageLimit(f.Code, f.Message):
		f.Category, f.Scope, f.Retryable = "quota", "agent", false
		if f.Code == "unknown" {
			f.Code = "usage_limit_reached"
		}
	case telemetry.IsConnectionFailure(f.Code, f.Message):
		f.Category, f.Code, f.Scope, f.Retryable = "network", "connection_error", "attempt", true
	case f.Code == "model_not_found" || f.Code == "model_not_supported" || (strings.Contains(msg, "model") && (strings.Contains(msg, "not supported") || strings.Contains(msg, "does not exist"))):
		f.Category, f.Code, f.Scope, f.Retryable = "configuration", "model_not_supported", "agent", false
	case (f.HTTPStatus != nil && *f.HTTPStatus == 401) || f.Code == "invalid_api_key" || f.Code == "authentication_error" || strings.Contains(msg, "not logged in") || strings.Contains(msg, "authentication required") || strings.Contains(msg, "please log in") || strings.Contains(msg, "run codex login"):
		f.Category, f.Scope, f.Retryable = "authentication", "agent", false
	case f.Code == "rate_limit_exceeded" || f.Code == "slow_down" || f.Code == "server_error" || (f.HTTPStatus != nil && (*f.HTTPStatus == 429 || *f.HTTPStatus >= 500)):
		f.Category = "service"
	}
	return f
}
