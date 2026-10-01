package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCopilotProxyDisabledCapabilities(t *testing.T) {
	h := copilotProxy("", nil)
	req := httptest.NewRequest(http.MethodGet, "/api/copilot/capabilities", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("capabilities tanpa COPILOT_URL harus 200 {\"enabled\":false}, dapat %d %s", w.Code, w.Body.String())
	}
}

func TestCopilotProxyDisabledOtherEndpoints(t *testing.T) {
	h := copilotProxy("", nil)
	for _, path := range []string{"/api/copilot/advise", "/api/copilot/ask", "/api/copilot"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		w := httptest.NewRecorder()
		h(w, req)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "copilot_disabled") {
			t.Fatalf("%s harus 503 copilot_disabled, dapat %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestCopilotProxyEnabledPassthrough(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		var buf strings.Builder
		_, _ = io.Copy(&buf, r.Body)
		gotBody = buf.String()
		_, _ = w.Write([]byte(`{"enabled":true}`))
	}))
	defer upstream.Close()

	h := copilotProxy(upstream.URL, &http.Client{Timeout: 2 * time.Second})
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/advise", strings.NewReader(`{"seed":7}`))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK || w.Body.String() != `{"enabled":true}` {
		t.Fatalf("passthrough salah: %d %s", w.Code, w.Body.String())
	}
	if gotPath != "/advise" || gotMethod != http.MethodPost || gotBody != `{"seed":7}` {
		t.Fatalf("upstream salah: %s %s %q", gotMethod, gotPath, gotBody)
	}
}

func TestCopilotProxyUpstreamDown(t *testing.T) {
	h := copilotProxy("http://127.0.0.1:1", &http.Client{Timeout: time.Second})
	req := httptest.NewRequest(http.MethodGet, "/api/copilot/capabilities", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("upstream mati harus 502, dapat %d", w.Code)
	}
}
