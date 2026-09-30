package main

import (
	"testing"
	"time"
)

// Percentile nearest-rank — konsisten dengan internal/duel.
func TestPercentileSorted(t *testing.T) {
	empty := []int64{}
	if percentileSorted(empty, 95) != 0 {
		t.Fatal("empty harus 0")
	}
	one := []int64{42}
	if percentileSorted(one, 50) != 42 || percentileSorted(one, 99) != 42 {
		t.Fatal("satu sampel")
	}
	v := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := percentileSorted(v, 50); got != 50 {
		t.Fatalf("p50=%v, want 50", got)
	}
	if got := percentileSorted(v, 95); got != 100 {
		t.Fatalf("p95=%v, want 100", got)
	}
	if got := percentileSorted(v, 99); got != 100 {
		t.Fatalf("p99=%v, want 100", got)
	}
}

func TestRollingPerMin(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	// 60 sampel dalam 59 detik, created naik 40 → 40/menit.
	samples := make([]counterSample, 0, 60)
	for i := 0; i < 60; i++ {
		samples = append(samples, counterSample{
			at:      now.Add(-59*time.Second + time.Duration(i)*time.Second),
			created: i * 40 / 59, delivered: i,
		})
	}
	samples[len(samples)-1].created = 40
	if got := rollingPerMin(samples, time.Minute, now); got == nil || *got < 39 || *got > 41 {
		t.Fatalf("rate=%v, want ~40", got)
	}
	// < 5 detik data → null (belum stabil).
	short := []counterSample{{at: now.Add(-2 * time.Second), created: 5}, {at: now, created: 9}}
	if got := rollingPerMin(short, time.Minute, now); got != nil {
		t.Fatalf("short window harus nil, dapat %v", *got)
	}
	// satu sampel → nil.
	if got := rollingPerMin(samples[:1], time.Minute, now); got != nil {
		t.Fatal("satu sampel harus nil")
	}
	// counter mundur (service restart) tidak menghasilkan angka negatif.
	rev := []counterSample{{at: now.Add(-30 * time.Second), created: 100}, {at: now, created: 10}}
	if got := rollingPerMin(rev, time.Minute, now); got == nil || *got != 0 {
		t.Fatalf("reverse harus clamp 0, dapat %v", got)
	}
}

func gridFixture() []gridNode {
	return []gridNode{
		{Name: "rider-sim", Group: "core", Status: "up"},
		{Name: "ws-gateway", Group: "core", Status: "down"},
		{Name: "api-gateway", Group: "core", Status: "up"},
		{Name: "strategy-lab", Group: "lab", Status: "up"},
		{Name: "order-ingestion", Group: "pipeline", Status: "down"},
		{Name: "dispatch-consumer", Group: "pipeline", Status: "down"},
	}
}

func TestBuildSLO(t *testing.T) {
	simOK := kpiSim{Reachable: true, DeliverySamples: 10, DeliveryP95Ms: 120_000}
	pipeStandby := kpiPipeline{}
	grid := gridFixture()
	slo := buildSLO(simOK, pipeStandby, grid, nil)
	if len(slo) != 4 {
		t.Fatalf("harap 4 SLO, dapat %d", len(slo))
	}
	// p95 ok
	if *slo[0].OK != true {
		t.Fatal("p95 120s harus ok")
	}
	// zero loss: pipeline standby → ok nil
	if slo[1].OK != nil {
		t.Fatal("zero loss standby harus ok=nil")
	}
	// grid health: ws-gateway down → false, value menyebut ws-gateway
	if slo[2].OK == nil || *slo[2].OK != false {
		t.Fatal("grid health harus false")
	}
	if got := *slo[2].Value; got != "down: ws-gateway" {
		t.Fatalf("grid value = %q", got)
	}
	// availability: chaos absent → nilai standby, ok nil
	if slo[3].OK != nil || *slo[3].Value != "chaos standby" {
		t.Fatalf("availability salah: %+v", slo[3])
	}

	// pelanggaran p95 (> 360 s) + budget terlampaui.
	simBad := kpiSim{Reachable: true, DeliverySamples: 10, DeliveryP95Ms: 400_000}
	budget := &errorBudget{Availability: 99.5, BudgetPct: 0.1, OK: false, WindowSec: 600, DowntimeSec: 3}
	slo2 := buildSLO(simBad, pipeStandby, grid, budget)
	if *slo2[0].OK != false {
		t.Fatal("p95 400s harus violation")
	}
	if *slo2[3].OK != false {
		t.Fatal("availability 99.5% harus violation")
	}
}

func TestAssembleKPI(t *testing.T) {
	eng := simEngineKPI{
		NowMs: 123_000, Riders: 60, IdleRiders: 20,
		OrdersActive: 8, OrdersWaiting: 5,
		Created: 100, Delivered: 90, Expired: 2,
		Surge: 2, Weather: 1, Strategy: "fifo",
		TaskDistM: 90_000, BusyMs: 3_000_000, RiderMs: 7_380_000,
		DeliveryRecentMs: []int64{30_000, 120_000, 60_000},
		DispatchRecentNs: []int64{1_000_000, 3_000_000, 9_000_000},
		DispatchCalls:    3,
	}
	ing := map[string]interface{}{
		"counters": map[string]interface{}{"published": float64(1000), "inflight": float64(2), "publish_errors": float64(0)},
	}
	con := map[string]interface{}{
		"counters": map[string]interface{}{"consumed": float64(995), "duplicates_local": float64(3), "parse_errors": float64(2), "db_errors": float64(0)},
	}
	in := kpiInput{
		now: time.UnixMilli(1696118400000), eng: eng, simReachable: true,
		ing: ing, ingOK: true, con: con, conOK: true,
		grid: gridFixture(),
	}
	r := assembleKPI(in)
	if r.Source != "live" {
		t.Fatalf("source=%s", r.Source)
	}
	if r.Sim.UtilizationPct <= 0 || r.Sim.UtilizationPct >= 100 {
		t.Fatalf("util=%v", r.Sim.UtilizationPct)
	}
	if r.Sim.CostPerOrderKm != 1.0 { // 90 km / 90 order
		t.Fatalf("cost=%v", r.Sim.CostPerOrderKm)
	}
	if r.Sim.DeliveryP95Ms != 120_000 {
		t.Fatalf("p95=%v", r.Sim.DeliveryP95Ms)
	}
	if r.Sim.P99DispatchMs != 9.0 {
		t.Fatalf("p99 dispatch=%v", r.Sim.P99DispatchMs)
	}
	if r.Pipeline.LagMsgs == nil || *r.Pipeline.LagMsgs != 0 { // 1000-995-3-2
		t.Fatalf("lag=%v", r.Pipeline.LagMsgs)
	}
	if r.Queue.WaitingOrders != 5 || *r.Queue.IngestionInflight != 2 {
		t.Fatalf("queue salah: %+v", r.Queue)
	}
	if r.ErrorBudget != nil || r.IncidentsOpen != nil {
		t.Fatal("tanpa chaos: budget & incidents harus nil")
	}

	// dengan chaos: budget + open incidents ikut.
	in.chaosReachable = true
	var ci chaosIncidents
	ci.Summary.WindowSec = 120
	ci.Summary.DowntimeTotalSec = 4.8
	ci.Summary.AvailabilityPct = 96 // 2 target down 4.8s dari 120s×2
	ci.Summary.BudgetPct = 0.1
	ci.Incidents = append(ci.Incidents, struct {
		ID       string `json:"id"`
		Target   string `json:"target"`
		Kind     string `json:"kind"`
		TStart   int64  `json:"t_start"`
		TDetect  int64  `json:"t_detect"`
		TRecover int64  `json:"t_recover"`
	}{ID: "inc-1", TRecover: 0})
	in.chaos = &ci
	r2 := assembleKPI(in)
	if r2.ErrorBudget == nil || r2.ErrorBudget.ConsumedPct != 4 {
		t.Fatalf("budget salah: %+v", r2.ErrorBudget)
	}
	if r2.IncidentsOpen == nil || *r2.IncidentsOpen != 1 {
		t.Fatalf("open=%v", r2.IncidentsOpen)
	}

	// sim mati → source partial, angka sim tetap zero-value (UI tampil "—").
	in2 := kpiInput{now: time.UnixMilli(1), ing: nil, con: nil, grid: gridFixture()}
	r3 := assembleKPI(in2)
	if r3.Source != "partial" || r3.Sim.Reachable {
		t.Fatalf("partial salah: %+v", r3.Sim)
	}
}

func TestGetU64(t *testing.T) {
	m := map[string]interface{}{
		"counters": map[string]interface{}{"published": float64(7)},
	}
	if got := getU64(m, "counters", "published"); got == nil || *got != 7 {
		t.Fatalf("getU64=%v", got)
	}
	if getU64(m, "counters", "absent") != nil || getU64(nil, "counters", "x") != nil {
		t.Fatal("absent harus nil")
	}
}
