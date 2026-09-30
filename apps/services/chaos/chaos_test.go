package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeNow(offsetSec *int) func() time.Time {
	start := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return start.Add(time.Duration(*offsetSec) * time.Second) }
}

// Ditolak keras: apa pun di luar allowlist (infra ber-state, chaos sendiri,
// container stack lain, wildcard) tidak boleh lolos.
func TestAllowlistDeny(t *testing.T) {
	for _, name := range []string{
		"postgres", "redis", "redpanda", "chaos", "loadgen", "db-migrate",
		"redpanda-init", "lastmile-rider-sim", "rider-sim ", "", "RIDER-SIM",
		"turnaround-prod", "aviation", "ro-botriv", "*", "all",
	} {
		if _, ok := targetByName(name); ok {
			t.Fatalf("target %q tidak boleh ada di allowlist", name)
		}
	}
	for _, want := range allowlistTargets {
		if !strings.HasPrefix(want.Container, "lastmile-") {
			t.Fatalf("container %s tidak berprefiks lastmile-", want.Container)
		}
		if got, ok := targetByName(want.Name); !ok || got.Container != want.Container {
			t.Fatalf("lookup %s gagal", want.Name)
		}
	}
}

// Mesin status: sehat → 2 kegagalan beruntun membuka incident (flap
// suppression: 1 kegagalan belum), t_start=terakhir sehat, t_detect=kini →
// sehat lagi menutup (t_recover).
func TestTrackerIncidentLifecycle(t *testing.T) {
	off := 0
	tr := newTracker(time.Second, fakeNow(&off))
	tr.probe("rider-sim", true) // baseline sehat
	off += 10
	tr.probe("rider-sim", false) // streak 1 — belum incident
	if got := tr.incidentList(); len(got) != 0 {
		t.Fatalf("1 kegagalan tidak boleh membuka incident: %+v", got)
	}
	off += 1
	tr.probe("rider-sim", false) // streak 2 — incident terbuka
	inc := tr.incidentList()
	if len(inc) != 1 || inc[0].Kind != "health" {
		t.Fatalf("incident tidak terbuka: %+v", inc)
	}
	if inc[0].TStart != fakeNow(&off)().Add(-11*time.Second).UnixMilli() {
		t.Fatalf("t_start salah: %d", inc[0].TStart)
	}
	if inc[0].MTTDMs() != 11000 {
		t.Fatalf("MTTD salah: %d", inc[0].MTTDMs())
	}
	off += 4
	if closed := tr.probe("rider-sim", true); closed == nil {
		t.Fatal("recovery harus menutup incident")
	} else {
		if closed.TRecover == 0 || closed.MTTRMs() != 4000 {
			t.Fatalf("MTTR salah: %d", closed.MTTRMs())
		}
	}
	if len(tr.incidentList()) != 1 {
		t.Fatalf("incident list = %d", len(tr.incidentList()))
	}
	if s := tr.summary(0.1); s.Total != 1 || s.Open != 0 || s.MTTRAvgMs != 4000 {
		t.Fatalf("summary salah: %+v", s)
	}
}

// Target yang belum pernah sehat (standby — pipeline mati) TIDAK membuka
// incident saat probe gagal; ini mencegah incident abadi di mode demo.
func TestTrackerStandbyNoIncident(t *testing.T) {
	off := 0
	tr := newTracker(time.Second, fakeNow(&off))
	for i := 0; i < 5; i++ {
		tr.probe("dispatch-consumer", false)
	}
	if got := tr.incidentList(); len(got) != 0 {
		t.Fatalf("standby tidak boleh membuka incident: %+v", got)
	}
	tv := tr.targets()
	for _, v := range tv {
		if v.Name == "dispatch-consumer" && v.Status != "standby" {
			t.Fatalf("status = %s, want standby", v.Status)
		}
	}
}

// openChaos menolak target down / standby; incident chaos-kill berjalan
// sampai monitor menutupnya (t_detect diisi monitor saat probe gagal).
func TestTrackerChaosKillFlow(t *testing.T) {
	off := 0
	tr := newTracker(time.Second, fakeNow(&off))
	if _, err := tr.openChaos("rider-sim", "test"); !errors.Is(err, errNotHealthy) {
		t.Fatalf("harap ditolak (never seen), dapat %v", err)
	}
	tr.probe("rider-sim", true)
	off += 2
	inc, err := tr.openChaos("rider-sim", "SIGKILL test")
	if err != nil {
		t.Fatalf("openChaos: %v", err)
	}
	if inc.Kind != "chaos-kill" || inc.TDetect != 0 {
		t.Fatalf("incident chaos salah: %+v", inc)
	}
	if _, err := tr.openChaos("rider-sim", "lagi"); !errors.Is(err, errNotHealthy) {
		t.Fatalf("double kill harus ditolak, dapat %v", err)
	}
	off += 1
	tr.probe("rider-sim", false) // monitor melihat kegagalan → t_detect
	if inc.TDetect != fakeNow(&off)().UnixMilli() {
		t.Fatalf("t_detect belum diisi monitor: %d", inc.TDetect)
	}
	off += 4
	tr.probe("rider-sim", true) // self-heal → t_recover
	if inc.TRecover == 0 || inc.MTTRMs() != 4000 {
		t.Fatalf("MTTR salah: %d", inc.MTTRMs())
	}
	if inc.MTTDMs() != 1000 {
		t.Fatalf("MTTD salah: %d", inc.MTTDMs())
	}
	s := tr.summary(0.1)
	if s.Kills != 1 || s.Total != 1 {
		t.Fatalf("summary: %+v", s)
	}
}

// Restart lebih cepat dari probe: t_detect kosong → diisi saat recovery.
func TestTrackerFastRecovery(t *testing.T) {
	off := 0
	tr := newTracker(time.Second, fakeNow(&off))
	tr.probe("ws-gateway", true)
	inc, err := tr.openChaos("ws-gateway", "kill")
	if err != nil {
		t.Fatalf("openChaos: %v", err)
	}
	off += 3 // selesai sebelum probe gagal teramati
	tr.probe("ws-gateway", true)
	if inc.TDetect == 0 || inc.TRecover == 0 || inc.TRecover != inc.TDetect {
		t.Fatalf("fast recovery: detect=%d recover=%d", inc.TDetect, inc.TRecover)
	}
}

// Persist: simpan → muat kembali, round-trip identik.
func TestIncidentStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incidents.json")
	s := &incidentStore{path: path}
	list := []*Incident{
		{ID: "inc-a", Target: "rider-sim", Kind: "chaos-kill", TStart: 1000, TDetect: 1500, TRecover: 5300, Detail: "x"},
		{ID: "inc-b", Target: "ws-gateway", Kind: "health", TStart: 2000, TDetect: 2500},
	}
	s.save(list)
	got := (&incidentStore{path: path}).load()
	if len(got) != 2 || got[0].ID != "inc-a" || got[1].MTTRMs() != 0 {
		t.Fatalf("round-trip salah: %+v", got)
	}
}

// Handler /kill: body rusak → 400; target asing → 403 (bukan 404 —
// eksplisit ditolak, bukan "tidak ditemukan").
func TestKillHandlerValidation(t *testing.T) {
	svc := &service{tr: newTracker(time.Second, nil), docker: newDockerClient("/nonexistent.sock"), store: &incidentStore{}}
	mux := svc.mux()

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/kill", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(`{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", rec.Code)
	}
	if rec := post(`{"target":"postgres"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("infra harus 403, dapat %d", rec.Code)
	}
	if rec := post(`{"target":"turnaround-prod"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("stack lain harus 403, dapat %d", rec.Code)
	}
	// docker sock tidak ada → target sah pun gagal terjangkau (bukan kill).
	if rec := post(`{"target":"rider-sim"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("docker unreachable harus 502, dapat %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/kill", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /kill: %d", rec.Code)
	}
}
