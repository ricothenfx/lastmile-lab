// strategy-lab — Strategy Lab backend (Fase 3, jaringan internal saja).
//
// Menjalankan duel A/B antar strategi dispatch pada dua engine identik
// (satu generator order seed sama di-pipe ke dua engine — lihat internal/duel)
// dan menyajikan hasilnya sebagai REST:
//
//	POST /run            {"strategy_a","strategy_b","preset","seconds","seed"}
//	                     → 202 {"id","status":"running"} — 409 bila duel lain aktif
//	GET  /results        daftar ringkas (duel selesai + duel berjalan)
//	GET  /results/{id}   hasil penuh (metrik, histogram, frame replay kembar)
//	GET  /healthz
//
// Hasil disimpan in-memory (maks MAX_RESULTS, default 8) dan — bila
// LAB_DATA_DIR diset — di-persist best-effort ke results.json.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/duel"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// defaultGraphPaths dicari berurutan bila GRAPH_PATH tidak diset.
var defaultGraphPaths = []string{
	os.Getenv("GRAPH_PATH"),
	"/app/data/berlin_graph.json",
	"../rider-sim/data/berlin_graph.json",
	"rider-sim/data/berlin_graph.json",
}

type runEntry struct {
	id       string
	stratA   string
	stratB   string
	preset   string
	seconds  int
	seed     int64
	created  time.Time
	progress atomic.Int64
	done     chan struct{}
}

type store struct {
	mu      sync.Mutex
	g       *graph.Graph
	results map[string]*duel.Result // selesai — immutable
	order   []string                // terbaru dulu
	running *runEntry               // maks satu duel bersamaan (VPS ramah)
	max     int
	dataDir string
}

type summary struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"`
	ProgressPct    int64     `json:"progress_pct"`
	Error          string    `json:"error,omitempty"`
	StrategyA      string    `json:"strategy_a"`
	StrategyB      string    `json:"strategy_b"`
	Preset         string    `json:"preset"`
	CreatedAt      time.Time `json:"created_at"`
	Seconds        int       `json:"seconds"`
	DeliveredA     int       `json:"delivered_a,omitempty"`
	DeliveredB     int       `json:"delivered_b,omitempty"`
	DeliveryP50AMS float64   `json:"delivery_p50_a_ms,omitempty"`
	DeliveryP50BMS float64   `json:"delivery_p50_b_ms,omitempty"`
	DurationWallMs int64     `json:"duration_wall_ms,omitempty"`
}

func (s *store) summaries() []summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]summary, 0, len(s.order)+1)
	if s.running != nil {
		r := s.running
		out = append(out, summary{
			ID: r.id, Status: "running", ProgressPct: r.progress.Load(),
			StrategyA: r.stratA, StrategyB: r.stratB, Preset: r.preset,
			CreatedAt: r.created, Seconds: r.seconds,
		})
	}
	for _, id := range s.order {
		r := s.results[id]
		sm := summary{
			ID: r.ID, Status: r.Status, Error: r.Error,
			StrategyA: r.StrategyA, StrategyB: r.StrategyB, Preset: r.Preset,
			CreatedAt: r.CreatedAt, Seconds: r.Seconds, DurationWallMs: r.DurationWallMs,
		}
		if r.A != nil {
			sm.DeliveredA, sm.DeliveryP50AMS = r.A.Delivered, r.A.DeliveryP50Ms
		}
		if r.B != nil {
			sm.DeliveredB, sm.DeliveryP50BMS = r.B.Delivered, r.B.DeliveryP50Ms
		}
		out = append(out, sm)
	}
	return out
}

func (s *store) start(req duel.Request) (string, int, error) {
	// Validasi cepat sebelum goroutine (400 rapi, bukan error asinkron).
	if _, err := dispatch.ByName(req.StrategyA); err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("strategy_a: %w", err)
	}
	if _, err := dispatch.ByName(req.StrategyB); err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("strategy_b: %w", err)
	}
	if _, err := duel.PresetByName(req.Preset); err != nil {
		return "", http.StatusBadRequest, err
	}

	id := "lab-" + strconv.FormatInt(time.Now().UnixMilli(), 36)
	entry := &runEntry{
		id: id, stratA: req.StrategyA, stratB: req.StrategyB,
		preset: req.Preset, seconds: req.Seconds, seed: req.Seed,
		created: time.Now().UTC(), done: make(chan struct{}),
	}

	s.mu.Lock()
	if s.running != nil {
		s.mu.Unlock()
		return "", http.StatusConflict, fmt.Errorf("duel lain sedang berjalan (%s)", s.running.id)
	}
	s.running = entry
	s.mu.Unlock()

	go func() {
		res, err := duel.Run(s.g, id, req, &entry.progress)
		s.mu.Lock()
		s.running = nil
		if err != nil {
			log.Printf("duel %s gagal: %v", id, err)
			res = &duel.Result{
				ID: id, Status: "error", Error: err.Error(),
				StrategyA: req.StrategyA, StrategyB: req.StrategyB,
				Preset: req.Preset, CreatedAt: entry.created.UTC(),
			}
		}
		s.results[id] = res
		s.order = append([]string{id}, s.order...)
		s.evict()
		s.mu.Unlock()
		s.persist()
		close(entry.done)
	}()
	return id, http.StatusAccepted, nil
}

// evict menjaga memori: simpan maks s.max hasil terbaru. Harus dipanggil
// dengan s.mu terkunci.
func (s *store) evict() {
	for len(s.order) > s.max {
		old := s.order[len(s.order)-1]
		s.order = s.order[:len(s.order)-1]
		delete(s.results, old)
	}
}

func (s *store) get(id string) (*duel.Result, *runEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.results[id]; ok {
		return r, nil, true
	}
	if s.running != nil && s.running.id == id {
		return nil, s.running, true
	}
	return nil, nil, false
}

// persist menulis hasil (best-effort) ke LAB_DATA_DIR/results.json.
func (s *store) persist() {
	if s.dataDir == "" {
		return
	}
	s.mu.Lock()
	all := make([]*duel.Result, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- { // urut lama→baru di file
		all = append(all, s.results[s.order[i]])
	}
	s.mu.Unlock()
	b, err := json.Marshal(all)
	if err != nil {
		log.Printf("persist marshal: %v", err)
		return
	}
	path := s.dataDir + "/results.json"
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Printf("persist write: %v", err)
	}
}

func (s *store) loadPersisted() {
	if s.dataDir == "" {
		return
	}
	b, err := os.ReadFile(s.dataDir + "/results.json")
	if err != nil {
		return // file belum ada — normal di boot pertama
	}
	var all []*duel.Result
	if err := json.Unmarshal(b, &all); err != nil {
		log.Printf("persisted results rusak, diabaikan: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range all {
		if r == nil || r.ID == "" {
			continue
		}
		s.results[r.ID] = r
		s.order = append([]string{r.ID}, s.order...)
	}
	s.evict()
	log.Printf("persisted results dimuat: %d duel", len(s.order))
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func muxFor(s *store, g *graph.Graph) *http.ServeMux {
	mux := http.NewServeMux()
	start := time.Now()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		running := s.running != nil
		n := len(s.order)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, model.Health{
			OK: true, Service: "strategy-lab",
			UptimeSec: int64(time.Since(start).Seconds()),
			Detail:    fmt.Sprintf("results=%d running=%v graph=%d nodes strategies=%v presets=%v", n, running, g.NodeCount(), dispatch.Names(), duel.PresetNames()),
		})
	})

	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var req duel.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StrategyA == "" || req.StrategyB == "" || req.Preset == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json — butuh strategy_a, strategy_b, preset"})
			return
		}
		id, code, err := s.start(req)
		if err != nil {
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, code, map[string]interface{}{"id": id, "status": "running"})
	})

	mux.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"results": s.summaries()})
	})

	mux.HandleFunc("/results/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/results/")
		if id == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "missing_id"})
			return
		}
		res, running, ok := s.get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if running != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id": running.id, "status": "running", "progress_pct": running.progress.Load(),
				"strategy_a": running.stratA, "strategy_b": running.stratB,
				"preset": running.preset, "seconds": running.seconds,
				"seed": running.seed, "created_at": running.created,
			})
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	return mux
}

func main() {
	port := envStr("PORT", "4205")
	maxResults := envInt("MAX_RESULTS", 8)
	dataDir := strings.TrimSuffix(envStr("LAB_DATA_DIR", ""), "/")

	g, err := loadGraph()
	if err != nil {
		log.Fatalf("load graph: %v", err)
	}
	log.Printf("graph: %d nodes, %d POIs", g.NodeCount(), len(g.POIs()))

	s := &store{g: g, results: map[string]*duel.Result{}, max: maxResults, dataDir: dataDir}
	s.loadPersisted()

	addr := "0.0.0.0:" + port
	log.Printf("strategy-lab listening on %s (max_results=%d data_dir=%q)", addr, maxResults, dataDir)
	srv := &http.Server{Addr: addr, Handler: muxFor(s, g), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func loadGraph() (*graph.Graph, error) {
	for _, p := range defaultGraphPaths {
		if p == "" {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		return graph.Load(f)
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
