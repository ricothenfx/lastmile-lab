package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNoopAlwaysErrNoLLM(t *testing.T) {
	resp, err := Noop{}.Complete(context.Background(), Request{System: "s", User: "u"})
	if resp != nil || !errors.Is(err, ErrNoLLM) {
		t.Fatalf("noop harus ErrNoLLM, dapat %v / %v", resp, err)
	}
}

func TestNewWithoutKeyIsNoop(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "   ") // whitespace = kosong
	if _, ok := New().(Noop); !ok {
		t.Fatal("tanpa API key provider harus Noop")
	}
	if Enabled() {
		t.Fatal("Enabled() harus false tanpa key")
	}
}

func TestNewWithKeyUsesOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	if _, ok := New().(*OpenAI); !ok {
		t.Fatal("dengan API key provider harus OpenAI")
	}
	if !Enabled() {
		t.Fatal("Enabled() harus true dengan key")
	}
}

func TestOpenAICompleteRequestShape(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody chatRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"test-model","choices":[{"message":{"role":"assistant","content":"{\"plans\":[]}"}}]}`))
	}))
	defer ts.Close()

	p := NewOpenAI(ts.URL, "secret", "test-model", 2*time.Second)
	resp, err := p.Complete(context.Background(), Request{System: "sys", User: "usr", JSONMode: true})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path salah: %s", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("auth salah: %s", gotAuth)
	}
	if gotBody.Model != "test-model" || len(gotBody.Messages) != 2 ||
		gotBody.Messages[0].Content != "sys" || gotBody.Messages[1].Content != "usr" {
		t.Fatalf("body salah: %+v", gotBody)
	}
	if gotBody.ResponseFormat == nil || gotBody.ResponseFormat.Type != "json_object" {
		t.Fatal("JSONMode harus meminta response_format json_object")
	}
	if resp.Text != `{"plans":[]}` || resp.Model != "test-model" {
		t.Fatalf("respons salah: %+v", resp)
	}
}

func TestOpenAIErrorSurfaced(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer ts.Close()
	p := NewOpenAI(ts.URL, "k", "m", time.Second)
	if _, err := p.Complete(context.Background(), Request{}); err == nil ||
		!strings.Contains(err.Error(), "429") {
		t.Fatalf("error status harus muncul, dapat: %v", err)
	}
}
