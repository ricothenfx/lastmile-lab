// Proxy copilot (Fase 7, ADR D24) — satu file agar pola plugin LLM mudah
// diaudit. Prinsip "tanpa key = fitur tersembunyi" berlaku di sini pada
// level routing: tanpa COPILOT_URL, capabilities tetap menjawab
// {"enabled":false} (200 — UI tak perlu menebak dari 404), endpoint lain
// 503 copilot_disabled. Service copilot menolak rapi sendiri (503
// llm_disabled) bila service hidup tanpa OPENAI_API_KEY.
package main

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// copilotProxy handler /api/copilot/* (fungsi package-level untuk test).
func copilotProxy(copilotURL string, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if copilotURL == "" {
			if strings.HasSuffix(r.URL.Path, "/capabilities") {
				writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
				return
			}
			http.Error(w, `{"error":"copilot_disabled"}`, http.StatusServiceUnavailable)
			return
		}
		target := copilotURL + strings.TrimPrefix(r.URL.Path, "/api/copilot")
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// advise menjalankan dry-run duel (120 s virtual per plan) — butuh
		// timeout lebih longgar dari client gateway umum (4 s).
		if client == nil {
			client = &http.Client{Timeout: 45 * time.Second}
		}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"copilot unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}
