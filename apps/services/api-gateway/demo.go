// Golden Demo presets (Fase 5) — narasi ±90 detik yang selalu konsisten.
//
// Orchestrator murni memakai kontrol yang SUDAH ada: surge/weather via
// sim-control (SIM_CONTROL_URL) dan kill node via chaos injector
// (CHAOS_URL, profile chaos). TIDAK ada jalur kode baru di engine, tanpa
// LLM; langkah narasi hard-coded di file ini saja. Kill dalam preset lewat
// chaos /kill biasa → incident "chaos-kill" terekam di Incident Timeline
// (satu sumber kebenaran, ADR D19).
//
//	GET  /api/demo/presets  daftar preset + langkah narasi
//	POST /api/demo/play     {"id":"dinner-rush"} — sekali jalan; 409 bila sibuk
//	POST /api/demo/stop     hentikan + kembalikan baseline
//	GET  /api/demo/state    {active, preset, step, elapsed, …}
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- definisi preset (satu-satunya tempat narasi) ----

type demoStepKind string

const (
	stepReset   demoStepKind = "reset"   // surge 1 + weather 1
	stepSurge   demoStepKind = "surge"   // value ×1–×10
	stepWeather demoStepKind = "weather" // value ×0.2–×2
	stepKill    demoStepKind = "kill"    // target = nama allowlist chaos
)

type demoStep struct {
	AtSec  float64      `json:"at_s"`
	Label  string       `json:"label"`
	Kind   demoStepKind `json:"kind"`
	Value  float64      `json:"value,omitempty"`
	Target string       `json:"target,omitempty"`
}

type demoPreset struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Desc        string     `json:"desc"`
	DurationSec float64    `json:"duration_s"`
	Steps       []demoStep `json:"steps"`
}

// Narasi Golden Demo — total ±90 detik per preset, selalu urutan yang sama.
// Target kill HANYA node stateless yang hidup di mode demo (sim+chaos) dan
// BUKAN api-gateway sendiri (orchestrator ikut mati) atau sim-control
// (aktuator surge/weather demo).
var demoPresets = []demoPreset{
	{
		ID:          "dinner-rush",
		Name:        "Dinner Rush in Berlin",
		Desc:        "Calm → flash sale ×8 → heavy rain → sim node dies → self-heals",
		DurationSec: 90,
		Steps: []demoStep{
			{AtSec: 0, Label: "Quiet network — fleet of 100 riders ready", Kind: stepReset},
			{AtSec: 12, Label: "FLASH SALE ×8 — orders flood in", Kind: stepSurge, Value: 8},
			{AtSec: 36, Label: "Heavy rain — riders slow down 40%", Kind: stepWeather, Value: 0.6},
			{AtSec: 54, Label: "CHAOS: sim node killed — self-heal", Kind: stepKill, Target: "rider-sim"},
			{AtSec: 64, Label: "Rain eases as the node recovers", Kind: stepWeather, Value: 1},
			{AtSec: 74, Label: "Surge fades — queue drains", Kind: stepSurge, Value: 2},
			{AtSec: 86, Label: "Back to calm — SLO recovered", Kind: stepReset},
		},
	},
	{
		ID:          "blackout-drill",
		Name:        "Blackout Drill",
		Desc:        "Three nodes killed in turn — MTTD/MTTR measured on the Incident Timeline",
		DurationSec: 90,
		Steps: []demoStep{
			{AtSec: 0, Label: "Healthy baseline — all nodes green", Kind: stepReset},
			{AtSec: 14, Label: "CHAOS: stream gateway cut — UI falls back to replay", Kind: stepKill, Target: "ws-gateway"},
			{AtSec: 40, Label: "CHAOS: lab analytics down", Kind: stepKill, Target: "strategy-lab"},
			{AtSec: 62, Label: "CHAOS: core simulation restarts", Kind: stepKill, Target: "rider-sim"},
			{AtSec: 84, Label: "Everything recovers without manual restart — incidents auto-recorded", Kind: stepReset},
		},
	},
	{
		ID:          "rain-commute",
		Name:        "Rain Commute",
		Desc:        "Rain + rising demand — queue and order TTL tested without chaos",
		DurationSec: 88,
		Steps: []demoStep{
			{AtSec: 0, Label: "Clear morning — normal demand", Kind: stepReset},
			{AtSec: 15, Label: "Rain starts — fleet speed drops", Kind: stepWeather, Value: 0.6},
			{AtSec: 30, Label: "Demand ×4 in the rain", Kind: stepSurge, Value: 4},
			{AtSec: 50, Label: "Peak pressure ×6 — queue builds up", Kind: stepSurge, Value: 6},
			{AtSec: 66, Label: "Rain eases — demand drops", Kind: stepSurge, Value: 2},
			{AtSec: 78, Label: "Clear again — queue drains", Kind: stepReset},
		},
	},
}

// ---- runner ----

type demoRunner struct {
	mu         sync.Mutex
	controlURL string // sim-control (wajib; tanpa ini demo 503)
	chaosURL   string // chaos injector (kosong → langkah kill di-skip)
	client     *http.Client
	active     *demoRun
	lastID     string
	lastDoneAt time.Time
}

type demoRun struct {
	preset     demoPreset
	startedAt  time.Time
	stopCh     chan struct{}
	stopped    bool
	stepStatus []string // pending|ok|skipped — index = steps
}

type demoStateView struct {
	Active     bool     `json:"active"`
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name,omitempty"`
	Step       int      `json:"step"`
	TotalSteps int      `json:"total_steps"`
	Label      string   `json:"label,omitempty"`
	ElapsedS   float64  `json:"elapsed_s"`
	DurationS  float64  `json:"duration_s"`
	StepStatus []string `json:"step_status,omitempty"`
	LastError  string   `json:"last_error,omitempty"`
	LastID     string   `json:"last_id,omitempty"`
}

func newDemoRunner(controlURL, chaosURL string) *demoRunner {
	return &demoRunner{
		controlURL: controlURL,
		chaosURL:   chaosURL,
		client:     &http.Client{Timeout: 3 * time.Second},
	}
}

func presetByID(id string) (demoPreset, bool) {
	for _, p := range demoPresets {
		if p.ID == id {
			return p, true
		}
	}
	return demoPreset{}, false
}

// postControl mengirim satu aksi ke sim-control /control/{kind}.
func (d *demoRunner) postControl(ctx context.Context, kind string, value float64) error {
	body := `{"factor":` + strconv.FormatFloat(value, 'f', -1, 64) + `}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.controlURL+"/control/"+kind, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sim-control %s: HTTP %s", kind, strconv.Itoa(resp.StatusCode))
	}
	return nil
}

// postKill mengirim kill ke chaos injector; return (skipped, err).
// 409 (node tidak sehat / container mati) = skip langkah, demo lanjut.
func (d *demoRunner) postKill(ctx context.Context, target string) (bool, error) {
	if d.chaosURL == "" {
		return true, nil // chaos opsional (profile chaos mati) → langkah dilewati
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.chaosURL+"/kill", strings.NewReader(`{"target":"`+target+`"}`))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusAccepted:
		return false, nil
	case resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusForbidden:
		return true, nil // tidak sehat / chaos off → skip naratif, jangan gagalkan demo
	default:
		return true, fmt.Errorf("chaos kill: HTTP %s", strconv.Itoa(resp.StatusCode))
	}
}

// applyStep mengeksekusi satu langkah; return label hasil untuk state.
func (d *demoRunner) applyStep(ctx context.Context, s demoStep) (bool, error) {
	switch s.Kind {
	case stepSurge:
		return false, d.postControl(ctx, "surge", s.Value)
	case stepWeather:
		return false, d.postControl(ctx, "weather", s.Value)
	case stepReset:
		if err := d.postControl(ctx, "surge", 1); err != nil {
			return false, err
		}
		return false, d.postControl(ctx, "weather", 1)
	case stepKill:
		return d.postKill(ctx, s.Target)
	}
	return true, nil
}

func (d *demoRunner) play(id string) (demoStateView, int) {
	p, ok := presetByID(id)
	if !ok {
		return demoStateView{}, http.StatusNotFound
	}
	d.mu.Lock()
	if d.active != nil {
		d.mu.Unlock()
		return demoStateView{}, http.StatusConflict
	}
	d.mu.Unlock()
	// Sim-control wajib hidup sebelum demo mulai (feedback < 1 s harus nyata).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := d.postControl(ctx, "surge", 1)
	cancel()
	if err != nil {
		return demoStateView{}, http.StatusServiceUnavailable
	}

	run := &demoRun{
		preset:     p,
		startedAt:  time.Now(),
		stopCh:     make(chan struct{}),
		stepStatus: make([]string, len(p.Steps)),
	}
	for i := range run.stepStatus {
		run.stepStatus[i] = "pending"
	}
	d.mu.Lock()
	if d.active != nil { // re-check: play ganda saat probe berjalan
		d.mu.Unlock()
		return demoStateView{}, http.StatusConflict
	}
	d.active = run
	d.mu.Unlock()
	go d.loop(run)
	return d.state(), http.StatusOK
}

// loop menggerakkan langkah ber-waktu: satu goroutine per demo, ticker
// 250 ms, langkah diterapkan saat AtSec tercapai; selesai sendiri di akhir.
func (d *demoRunner) loop(run *demoRun) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	next := 0
	for {
		select {
		case <-run.stopCh:
			return
		case <-ticker.C:
			elapsed := time.Since(run.startedAt).Seconds()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			for next < len(run.preset.Steps) && elapsed >= run.preset.Steps[next].AtSec {
				s := run.preset.Steps[next]
				skipped, err := d.applyStep(ctx, s)
				d.mu.Lock()
				if err != nil {
					run.stepStatus[next] = "skipped"
					log.Printf("demo %s: langkah %d (%s) gagal: %v", run.preset.ID, next, s.Label, err)
				} else if skipped {
					run.stepStatus[next] = "skipped"
				} else {
					run.stepStatus[next] = "ok"
				}
				d.mu.Unlock()
				log.Printf("demo %s: langkah %d/%d t+%.1fs — %s [%s]",
					run.preset.ID, next+1, len(run.preset.Steps), elapsed, s.Label, run.stepStatus[next])
				next++
			}
			cancel()
			if next >= len(run.preset.Steps) && elapsed >= run.preset.DurationSec {
				log.Printf("demo %s: selesai t+%.1fs (durasi preset %.0fs)", run.preset.ID, elapsed, run.preset.DurationSec)
				d.finish(run)
				return
			}
		}
	}
}

func (d *demoRunner) finish(run *demoRun) {
	// Baseline dikembalikan best-effort (engine yang di-kill sudah reset sendiri).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = d.postControl(ctx, "surge", 1)
	_ = d.postControl(ctx, "weather", 1)
	d.mu.Lock()
	if d.active == run {
		d.active = nil
		d.lastID = run.preset.ID
		d.lastDoneAt = time.Now()
	}
	d.mu.Unlock()
}

func (d *demoRunner) stop() {
	d.mu.Lock()
	run := d.active
	if run != nil {
		run.stopped = true
	}
	d.mu.Unlock()
	if run != nil {
		close(run.stopCh)
		d.finish(run)
	}
}

func (d *demoRunner) state() demoStateView {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.active == nil {
		return demoStateView{Active: false, LastID: d.lastID}
	}
	run := d.active
	elapsed := time.Since(run.startedAt).Seconds()
	step := 0
	label := ""
	for i, st := range run.stepStatus {
		if st != "pending" {
			step = i + 1
			if i < len(run.preset.Steps) {
				label = run.preset.Steps[i].Label
			}
		}
	}
	if run.stopped {
		label = "DIHENTIKAN"
	}
	return demoStateView{
		Active: true, ID: run.preset.ID, Name: run.preset.Name,
		Step: step, TotalSteps: len(run.preset.Steps), Label: label,
		ElapsedS: elapsed, DurationS: run.preset.DurationSec,
		StepStatus: run.stepStatus,
	}
}

// registerDemo memasang endpoint /api/demo/*.
func registerDemo(mux *http.ServeMux, d *demoRunner) {
	mux.HandleFunc("GET /api/demo/presets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"presets": demoPresets})
	})
	mux.HandleFunc("POST /api/demo/play", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			http.Error(w, `{"error":"bad_json — butuh id"}`, http.StatusBadRequest)
			return
		}
		state, code := d.play(req.ID)
		if code != http.StatusOK {
			msg := map[int]string{
				http.StatusNotFound:           `{"error":"preset_not_found"}`,
				http.StatusConflict:           `{"error":"demo_already_running"}`,
				http.StatusServiceUnavailable: `{"error":"sim_control_unreachable"}`,
			}[code]
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("POST /api/demo/stop", func(w http.ResponseWriter, r *http.Request) {
		d.stop()
		writeJSON(w, http.StatusOK, d.state())
	})
	mux.HandleFunc("GET /api/demo/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, d.state())
	})
}
