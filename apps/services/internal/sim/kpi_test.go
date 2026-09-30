package sim

import (
	"strconv"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
)

// KPI ring harus terisi dari eksekusi nyata: dispatch terukur saat ada order
// + rider idle, delivery terukur saat order selesai. Tanpa trafik → kosong.
func TestKPITracksDispatchAndDelivery(t *testing.T) {
	g := tinyGraph(t)
	e, err := New(g, cfgFast(), dispatch.FIFO{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := 0; i < 600; i++ {
		e.Tick(100)
	}
	k := e.KPI()
	if k.DispatchCalls == 0 || len(k.DispatchRecentNs) == 0 {
		t.Fatalf("dispatch ring kosong: calls=%d", k.DispatchCalls)
	}
	for _, d := range k.DispatchRecentNs {
		if d <= 0 {
			t.Fatalf("durasi dispatch <= 0: %d", d)
		}
	}
	if k.Delivered == 0 || len(k.DeliveryRecentMs) == 0 {
		t.Fatalf("delivery ring kosong: delivered=%d", k.Delivered)
	}
	for _, d := range k.DeliveryRecentMs {
		if d <= 0 {
			t.Fatalf("durasi delivery <= 0: %d", d)
		}
	}
	if k.Created == 0 {
		t.Fatalf("created tidak konsisten: %+v", k)
	}
	if k.RiderMs <= 0 || k.BusyMs <= 0 {
		t.Fatalf("util harus terukur: busy=%d riderms=%d", k.BusyMs, k.RiderMs)
	}
}

// Ring berkapasitas tetap: tidak tumbuh tanpa batas. Armada besar + order
// stabil supaya ada titik keputusan (order menunggu × rider idle) tiap tick.
func TestKPIRingBounded(t *testing.T) {
	g := tinyGraph(t)
	cfg := cfgFast()
	cfg.Riders = 120
	cfg.OrderRatePerMin = 1200
	cfg.SpeedMPS = 200 // rider cepat selesai → sering idle → titik keputusan rapat
	cfg.IdleSpeedMPS = 100
	e, err := New(g, cfg, dispatch.FIFO{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := 0; i < 2000; i++ {
		e.Tick(100)
	}
	k := e.KPI()
	if len(k.DispatchRecentNs) != KPIRingCap {
		t.Fatalf("dispatch ring = %d, harap %d", len(k.DispatchRecentNs), KPIRingCap)
	}
	if len(k.DeliveryRecentMs) > KPIRingCap {
		t.Fatalf("delivery ring melebihi kapasitas: %d", len(k.DeliveryRecentMs))
	}
	if k.DispatchCalls < uint64(KPIRingCap) {
		t.Fatalf("dispatch calls %d < cap", k.DispatchCalls)
	}
}

// copyRing: urutan oldest→newest benar saat ring belum penuh dan penuh (wrap).
func TestCopyRingOrder(t *testing.T) {
	e := &Engine{}
	ring := []int64{10, 20, 30, 0}
	got := e.copyRing(ring, 3, 3) // belum penuh: 10,20,30
	want := []int64{10, 20, 30}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// penuh: tulis 10,20,30,40 lalu 50 menimpa idx1 → oldest di idx=1.
	ring = []int64{10, 50, 30, 40}
	got = e.copyRing(ring, 1, 5)
	want = []int64{50, 30, 40, 10}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrap: got %v want %v", got, want)
		}
	}
}

// Instrumentasi tidak boleh mengubah determinisme: dua engine same-seed
// menghasilkan urutan keputusan identik (timing wall-clock di luar logika).
func TestKPIInstrumentationDeterminism(t *testing.T) {
	g := tinyGraph(t)
	run := func() []string {
		e, err := New(g, cfgFast(), dispatch.FIFO{})
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		var out []string
		for i := 0; i < 400; i++ {
			e.Tick(100)
			for _, d := range e.FullSnapshot().Decisions {
				out = append(out, d.OrderID+"|"+strconv.Itoa(d.RiderID))
			}
		}
		return out
	}
	a, b := run(), run()
	if len(a) == 0 {
		t.Fatal("tidak ada keputusan — test tidak bermakna")
	}
	if len(a) != len(b) {
		t.Fatalf("jumlah keputusan beda: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("keputusan %d beda: %q vs %q", i, a[i], b[i])
		}
	}
}

// Guard: waktu nyata tidak bocor ke KPI (jam virtual), dan tanpa trafik
// ring dispatch kosong.
func TestKPIVirtualClock(t *testing.T) {
	g := tinyGraph(t)
	e, err := New(g, Config{Seed: 1, Riders: 2, OrderRatePerMin: 0, TickHz: 10, MinTripM: 1}, dispatch.FIFO{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	e.Tick(250)
	k := e.KPI()
	if k.NowMs != 250 {
		t.Fatalf("NowMs=%d, want 250", k.NowMs)
	}
	if k.DispatchCalls != 0 || len(k.DispatchRecentNs) != 0 {
		t.Fatalf("tanpa order dispatch ring harus kosong: %d", k.DispatchCalls)
	}
	if k.RiderMs != 2*250 {
		t.Fatalf("RiderMs=%d, want 500", k.RiderMs)
	}
}
