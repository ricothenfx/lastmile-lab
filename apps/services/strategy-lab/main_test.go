package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/duel"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
)

func tinyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	meta := graph.Meta{City: "LabTest", Source: "test", BBox: []float64{0, 0, 1, 1}}
	nodes := [][2]float64{
		{0.000, 0.000}, {0.000, 0.010}, {0.010, 0.000}, {0.010, 0.010},
	}
	edges := [][3]float64{{0, 1, 800}, {0, 2, 800}, {1, 3, 800}, {2, 3, 800}, {1, 2, 1100}}
	g, err := graph.New(meta, nodes, edges, nil, []int{0, 3})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func post(ts *httptest.Server, url, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(w, req)
	return w
}

func get(ts *httptest.Server, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(w, req)
	return w
}

func TestRunValidatesAndExecutes(t *testing.T) {
	s := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 4}
	ts := httptest.NewServer(muxFor(s, s.g))
	defer ts.Close()

	// payload tidak lengkap → 400
	if res := post(ts, "/run", `{"strategy_a":"fifo"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("payload setengah jalan harus 400, dapat %d", res.Code)
	}
	// strategi tak dikenal → 400
	if res := post(ts, "/run", `{"strategy_a":"gpt","strategy_b":"fifo","preset":"steady"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("strategi tak dikenal harus 400, dapat %d", res.Code)
	}
	// preset tak dikenal → 400
	if res := post(ts, "/run", `{"strategy_a":"fifo","strategy_b":"zone","preset":"nope"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("preset tak dikenal harus 400, dapat %d", res.Code)
	}

	// duel valid → 202 + id
	res := post(ts, "/run", `{"strategy_a":"fifo","strategy_b":"optimal","preset":"steady","seconds":60,"seed":7}`)
	if res.Code != http.StatusAccepted {
		t.Fatalf("duel valid harus 202, dapat %d: %s", res.Code, res.Body.String())
	}
	var started struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &started); err != nil || started.ID == "" {
		t.Fatalf("respons 202 harus berisi id: %v (%s)", err, res.Body.String())
	}

	// duel kedua saat sibuk → 409
	if res := post(ts, "/run", `{"strategy_a":"fifo","strategy_b":"zone","preset":"steady","seconds":60}`); res.Code != http.StatusConflict {
		t.Fatalf("duel paralel harus 409, dapat %d", res.Code)
	}

	// duel berjalan dilaporkan di daftar
	if res := get(ts, "/results"); !strings.Contains(res.Body.String(), `"running"`) {
		t.Fatalf("daftar harus memuat duel berjalan: %s", res.Body.String())
	}

	// poll sampai selesai (duel 60s virtual ≈ < 10 dinding)
	var full struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		A      *struct {
			Delivered int `json:"delivered"`
		} `json:"a"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		res := get(ts, "/results/"+started.ID)
		if res.Code != http.StatusOK {
			t.Fatalf("GET hasil harus 200, dapat %d", res.Code)
		}
		if err := json.Unmarshal(res.Body.Bytes(), &full); err != nil {
			t.Fatalf("decode hasil: %v (%s)", err, res.Body.String())
		}
		if full.Status != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("duel tidak selesai dalam 30 dinding")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Graph test mini + preset steady 60s virtual → order masuk & duel selesai;
	// nilai metrik penuh sudah divalidasi di internal/duel (duel_test).
	if full.Status != "done" || full.A == nil {
		t.Fatalf("hasil akhir harus done + sisi A terisi: %+v", full)
	}

	// daftar ringkas memuat duel tersebut
	if res := get(ts, "/results"); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), started.ID) {
		t.Fatalf("daftar results harus memuat %s", started.ID)
	}
	// id tak dikenal → 404
	if res := get(ts, "/results/lab-tidak-ada"); res.Code != http.StatusNotFound {
		t.Fatalf("hasil tak dikenal harus 404, dapat %d", res.Code)
	}
}

func TestProgressReportedWhileRunning(t *testing.T) {
	s := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 4}
	e := &runEntry{id: "lab-x", stratA: "fifo", stratB: "zone", preset: "rush",
		created: time.Now().UTC(), done: make(chan struct{})}
	e.progress.Store(42)
	s.mu.Lock()
	s.running = e
	s.mu.Unlock()
	sums := s.summaries()
	if len(sums) != 1 || sums[0].Status != "running" || sums[0].ProgressPct != 42 {
		t.Fatalf("summary duel berjalan salah: %+v", sums)
	}
}

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 4, dataDir: dir}
	s.results["lab-1"] = &duel.Result{ID: "lab-1", Status: "done", CreatedAt: time.Now().UTC()}
	s.order = []string{"lab-1"}
	s.persist()

	s2 := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 4, dataDir: dir}
	s2.loadPersisted()
	if len(s2.order) != 1 || s2.order[0] != "lab-1" {
		t.Fatalf("persisted harus dimuat kembali, dapat %v", s2.order)
	}
	if _, err := os.Stat(filepath.Join(dir, "results.json")); err != nil {
		t.Fatalf("results.json harus ada: %v", err)
	}
}

func TestEvictionKeepsMaxResults(t *testing.T) {
	s := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 2}
	for i := 3; i >= 1; i-- {
		id := "lab-" + string(rune('0'+i))
		s.results[id] = &duel.Result{ID: id, Status: "done"}
		s.order = append([]string{id}, s.order...)
	}
	s.mu.Lock()
	s.evict()
	s.mu.Unlock()
	if len(s.order) != 2 || s.order[0] != "lab-1" || s.order[1] != "lab-2" {
		t.Fatalf("eviction salah: %v", s.order)
	}
}

func TestHealthz(t *testing.T) {
	s := &store{g: tinyGraph(t), results: map[string]*duel.Result{}, max: 4}
	ts := httptest.NewServer(muxFor(s, s.g))
	defer ts.Close()
	res := get(ts, "/healthz")
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "strategy-lab") {
		t.Fatalf("healthz harus ok: %d %s", res.Code, res.Body.String())
	}
}
