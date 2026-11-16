package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeAPI serves one canned JSON body and records the request.
func fakeAPI(t *testing.T, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	got := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(context.Background())
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func check(t *testing.T, res ChatResult, err error, in, out int) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hi" || res.InputTokens != in || res.OutputTokens != out {
		t.Errorf("result = %+v, want text hi, tokens %d/%d", res, in, out)
	}
}

func TestAnthropicBaseURLAndUsage(t *testing.T) {
	srv, req := fakeAPI(t, `{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":11,"output_tokens":3}}`)
	c := NewAnthropicClient("k", "m", 10, time.Second)
	c.BaseURL = srv.URL + "/"
	res, err := c.Chat(context.Background(), "sys", "user")
	check(t, res, err, 11, 3)
	if req.URL.Path != "/v1/messages" || req.Header.Get("x-api-key") != "k" {
		t.Errorf("request = %s key=%q", req.URL.Path, req.Header.Get("x-api-key"))
	}
}

func TestGeminiBaseURLAndUsage(t *testing.T) {
	srv, req := fakeAPI(t, `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2}}`)
	c := NewGeminiClient("k", "gem", 10, time.Second)
	c.BaseURL = srv.URL
	res, err := c.Chat(context.Background(), "sys", "user")
	check(t, res, err, 7, 2)
	if req.URL.Path != "/v1beta/models/gem:generateContent" || req.URL.RawQuery != "" || req.Header.Get("x-goog-api-key") != "k" {
		t.Errorf("request = %s?%s key header=%q", req.URL.Path, req.URL.RawQuery, req.Header.Get("x-goog-api-key"))
	}
}

func TestNvidiaBaseURLAndUsage(t *testing.T) {
	srv, req := fakeAPI(t, `{"choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`)
	c := NewNvidiaClient("k", "nv", 10, time.Second, NvidiaOpts{})
	c.BaseURL = srv.URL
	res, err := c.Chat(context.Background(), "sys", "user")
	check(t, res, err, 5, 1)
	if req.URL.Path != "/v1/chat/completions" || req.Header.Get("Authorization") != "Bearer k" {
		t.Errorf("request = %s auth=%q", req.URL.Path, req.Header.Get("Authorization"))
	}
}

func TestEndpointDefaultsToPublicHost(t *testing.T) {
	if got := endpoint("", anthropicHost, "/v1/messages"); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("endpoint = %s", got)
	}
}
