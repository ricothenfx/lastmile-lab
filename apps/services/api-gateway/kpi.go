// KPI agregat (Fase 4) — sumber tunggal angka KPI Command Deck + System
// Health. Semua nilai dihitung dari metrik nyata yang sudah ada:
//
//	rider-sim /internal/metrics (Engine.KPI — ring delivery/dispatch live)
//	order-ingestion + dispatch-consumer /metrics (counters pipeline)
//	chaos /incidents (MTTD/MTTR/error budget dari eksperimen nyata)
//	healthz grid (probe berkala, cache 2 s)
//
// Tanpa angka sintetis: nilai yang tidak bisa diukur dikirim null dan UI
// menampilkan "—" (fallback replay/jaringan mati — bukan angka karangan).
package main

import (
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ---- bentuk respons /api/kpi ----

type kpiSim struct {
	Reachable       bool    `json:"reachable"`
	NowMs           int64   `json:"now_ms"`
	Riders          int     `json:"riders"`
	IdleRiders      int     `json:"idle_riders"`
	OrdersActive    int     `json:"orders_active"`
	OrdersWaiting   int     `json:"orders_waiting"`
	Created         int     `json:"orders_created"`
	Delivered       int     `json:"orders_delivered"`
	Expired         int     `json:"orders_expired"`
	Surge           float64 `json:"surge"`
	Weather         float64 `json:"weather"`
	Strategy        string  `json:"strategy"`
	UtilizationPct  float64 `json:"utilization_pct"`
	CostPerOrderKm  float64 `json:"cost_per_order_km"`
	DeliveryP50Ms   float64 `json:"delivery_p50_ms"`
	DeliveryP95Ms   float64 `json:"delivery_p95_ms"`
	DeliverySamples int     `json:"delivery_samples"`
	P50DispatchMs   float64 `json:"p50_dispatch_ms"`
	P99DispatchMs   float64 `json:"p99_dispatch_ms"`
	DispatchCalls   uint64  `json:"dispatch_calls"`
}

// simEngineKPI mirror subset internal/sim.KPI (JSON rider-sim /internal/metrics).
type simEngineKPI struct {
	NowMs            int64   `json:"now_ms"`
	Riders           int     `json:"riders"`
	IdleRiders       int     `json:"idle_riders"`
	OrdersActive     int     `json:"orders_active"`
	OrdersWaiting    int     `json:"orders_waiting"`
	Created          int     `json:"orders_created"`
	Delivered        int     `json:"orders_delivered"`
	Expired          int     `json:"orders_expired"`
	Surge            float64 `json:"surge"`
	Weather          float64 `json:"weather"`
	Strategy         string  `json:"strategy"`
	TaskDistM        float64 `json:"task_dist_m"`
	BusyMs           int64   `json:"busy_ms"`
	RiderMs          int64   `json:"rider_ms"`
	DeliveryRecentMs []int64 `json:"delivery_recent_ms"`
	DispatchRecentNs []int64 `json:"dispatch_recent_ns"`
	DispatchCalls    uint64  `json:"dispatch_calls"`
}

type kpiQueue struct {
	WaitingOrders     int     `json:"waiting_orders"`
	IngestionInflight *uint64 `json:"ingestion_inflight"`
}

type kpiPipeline struct {
	IngestionOK bool    `json:"ingestion_ok"`
	ConsumerOK  bool    `json:"consumer_ok"`
	Published   *uint64 `json:"published"`
	Consumed    *uint64 `json:"consumed"`
	LagMsgs     *uint64 `json:"lag_msgs"`
	PublishErrs *uint64 `json:"publish_errors"`
	DBErrs      *uint64 `json:"db_errors"`
}

type gridNode struct {
	Name      string `json:"name"`
	Group     string `json:"group"`  // core|lab|pipeline|chaos
	Status    string `json:"status"` // up|down|standby
	LatencyMs int64  `json:"latency_ms"`
}

type sloItem struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Target string  `json:"target"`
	OK     *bool   `json:"ok"`
	Value  *string `json:"value"`
}

type errorBudget struct {
	WindowSec    float64 `json:"window_sec"`
	DowntimeSec  float64 `json:"downtime_sec"`
	ConsumedPct  float64 `json:"consumed_pct"`
	BudgetPct    float64 `json:"budget_pct"`
	Availability float64 `json:"availability_pct"`
	OK           bool    `json:"ok"`
}

type kpiResponse struct {
	TSMS          int64        `json:"ts_ms"`
	Source        string       `json:"source"` // live | partial
	Sim           kpiSim       `json:"sim"`
	OrdersPerMin  *float64     `json:"orders_per_min"`
	DelivPerMin   *float64     `json:"delivered_per_min"`
	Queue         kpiQueue     `json:"queue"`
	Pipeline      kpiPipeline  `json:"pipeline"`
	Grid          []gridNode   `json:"grid"`
	SLO           []sloItem    `json:"slo"`
	ErrorBudget   *errorBudget `json:"error_budget"`
	IncidentsOpen *int         `json:"incidents_open"`
}

// ---- helpers murni (tertes) ----

// percentileSorted nearest-rank pada slice TERURUT — konsisten dengan
// internal/duel (sumber angka Strategy Lab).
func percentileSorted(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx])
}

type counterSample struct {
	at        time.Time
	created   int
	delivered int
}

// rollingPerMin: laju per menit dari sampel counter kumulatif dalam window.
// Null bila data < 2 sampel / window terlalu pendek — bukan angka karangan.
func rollingPerMin(samples []counterSample, window time.Duration, now time.Time) *float64 {
	cut := now.Add(-window)
	first := -1
	for i, s := range samples {
		if !s.at.Before(cut) {
			first = i
			break
		}
	}
	if first < 0 || first == len(samples)-1 {
		return nil
	}
	a := samples[first]
	b := samples[len(samples)-1]
	dtMin := b.at.Sub(a.at).Minutes()
	if dtMin < 5.0/60.0 { // < 5 detik → belum stabil
		return nil
	}
	v := float64(b.created-a.created) / dtMin
	if v < 0 {
		v = 0 // counter tidak bisa mundur (service restart)
	}
	return &v
}

func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }
func u64Ptr(v uint64) *uint64 { return &v }

func f1(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// buildSLO mengevaluasi SLO eksplisit fase 4 dari angka yang sudah ada.
// Target (didefinisikan eksplisit, dirinci di reports/phase-04-chaos.md §1):
//  1. delivery_p95: p95 created→delivered < 6 menit (360 000 ms) — target spec
//  2. zero_loss: lag konsumen 0, error publish/db 0 (hanya terukur saat pipeline hidup)
//  3. grid_health: semua service core (rider-sim, ws-gateway, api-gateway) up
//  4. availability: ≥ 99,9% pada window monitor chaos (error budget 0,1%)
func buildSLO(sim kpiSim, pipe kpiPipeline, grid []gridNode, budget *errorBudget) []sloItem {
	slo := make([]sloItem, 0, 4)

	if sim.Reachable && sim.DeliverySamples > 0 {
		slo = append(slo, sloItem{
			ID: "delivery_p95", Label: "Delivery p95", Target: "< 360 s",
			OK:    boolPtr(sim.DeliveryP95Ms < 360_000),
			Value: strPtr(f1(sim.DeliveryP95Ms/1000) + "s"),
		})
	} else {
		slo = append(slo, sloItem{ID: "delivery_p95", Label: "Delivery p95", Target: "< 360 s"})
	}

	if pipe.Published != nil && pipe.Consumed != nil && pipe.LagMsgs != nil {
		ok := *pipe.LagMsgs == 0 &&
			(pipe.PublishErrs == nil || *pipe.PublishErrs == 0) &&
			(pipe.DBErrs == nil || *pipe.DBErrs == 0)
		slo = append(slo, sloItem{
			ID: "zero_loss", Label: "Zero message loss", Target: "lag 0 · err 0",
			OK:    boolPtr(ok),
			Value: strPtr(strconv.FormatUint(*pipe.LagMsgs, 10) + " lag"),
		})
	} else {
		slo = append(slo, sloItem{ID: "zero_loss", Label: "Zero message loss", Target: "lag 0 · err 0", Value: strPtr("pipeline standby")})
	}

	var down []string
	for _, n := range grid {
		if n.Group == "core" && n.Status != "up" {
			down = append(down, n.Name)
		}
	}
	if len(down) == 0 {
		slo = append(slo, sloItem{
			ID: "grid_health", Label: "Grid health", Target: "core up",
			OK: boolPtr(true), Value: strPtr("core ok"),
		})
	} else {
		slo = append(slo, sloItem{
			ID: "grid_health", Label: "Grid health", Target: "core up",
			OK: boolPtr(false), Value: strPtr("down: " + joinNames(down)),
		})
	}

	if budget != nil {
		slo = append(slo, sloItem{
			ID: "availability", Label: "Availability", Target: "≥ 99.9%",
			OK:    boolPtr(budget.OK),
			Value: strPtr(f2(budget.Availability) + "%"),
		})
	} else {
		slo = append(slo, sloItem{ID: "availability", Label: "Availability", Target: "≥ 99.9%", Value: strPtr("chaos standby")})
	}
	return slo
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// ---- aggregator (state di api-gateway) ----

// chaosIncidents adalah subset respons /incidents service chaos yang dipakai.
type chaosIncidents struct {
	Incidents []struct {
		ID       string `json:"id"`
		Target   string `json:"target"`
		Kind     string `json:"kind"`
		TStart   int64  `json:"t_start"`
		TDetect  int64  `json:"t_detect"`
		TRecover int64  `json:"t_recover"`
	} `json:"incidents"`
	Summary struct {
		WindowSec        float64 `json:"window_sec"`
		DowntimeTotalSec float64 `json:"downtime_total_sec"`
		AvailabilityPct  float64 `json:"availability_pct"`
		BudgetPct        float64 `json:"budget_pct"`
		BudgetOK         bool    `json:"budget_ok"`
	} `json:"summary"`
}

// kpiAggregator menyimpan sampel counter untuk laju order + cache respons
// (polling UI 2 Hz tidak menghajar upstream — refresh efektif ≤ 1/gap).
type kpiAggregator struct {
	mu          sync.Mutex
	lastPayload *kpiResponse
	lastAt      time.Time
	samples     []counterSample
}

func (a *kpiAggregator) cached(now time.Time, gap time.Duration) (*kpiResponse, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lastPayload != nil && now.Sub(a.lastAt) < gap {
		return a.lastPayload, true
	}
	return nil, false
}

// store menyimpan payload + mencatat sampel counter (≥900 ms antar sampel).
func (a *kpiAggregator) store(p *kpiResponse, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastPayload, a.lastAt = p, now
	if n := len(a.samples); n == 0 || now.Sub(a.samples[n-1].at) >= 900*time.Millisecond {
		a.samples = append(a.samples, counterSample{at: now, created: p.Sim.Created, delivered: p.Sim.Delivered})
		for len(a.samples) > 0 && now.Sub(a.samples[0].at) > 2*time.Minute {
			a.samples = a.samples[1:]
		}
	}
}

// rates menghitung laju orders/menit & delivered/menit (window 60 s).
func (a *kpiAggregator) rates(now time.Time) (*float64, *float64) {
	a.mu.Lock()
	cp := make([]counterSample, len(a.samples))
	copy(cp, a.samples)
	a.mu.Unlock()
	return rollingPerMin(cp, time.Minute, now), rollingPerMin(cp, time.Minute, now)
}

// gridTarget: satu baris konfigurasi probe grid (urutan = urutan tampil).
type gridTarget struct {
	name  string
	group string
	url   string // kosong = standby (service tidak dikonfigurasi)
}

// probeGrid melakukan GET /healthz paralel ke semua target dengan cache TTL.
func probeGrid(targets []gridTarget, client *http.Client, cache *sync.Map, ttl time.Duration) []gridNode {
	type cached struct {
		node gridNode
		at   time.Time
	}
	out := make([]gridNode, len(targets))
	var wg sync.WaitGroup
	now := time.Now()
	for i, t := range targets {
		if t.url == "" {
			out[i] = gridNode{Name: t.name, Group: t.group, Status: "standby"}
			continue
		}
		if v, ok := cache.Load(t.name); ok {
			c := v.(cached)
			if now.Sub(c.at) < ttl {
				out[i] = c.node
				continue
			}
		}
		wg.Add(1)
		go func(i int, t gridTarget) {
			defer wg.Done()
			start := time.Now()
			ok := false
			if resp, err := client.Get(t.url + "/healthz"); err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				ok = resp.StatusCode == http.StatusOK
			}
			node := gridNode{
				Name: t.name, Group: t.group,
				Status:    map[bool]string{true: "up", false: "down"}[ok],
				LatencyMs: time.Since(start).Milliseconds(),
			}
			out[i] = node
			cache.Store(t.name, cached{node: node, at: time.Now()})
		}(i, t)
	}
	wg.Wait()
	return out
}

// ---- perakitan payload ----

type kpiInput struct {
	now            time.Time
	eng            simEngineKPI
	simReachable   bool
	ing            map[string]interface{}
	ingOK          bool
	con            map[string]interface{}
	conOK          bool
	chaos          *chaosIncidents
	chaosReachable bool
	grid           []gridNode
}

// getU64 membaca counter uint64 dari map metrik pipeline (null bila absen).
func getU64(m map[string]interface{}, counters, key string) *uint64 {
	if m == nil {
		return nil
	}
	cm, ok := m[counters].(map[string]interface{})
	if !ok {
		return nil
	}
	v, ok := cm[key].(float64)
	if !ok {
		return nil
	}
	return u64Ptr(uint64(v))
}

// assembleKPI merangkai payload /api/kpi dari hasil fetch paralel.
// Murni (tidak melakukan I/O) supaya bisa dites langsung.
func assembleKPI(in kpiInput) *kpiResponse {
	sim := kpiSim{Reachable: in.simReachable}
	if in.simReachable {
		sim.NowMs = in.eng.NowMs
		sim.Riders = in.eng.Riders
		sim.IdleRiders = in.eng.IdleRiders
		sim.OrdersActive = in.eng.OrdersActive
		sim.OrdersWaiting = in.eng.OrdersWaiting
		sim.Created = in.eng.Created
		sim.Delivered = in.eng.Delivered
		sim.Expired = in.eng.Expired
		sim.Surge = in.eng.Surge
		sim.Weather = in.eng.Weather
		sim.Strategy = in.eng.Strategy
		sim.DispatchCalls = in.eng.DispatchCalls
		sim.DeliverySamples = len(in.eng.DeliveryRecentMs)
		// Utilisation & cost/order: lifetime dari counter engine nyata.
		if in.eng.RiderMs > 0 {
			sim.UtilizationPct = 100 * float64(in.eng.BusyMs) / float64(in.eng.RiderMs)
		}
		if in.eng.Delivered > 0 {
			sim.CostPerOrderKm = in.eng.TaskDistM / 1000 / float64(in.eng.Delivered)
		}
		if len(in.eng.DeliveryRecentMs) > 0 {
			d := append([]int64(nil), in.eng.DeliveryRecentMs...)
			sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
			sim.DeliveryP50Ms = percentileSorted(d, 50)
			sim.DeliveryP95Ms = percentileSorted(d, 95)
		}
		if len(in.eng.DispatchRecentNs) > 0 {
			d := append([]int64(nil), in.eng.DispatchRecentNs...)
			sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
			sim.P50DispatchMs = percentileSorted(d, 50) / 1e6
			sim.P99DispatchMs = percentileSorted(d, 99) / 1e6
		}
	}

	pipe := kpiPipeline{IngestionOK: in.ingOK, ConsumerOK: in.conOK}
	pub := getU64(in.ing, "counters", "published")
	consumed := getU64(in.con, "counters", "consumed")
	dup := getU64(in.con, "counters", "duplicates_local")
	parse := getU64(in.con, "counters", "parse_errors")
	pipe.Published = pub
	pipe.Consumed = consumed
	pipe.PublishErrs = getU64(in.ing, "counters", "publish_errors")
	pipe.DBErrs = getU64(in.con, "counters", "db_errors")
	// Lag = published − consumed − dup − parse (pesan masih di topik);
	// hanya bermakna saat kedua sisi pipeline hidup.
	if pub != nil && consumed != nil && dup != nil && parse != nil {
		lag := *pub - *consumed - *dup - *parse
		if lag < 0 {
			lag = 0 // consumer bisa di-reset; bukan lag negatif
		}
		pipe.LagMsgs = &lag
	}

	queue := kpiQueue{WaitingOrders: sim.OrdersWaiting}
	if v := getU64(in.ing, "counters", "inflight"); v != nil {
		queue.IngestionInflight = v
	}

	var budget *errorBudget
	var open *int
	if in.chaosReachable && in.chaos != nil {
		s := in.chaos.Summary
		budget = &errorBudget{
			WindowSec:    s.WindowSec,
			DowntimeSec:  s.DowntimeTotalSec,
			ConsumedPct:  0,
			BudgetPct:    s.BudgetPct,
			Availability: s.AvailabilityPct,
			OK:           s.BudgetOK,
		}
		if s.WindowSec > 0 {
			budget.ConsumedPct = 100 * s.DowntimeTotalSec / s.WindowSec
		}
		n := 0
		for _, inc := range in.chaos.Incidents {
			if inc.TRecover <= 0 {
				n++
			}
		}
		open = &n
	}

	resp := &kpiResponse{
		TSMS:          in.now.UnixMilli(),
		Source:        map[bool]string{true: "live", false: "partial"}[in.simReachable],
		Sim:           sim,
		Queue:         queue,
		Pipeline:      pipe,
		Grid:          in.grid,
		ErrorBudget:   budget,
		IncidentsOpen: open,
	}
	resp.SLO = buildSLO(sim, pipe, in.grid, budget)
	return resp
}
