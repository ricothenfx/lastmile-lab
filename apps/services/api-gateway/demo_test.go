package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubControl = sim-control palsu: mencatat setiap aksi kontrol berurutan.
type stubControl struct {
	mu     sync.Mutex
	calls  []string // "surge=8", "weather=0.6", …
	server *httptest.Server
}

func newStubControl(t *testing.T) *stubControl {
	s := &stubControl{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /control/surge", s.handle("surge"))
	mux.HandleFunc("POST /control/weather", s.handle("weather"))
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"surge":1,"weather":1}`))
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *stubControl) handle(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Factor float64 `json:"factor"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		s.mu.Lock()
		s.calls = append(s.calls, kind+"="+strings.TrimRight(strings.TrimRight(
			json.Number(jsonFloat(req.Factor)).String(), "0"), "."))
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}
}

func jsonFloat(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func (s *stubControl) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

// stubChaos = chaos palsu: mencatat target kill; bisa dikonfigurasi 409.
type stubChaos struct {
	mu      sync.Mutex
	targets []string
	deny    bool
	server  *httptest.Server
}

func newStubChaos(t *testing.T, deny bool) *stubChaos {
	c := &stubChaos{deny: deny}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /kill", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target string `json:"target"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		c.mu.Lock()
		c.targets = append(c.targets, req.Target)
		c.mu.Unlock()
		if c.deny {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"container_not_running"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"accepted":true}`))
	})
	c.server = httptest.NewServer(mux)
	t.Cleanup(c.server.Close)
	return c
}

func (c *stubChaos) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.targets))
	copy(out, c.targets)
	return out
}

// shortPreset mengganti daftar preset global dengan preset uji berdurasi
// pendek — langkah & mesin runner-nya persis produksi.
func shortPreset(t *testing.T, killTarget string, chaosDeny bool) {
	old := demoPresets
	demoPresets = []demoPreset{{
		ID: "test", Name: "Test", DurationSec: 1,
		Steps: []demoStep{
			{AtSec: 0, Label: "reset awal", Kind: stepReset},
			{AtSec: 0.3, Label: "naikkan surge", Kind: stepSurge, Value: 8},
			{AtSec: 0.6, Label: "kill node", Kind: stepKill, Target: killTarget},
		},
	}}
	t.Cleanup(func() { demoPresets = old })
}

func waitState(d *demoRunner, want bool, timeout time.Duration) demoStateView {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := d.state()
		if st.Active == want {
			return st
		}
		time.Sleep(25 * time.Millisecond)
	}
	t := d.state()
	return t
}

func TestDemoStepsFireInOrder(t *testing.T) {
	shortPreset(t, "rider-sim", false)
	ctl := newStubControl(t)
	chaos := newStubChaos(t, false)
	d := newDemoRunner(ctl.server.URL, chaos.server.URL)

	state, code := d.play("test")
	if code != http.StatusOK {
		t.Fatalf("play = %d", code)
	}
	if !state.Active || state.TotalSteps != 3 {
		t.Fatalf("state awal salah: %+v", state)
	}
	final := waitState(d, false, 4*time.Second)
	if final.LastID != "test" {
		t.Fatalf("demo tidak selesai: %+v", final)
	}
	got := ctl.snapshot()
	// reset awal(surge+weather) → probe play(surge 1) + reset finish(surge+weather)
	var surges, weathers int
	for _, c := range got {
		switch {
		case strings.HasPrefix(c, "surge=8"):
			surges++
		case strings.HasPrefix(c, "weather"):
			weathers++
		}
	}
	if surges != 1 || weathers < 2 {
		t.Fatalf("aksi kontrol salah: %v", got)
	}
	if kills := chaos.snapshot(); len(kills) != 1 || kills[0] != "rider-sim" {
		t.Fatalf("kill salah: %v", kills)
	}
}

func TestDemoKillSkippedContinues(t *testing.T) {
	shortPreset(t, "rider-sim", true) // chaos menolak (409)
	ctl := newStubControl(t)
	chaos := newStubChaos(t, true)
	d := newDemoRunner(ctl.server.URL, chaos.server.URL)

	if _, code := d.play("test"); code != http.StatusOK {
		t.Fatalf("play = %d", code)
	}
	final := waitState(d, false, 4*time.Second)
	if final.LastID != "test" {
		t.Fatalf("langkah kill 409 harus di-skip tanpa menggagalkan demo: %+v", final)
	}
	st := d.state()
	for i, s := range st.StepStatus {
		if s == "pending" {
			t.Fatalf("langkah %d tidak pernah diproses", i)
		}
	}
}

func TestDemoStopAndConflict(t *testing.T) {
	shortPreset(t, "rider-sim", false)
	ctl := newStubControl(t)
	d := newDemoRunner(ctl.server.URL, "")

	if _, code := d.play("test"); code != http.StatusOK {
		t.Fatalf("play = %d", code)
	}
	d.stop()
	if st := d.state(); st.Active {
		t.Fatalf("stop tidak menghentikan demo")
	}
	// setelah stop, play lagi boleh (slot bebas)
	if _, code := d.play("test"); code != http.StatusOK {
		t.Fatalf("play kedua = %d, want 200", code)
	}
	if _, code := d.play("test"); code != http.StatusConflict {
		t.Fatalf("play tumpang-tindih = %d, want 409", code)
	}
	d.stop()
}

func TestDemoPlayNeedsSimControl(t *testing.T) {
	shortPreset(t, "rider-sim", false)
	// sim-control mati: URL tak dilayani
	d := newDemoRunner("http://127.0.0.1:1", "")
	if _, code := d.play("test"); code != http.StatusServiceUnavailable {
		t.Fatalf("play tanpa sim-control = %d, want 503", code)
	}
}

func TestDemoPresetNotFound(t *testing.T) {
	ctl := newStubControl(t)
	d := newDemoRunner(ctl.server.URL, "")
	if _, code := d.play("tidak-ada"); code != http.StatusNotFound {
		t.Fatalf("play preset asing = %d, want 404", code)
	}
}

// Invarian narasi Golden Demo produksi: durasi ±90 detik, langkah naik,
// kill hanya node stateless mode demo (BUKAN api-gateway sendiri / sim-control),
// reset di awal, dan minimal satu preset memuat chaos kill.
func TestDemoPresetsNarrationInvariants(t *testing.T) {
	allowedKills := map[string]bool{"rider-sim": true, "ws-gateway": true, "strategy-lab": true}
	hasKill := false
	ids := map[string]bool{}
	for _, p := range demoPresets {
		if ids[p.ID] {
			t.Fatalf("id preset duplikat: %s", p.ID)
		}
		ids[p.ID] = true
		if p.DurationSec < 80 || p.DurationSec > 95 {
			t.Fatalf("%s: durasi %.0f s di luar narasi ±90 s", p.ID, p.DurationSec)
		}
		if len(p.Steps) < 3 {
			t.Fatalf("%s: narasi terlalu sedikit", p.ID)
		}
		if p.Steps[0].Kind != stepReset {
			t.Fatalf("%s: langkah pertama harus reset baseline", p.ID)
		}
		last := -1.0
		for _, s := range p.Steps {
			if s.AtSec < last {
				t.Fatalf("%s: langkah tidak naik", p.ID)
			}
			last = s.AtSec
			if s.AtSec >= p.DurationSec {
				t.Fatalf("%s: langkah melewati durasi", p.ID)
			}
			if s.Kind == stepKill {
				hasKill = true
				if !allowedKills[s.Target] {
					t.Fatalf("%s: kill target %q di luar set aman", p.ID, s.Target)
				}
			}
			if s.Kind == stepSurge && (s.Value < 1 || s.Value > 10) {
				t.Fatalf("%s: surge %.1f di luar rentang sim-control", p.ID, s.Value)
			}
			if s.Kind == stepWeather && (s.Value < 0.2 || s.Value > 2) {
				t.Fatalf("%s: weather %.2f di luar rentang sim-control", p.ID, s.Value)
			}
		}
	}
	if !hasKill {
		t.Fatalf("minimal satu preset harus memuat langkah chaos kill + pulih")
	}
	if len(demoPresets) < 2 || len(demoPresets) > 3 {
		t.Fatalf("jumlah preset = %d, want 2–3", len(demoPresets))
	}
}

func TestReplayProxyPassthroughEncoding(t *testing.T) {
	var blob bytes.Buffer
	zw := gzip.NewWriter(&blob)
	zw.Write([]byte(`{"meta":{}}`))
	zw.Close()
	var gotAE string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAE = r.Header.Get("Accept-Encoding")
		if r.URL.Path != "/api/replay/sessions/live" {
			t.Errorf("path upstream = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(blob.Bytes())
	}))
	defer up.Close()

	proxy := httptest.NewServer(replayProxy(up.URL, up.Client()))
	defer proxy.Close()
	// Accept-Encoding diset eksplisit → transport Go tidak mendekompresi
	// otomatis, sehingga blob mentah bisa dibandingkan byte-per-byte.
	req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/api/replay/sessions/live", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := proxy.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(body, blob.Bytes()) {
		t.Fatalf("body berubah di proxy (%d vs %d byte)", len(body), blob.Len())
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding hilang di proxy")
	}
	if !strings.Contains(gotAE, "gzip") {
		t.Fatalf("Accept-Encoding upstream = %q, want gzip", gotAE)
	}
}

func TestReplayProxyUpstreamDown(t *testing.T) {
	proxy := httptest.NewServer(replayProxy("http://127.0.0.1:1", http.DefaultClient))
	defer proxy.Close()
	resp, err := proxy.Client().Get(proxy.URL + "/api/replay/sessions")
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}
