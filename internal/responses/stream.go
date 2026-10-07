// Package responses relays HTTP SSE without retries or credential persistence.
package responses

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Request struct {
	Model           string     `json:"model"`
	Input           string     `json:"input"`
	Stream          bool       `json:"stream"`
	Store           bool       `json:"store"`
	MaxOutputTokens int        `json:"max_output_tokens"`
	Reasoning       *Reasoning `json:"reasoning,omitempty"`
}
type Reasoning struct {
	Effort string `json:"effort"`
}

func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid Responses endpoint")
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return fmt.Errorf("Responses endpoint requires HTTPS; loopback HTTP is allowed for tests")
	}
	return nil
}

func Stream(ctx context.Context, endpoint, key string, request Request, output io.Writer) error {
	if err := ValidateEndpoint(endpoint); err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("authentication required")
	}
	if request.Model == "" || request.MaxOutputTokens < 1 {
		return fmt.Errorf("model and output token limit are required")
	}
	request.Stream = true
	request.Store = false
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid API request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Responses transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		failure := map[string]any{"type": "error", "error": map[string]any{"status": response.StatusCode, "code": "http_error", "message": "Responses HTTP request failed"}}
		if err := json.NewEncoder(output).Encode(failure); err != nil {
			return err
		}
		return fmt.Errorf("Responses HTTP status %d", response.StatusCode)
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		return fmt.Errorf("Responses server did not return SSE")
	}
	return Relay(response.Body, output)
}

// Relay supports multiline SSE data, heartbeats, CRLF and bounded events.
func Relay(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 8*1024*1024)
	var data strings.Builder
	terminal := false
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		payload := strings.TrimSuffix(data.String(), "\n")
		data.Reset()
		if payload == "[DONE]" {
			return nil
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return fmt.Errorf("malformed SSE data")
		}
		compact := new(bytes.Buffer)
		if err := json.Compact(compact, []byte(payload)); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, compact.String()); err != nil {
			return err
		}
		if event.Type == "response.completed" {
			terminal = true
		}
		if event.Type == "response.failed" || event.Type == "response.incomplete" || event.Type == "error" {
			return fmt.Errorf("Responses request did not complete")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			if terminal {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			if data.Len()+len(value)+1 > 8*1024*1024 {
				return fmt.Errorf("SSE event exceeds limit")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE stream read failed")
	}
	if err := flush(); err != nil {
		return err
	}
	if !terminal {
		return fmt.Errorf("SSE stream ended without response.completed")
	}
	return nil
}
