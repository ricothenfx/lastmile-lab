// copilot — AI Ops Copilot & Plan Advisor (Fase 7, ADR D24).
//
// Service plugin LLM di LUAR core deterministik. Tanpa OPENAI_API_KEY:
// provider noop → fitur menolak rapi, UI menyembunyikan seluruh panel,
// aplikasi tetap utuh. LLM tidak pernah punya kredensial dan tidak pernah
// mengeksekusi apa pun — dry-run plan memakai simulator (internal/dryrun
// di atas jalur duel internal/duel), eksekusi live tetap tombol manusia
// via endpoint kontrol existing (sim-control / chaos).
//
//	GET  /healthz        liveness + enabled + graph
//	GET  /capabilities   {"enabled":bool} — sumber UI render/sembunyikan panel
//	POST /advise         metrik live → ≤3 plan (schema ketat) → dry-run per plan
//	POST /ask            pertanyaan → jawaban WAJIB sitasi data internal
//
// Port 4207, jaringan internal saja (diakses via proxy api-gateway),
// profile compose `copilot` OFF-by-default, mem_limit 128 MiB.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/advise"
	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/ask"
	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/dryrun"
	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/llm"
	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/opsctx"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
)

// defaultGraphPaths dicari berurutan bila GRAPH_PATH tidak diset (pola
// strategy-lab — graph Berlin satu sumber kebenaran dari rider-sim).
var defaultGraphPaths = []string{
	os.Getenv("GRAPH_PATH"),
	"/app/data/berlin_graph.json",
	"../rider-sim/data/berlin_graph.json",
	"rider-sim/data/berlin_graph.json",
}

// limiter token bucket sederhana per endpoint (spec: rate limit sederhana
// di memori — bukan auth besar, D23).
type limiter struct {
	mu     sync.Mutex
	tokens float64
	max    float64
	perSec float64
	last   time.Time
}

func newLimiter(perMin int) *limiter {
	if perMin <= 0 {
		perMin = 6
	}
	return &limiter{tokens: float64(perMin), max: float64(perMin), perSec: float64(perMin) / 60, last: time.Now()}
}

func (l *limiter) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.tokens += now.Sub(l.last).Seconds() * l.perSec
	if l.tokens > l.max {
		l.tokens = l.max
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

type server struct {
	prov      llm.Provider
	gw        *opsctx.Client
	g         *graph.Graph
	limAdvise *limiter
	limAsk    *limiter
}

// writeErr respons error JSON konsisten (tanpa stack ke klien).
func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *server) mux() *http.ServeMux {
	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		enabled := llm.Enabled()
		status := http.StatusOK
		if s.g == nil {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]interface{}{
			"ok":          s.g != nil,
			"service":     "copilot",
			"enabled":     enabled,
			"uptime_sec":  int64(time.Since(start).Seconds()),
			"graph_nodes": graphNodes(s.g),
		})
	})
	mux.HandleFunc("/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": llm.Enabled()})
	})
	mux.HandleFunc("/advise", s.handleAdvise)
	mux.HandleFunc("/ask", s.handleAsk)
	return mux
}

func graphNodes(g *graph.Graph) int {
	if g == nil {
		return 0
	}
	return g.NodeCount()
}

// handleAdvise: metrik live → 1 call LLM → parse ketat → dry-run duel per plan.
func (s *server) handleAdvise(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !llm.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "llm_disabled — copilot tanpa OPENAI_API_KEY")
		return
	}
	if s.g == nil {
		writeErr(w, http.StatusServiceUnavailable, "graph_not_loaded")
		return
	}
	if !s.limAdvise.allow() {
		writeErr(w, http.StatusTooManyRequests, "rate_limited — coba lagi sebentar")
		return
	}
	var body struct {
		Seed int64 `json:"seed,omitempty"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cx := s.gw.Fetch(ctx)

	prompt := advise.BuildUserPrompt(cx.JSON, liveStrategy(cx))
	resp, err := s.prov.Complete(ctx, llm.Request{System: advise.SystemPrompt(), User: prompt, JSONMode: true})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "llm_failed: "+err.Error())
		return
	}
	plans, err := advise.ParsePlans(resp.Text)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "plan_invalid: "+err.Error())
		return
	}

	live := liveState(cx)
	seed := body.Seed
	if seed == 0 {
		seed = time.Now().UnixNano() & 0x7FFFFFFF // baseline & plan SATU seed
	}
	dr, err := dryrun.Run(ctx, s.g, live, seed, plans)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "dryrun_failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"seed":              dr.Seed,
		"seconds":           dr.Seconds,
		"base_rate_per_min": dr.BaseRate,
		"baseline":          dr.Baseline,
		"plans":             dr.PlanResults,
		"model":             resp.Model,
		"took_ms":           time.Since(cx.FetchedAt).Milliseconds(),
	})
}

// handleAsk: pertanyaan → jawaban wajib sitasi; tanpa sitasi DITOLAK (422).
func (s *server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !llm.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "llm_disabled — copilot tanpa OPENAI_API_KEY")
		return
	}
	if !s.limAsk.allow() {
		writeErr(w, http.StatusTooManyRequests, "rate_limited — coba lagi sebentar")
		return
	}
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json")
		return
	}
	q := strings.TrimSpace(body.Question)
	if len(q) < 3 || len(q) > 500 {
		writeErr(w, http.StatusBadRequest, "question 3..500 karakter")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cx := s.gw.Fetch(ctx)

	resp, err := s.prov.Complete(ctx, llm.Request{
		System:   ask.SystemPrompt(),
		User:     ask.BuildUserPrompt(q, toAskSources(cx.Sources)),
		JSONMode: true,
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "llm_failed: "+err.Error())
		return
	}
	ans, err := ask.Validate(resp.Text, toAskSources(cx.Sources))
	if err != nil {
		// jawaban tanpa sitasi / sitasi tak dikenal → ditolak, bukan ditampilkan.
		writeErr(w, http.StatusUnprocessableEntity, "answer_rejected: "+err.Error())
		return
	}
	cited := make([]opsctx.Source, 0, len(ans.Sources))
	for _, id := range ans.Sources {
		for _, src := range cx.Sources {
			if src.ID == id {
				cited = append(cited, src)
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"text":    ans.Text,
		"sources": cited,
		"model":   resp.Model,
		"took_ms": time.Since(cx.FetchedAt).Milliseconds(),
	})
}

// toAskSources mengonversi sitasi opsctx → tipe paket ask (kontrak ketat).
func toAskSources(src []opsctx.Source) []ask.Source {
	out := make([]ask.Source, 0, len(src))
	for _, s := range src {
		out = append(out, ask.Source{ID: s.ID, Label: s.Label, Value: s.Value})
	}
	return out
}

// liveState mengekstrak baseline live dari payload KPI (fallback deterministik
// bila metrik tak terukur — dry-run tetap jalan, dijelaskan di laporan).
func liveState(cx *opsctx.Context) dryrun.LiveState {
	st := dryrun.LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 20}
	var doc struct {
		KPI struct {
			Sim struct {
				Strategy string  `json:"strategy"`
				Surge    float64 `json:"surge"`
				Weather  float64 `json:"weather"`
			} `json:"sim"`
			OrdersPerMin *float64 `json:"orders_per_min"`
		} `json:"kpi"`
	}
	if err := json.Unmarshal([]byte(cx.JSON), &doc); err != nil {
		return st
	}
	if doc.KPI.Sim.Strategy != "" {
		st.Strategy = doc.KPI.Sim.Strategy
	}
	if doc.KPI.Sim.Surge > 0 {
		st.Surge = doc.KPI.Sim.Surge
	}
	if doc.KPI.Sim.Weather > 0 {
		st.Weather = doc.KPI.Sim.Weather
	}
	if doc.KPI.OrdersPerMin != nil && *doc.KPI.OrdersPerMin > 0 {
		st.RatePerMin = *doc.KPI.OrdersPerMin
	}
	return st
}

func liveStrategy(cx *opsctx.Context) string {
	return liveState(cx).Strategy
}

func loadGraph() (*graph.Graph, error) {
	for _, p := range defaultGraphPaths {
		if p == "" {
			continue
		}
		if f, err := os.Open(p); err == nil {
			defer f.Close()
			return graph.Load(f)
		}
	}
	return nil, fmt.Errorf("graph tidak ditemukan (set GRAPH_PATH; coba: %v)", defaultGraphPaths)
}

func envStr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	port := envStr("PORT", "4207")
	limPerMin := envInt("RATE_PER_MIN", 6)

	g, err := loadGraph()
	if err != nil {
		// graph hanya dibutuhkan dry-run advise — service tetap hidup untuk
		// capabilities/ask; healthz menandai tidak siap.
		log.Printf("WARN graph: %v", err)
	}
	s := &server{
		prov:      llm.New(),
		gw:        opsctx.New(envStr("API_GATEWAY_URL", "http://127.0.0.1:3010")),
		g:         g,
		limAdvise: newLimiter(limPerMin),
		limAsk:    newLimiter(limPerMin),
	}
	enabled := llm.Enabled()
	log.Printf("copilot listening on :%s (enabled=%v graph=%v rate=%d/min)",
		port, enabled, g != nil, limPerMin)
	srv := &http.Server{Addr: "0.0.0.0:" + port, Handler: s.mux(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
