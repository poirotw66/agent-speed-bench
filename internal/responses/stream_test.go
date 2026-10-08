package responses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamingRequestAndRelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TEST_PRIVATE_KEY" {
			t.Error("missing authentication")
		}
		var body Request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.Stream || body.Store || body.MaxOutputTokens != 30 {
			t.Error("invalid request", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io := ": heartbeat\r\nevent: response.output_text.delta\r\ndata: {\"type\":\"response.output_text.delta\",\r\ndata: \"delta\":\"hello\"}\r\n\r\ndata: {\"type\":\"response.completed\"}\r\n\r\n"
		w.Write([]byte(io))
	}))
	defer server.Close()
	var out bytes.Buffer
	err := Stream(context.Background(), server.URL, "TEST_PRIVATE_KEY", Request{Model: "test-model", MaxOutputTokens: 30}, &out)
	if err != nil || !strings.Contains(out.String(), `"delta":"hello"`) || strings.Contains(out.String(), "TEST_PRIVATE_KEY") {
		t.Fatal(out.String(), err)
	}
}

func TestRelayRejectsIncompleteAndMalformedStreams(t *testing.T) {
	for _, input := range []string{`data: {"type":"response.output_text.delta","delta":"x"}` + "\n\n", "data: invalid\n\n", `data: {"type":"response.incomplete"}` + "\n\n"} {
		if err := Relay(strings.NewReader(input), &bytes.Buffer{}); err == nil {
			t.Fatal("invalid stream accepted")
		}
	}
	for _, endpoint := range []string{"http://example.com/v1/responses", "https://user:password@example.com/v1/responses", "https://example.com/?key=secret"} {
		if ValidateEndpoint(endpoint) == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}

func TestHTTPFailureDoesNotExposeCredentialsOrBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("TEST_PRIVATE_KEY")) }))
	defer server.Close()
	var out bytes.Buffer
	err := Stream(context.Background(), server.URL, "TEST_PRIVATE_KEY", Request{Model: "test", MaxOutputTokens: 1}, &out)
	if err == nil || strings.Contains(out.String(), "TEST_PRIVATE_KEY") || !strings.Contains(out.String(), `"status":401`) {
		t.Fatal(out.String(), err)
	}
}

func TestHTTPFailurePreservesOnlyRecognizedCodes(t *testing.T) {
	for _, code := range []string{"insufficient_quota", "project_spend_limit_exceeded", "model_not_found", "rate_limit_exceeded", "SECRET_PROVIDER_VALUE"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "TEST_PRIVATE_KEY"}})
		}))
		var out bytes.Buffer
		err := Stream(context.Background(), server.URL, "TEST_PRIVATE_KEY", Request{Model: "test", MaxOutputTokens: 1}, &out)
		server.Close()
		if err == nil || strings.Contains(out.String(), "TEST_PRIVATE_KEY") || strings.Contains(out.String(), "SECRET_PROVIDER_VALUE") {
			t.Fatal(out.String(), err)
		}
		if code != "SECRET_PROVIDER_VALUE" && !strings.Contains(out.String(), code) {
			t.Fatal("recognized code lost", out.String())
		}
	}
	for _, body := range []string{"invalid", strings.Repeat("x", 64*1024+1), `{"error":{"type":"insufficient_quota","code":"unrecognized"}}`} {
		got := safeHTTPErrorCode(strings.NewReader(body))
		want := "http_error"
		if strings.Contains(body, "insufficient_quota") {
			want = "insufficient_quota"
		}
		if got != want {
			t.Fatal(got, want)
		}
	}
}
