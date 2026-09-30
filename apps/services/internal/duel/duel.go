// Package duel menjalankan dua engine simulasi identik (seed sama, config sama)
// yang dibedakan HANYA oleh strategi dispatch — bukti A/B yang adil.
//
// Jaminan keadilan (spesifikasi Fase 3): SATU generator order (RNG seed sama)
// menghasilkan setiap order SEKALI dan menyuntikkannya ke KEDUA engine pada
// tick yang sama via InjectExternal — bukan dua generator terpisah. Kedua
// engine menerima input byte-identical; rider wander RNG bersifat per-engine
// (deterministik per sisi, boleh berbeda antar strategi karena state armada
// memang berbeda).
//
// Run() murni: request sama → hasil sama (determinisme diuji di duel_test).
package duel

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/sim"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

const (
	tickMs           = 100 // 10 Hz — sama dengan engine live
	frameEveryMs     = 2000
	maxFramesPerSide = 600
	bins             = 24
)

// Preset = skenario beban bernama (armada + demand + TTL).
type Preset struct {
	Name       string
	Riders     int
	RatePerMin float64
	TTLSec     float64
	Seconds    int
	Note       string
}

// Kapasitas armada 100 rider di POI Berlin terukur ±25–30 order/menit
// (trip POI acak pendek — terukur di duel smoke Fase 3). Preset diposisikan
// di bawah/sekitar/di atas kapasitas agar delta strategi bermakna,
// bukan degenerate (semua strategi sama baiknya saat beban lengang).
var presets = map[string]Preset{
	"steady": {Name: "steady", Riders: 100, RatePerMin: 20, TTLSec: 90, Seconds: 300,
		Note: "demand ±70% kapasitas armada — delta kecepatan terlihat bersih"},
	"flash": {Name: "flash", Riders: 100, RatePerMin: 30, TTLSec: 120, Seconds: 600,
		Note: "beban di atas kapasitas ala flash sale — antrean & expiry mulai membedakan"},
	"rush": {Name: "rush", Riders: 100, RatePerMin: 45, TTLSec: 240, Seconds: 600,
		Note: "overload dinner rush ±1,5× — throughput & expiry pembeda utama"},
}

func PresetByName(name string) (Preset, error) {
	p, ok := presets[name]
	if !ok {
		return Preset{}, fmt.Errorf("preset tidak dikenal %q (pilihan: %v)", name, PresetNames())
	}
	return p, nil
}

func PresetNames() []string {
	return []string{"flash", "rush", "steady"}
}

// Request param duel; Seed 0 → 42. Seconds kosong → default preset.
type Request struct {
	StrategyA string `json:"strategy_a"`
	StrategyB string `json:"strategy_b"`
	Preset    string `json:"preset"`
	Seconds   int    `json:"seconds,omitempty"`
	Seed      int64  `json:"seed,omitempty"`
}

// Side = hasil satu engine.
type Side struct {
	Strategy         string           `json:"strategy"`
	Created          int              `json:"created"`
	Delivered        int              `json:"delivered"`
	Expired          int              `json:"expired"`
	DeliveryP50Ms    float64          `json:"delivery_p50_ms"`
	DeliveryP95Ms    float64          `json:"delivery_p95_ms"`
	UtilizationPct   float64          `json:"utilization_pct"`   // rider-ms on-task / total rider-ms
	TaskKm           float64          `json:"task_km"`           // total km on-task armada
	CostPerOrderKm   float64          `json:"cost_per_order_km"` // km on-task per order terkirim (1 unit biaya = 1 km)
	ThroughputPerMin float64          `json:"throughput_per_min"`
	DeliveryDursMs   []int64          `json:"delivery_durs_ms,omitempty"`
	Frames           []model.Snapshot `json:"frames,omitempty"`
}

// Histogram overlay bin BERSAMA untuk A & B (satu sumbu, spesifikasi lab).
// Batas atas = p95 gabungan agar outlier tidak memipihkan distribusi.
type Histogram struct {
	EdgesMS []float64 `json:"edges_ms"`
	CountA  []int     `json:"count_a"`
	CountB  []int     `json:"count_b"`
}

// Result duel lengkap — direstorasi via GET /api/lab/results/{id}.
type Result struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"` // done | error
	Error          string     `json:"error,omitempty"`
	StrategyA      string     `json:"strategy_a"`
	StrategyB      string     `json:"strategy_b"`
	Preset         string     `json:"preset"`
	PresetNote     string     `json:"preset_note,omitempty"`
	Seed           int64      `json:"seed"`
	Seconds        int        `json:"seconds"`
	Riders         int        `json:"riders"`
	RatePerMin     float64    `json:"rate_per_min"`
	TTLSec         float64    `json:"ttl_sec"`
	CreatedAt      time.Time  `json:"created_at"`
	DurationWallMs int64      `json:"duration_wall_ms"`
	A              *Side      `json:"a,omitempty"`
	B              *Side      `json:"b,omitempty"`
	Histogram      *Histogram `json:"histogram,omitempty"`
}

// Progress di-update goroutine runner saat duel jalan; pembaca HTTP aman race.
func Run(g *graph.Graph, id string, req Request, progress *atomic.Int64) (*Result, error) {
	if req.Seed == 0 {
		req.Seed = 42
	}
	preset, err := PresetByName(req.Preset)
	if err != nil {
		return nil, err
	}
	stratA, err := dispatch.ByName(req.StrategyA)
	if err != nil {
		return nil, fmt.Errorf("strategy_a: %w", err)
	}
	stratB, err := dispatch.ByName(req.StrategyB)
	if err != nil {
		return nil, fmt.Errorf("strategy_b: %w", err)
	}
	seconds := req.Seconds
	if seconds <= 0 {
		seconds = preset.Seconds
	}

	cfg := sim.DefaultConfig()
	cfg.Seed = req.Seed
	cfg.Riders = preset.Riders
	cfg.OrderRatePerMin = preset.RatePerMin // laju GENERATOR duel (engine dibekukan)
	cfg.OrderTTLSec = preset.TTLSec

	res, err := runDuel(g, id, cfg, stratA, stratB, seconds, progress)
	if res != nil {
		res.Preset = preset.Name
		res.PresetNote = preset.Note
	}
	return res, err
}

// RunWithConfig menjalankan duel dengan config penuh — dipakai tooling & test.
// cfg.OrderRatePerMin di sini adalah laju GENERATOR (bukan engine); cfg.Seed
// menentukan seluruh determinisme duel.
func RunWithConfig(g *graph.Graph, id string, cfg sim.Config, stratA, stratB dispatch.Strategy,
	seconds int, progress *atomic.Int64) (*Result, error) {
	if stratA == nil || stratB == nil {
		return nil, fmt.Errorf("strategi duel tidak boleh nil")
	}
	return runDuel(g, id, cfg, stratA, stratB, seconds, progress)
}

func runDuel(g *graph.Graph, id string, cfg sim.Config, stratA, stratB dispatch.Strategy,
	seconds int, progress *atomic.Int64) (*Result, error) {
	seed := cfg.Seed
	if seed == 0 {
		seed = 42
	}
	rate := cfg.OrderRatePerMin
	seconds = clampSeconds(seconds)

	engCfg := cfg
	engCfg.Seed = seed
	engCfg.OrderRatePerMin = 0 // generator internal MATI — order hanya dari duel
	engA, err := sim.New(g, engCfg, stratA)
	if err != nil {
		return nil, err
	}
	engB, err := sim.New(g, engCfg, stratB)
	if err != nil {
		return nil, err
	}

	res := &Result{
		ID: id, Status: "done",
		StrategyA: stratA.Name(), StrategyB: stratB.Name(),
		Seed: seed, Seconds: seconds,
		Riders: engCfg.Riders, RatePerMin: rate, TTLSec: engCfg.OrderTTLSec,
		CreatedAt: time.Now().UTC(),
	}

	// Generator tunggal — pipe order yang sama ke kedua engine.
	gen := rand.New(rand.NewSource(seed))
	nextSpawnMs := 500 + int64(gen.Float64()*800) // settle, sama dengan engine
	durationMs := int64(seconds) * 1000
	nextFrameMs := int64(frameEveryMs)
	var framesA, framesB []model.Snapshot
	startWall := time.Now()

	for t := int64(0); t < durationMs; t += tickMs {
		for nextSpawnMs <= t {
			nextSpawnMs += expDelayMs(gen, rate)
			o, ok := makeOrder(g, gen, t)
			if !ok {
				continue
			}
			if _, rid := engA.InjectExternal(o); rid != -1 {
				return nil, fmt.Errorf("engine A menerima order %s tanpa antre (rider %d) — duel tidak valid", o.ID, rid)
			}
			if _, rid := engB.InjectExternal(o); rid != -1 {
				return nil, fmt.Errorf("engine B menerima order %s tanpa antre (rider %d) — duel tidak valid", o.ID, rid)
			}
		}
		engA.Tick(tickMs)
		engB.Tick(tickMs)
		if t+tickMs >= nextFrameMs && len(framesA) < maxFramesPerSide {
			framesA = append(framesA, engA.Snapshot())
			framesB = append(framesB, engB.Snapshot())
			nextFrameMs += frameEveryMs
		}
		if progress != nil {
			progress.Store(int64(float64(t+tickMs) / float64(durationMs) * 100))
		}
	}

	mA, mB := engA.Metrics(), engB.Metrics()
	if mA.Created != mB.Created {
		return nil, fmt.Errorf("duel input divergen: engine A %d order vs B %d", mA.Created, mB.Created)
	}
	res.A = buildSide(stratA.Name(), seconds, mA, framesA)
	res.B = buildSide(stratB.Name(), seconds, mB, framesB)
	res.Histogram = buildHistogram(mA.DeliveryDursMs, mB.DeliveryDursMs)
	res.DurationWallMs = time.Since(startWall).Milliseconds()
	if progress != nil {
		progress.Store(100)
	}
	return res, nil
}

func clampSeconds(s int) int {
	if s < 60 {
		return 60
	}
	if s > 1800 {
		return 1800
	}
	return s
}

func expDelayMs(gen *rand.Rand, ratePerMin float64) int64 {
	lambdaPerMs := ratePerMin / 60000
	if lambdaPerMs <= 0 {
		return math.MaxInt64 / 4
	}
	d := int64(gen.ExpFloat64() / lambdaPerMs)
	if d < 1 {
		d = 1
	}
	return d
}

// makeOrder meniru spawnOrder engine: pickup & dropoff POI acak, trip ≥ 400 m
// (koordinat POI asli → nearestPOI engine memetakan balik ke POI yang sama,
// jadi kedua engine melihat pickup/dropoff node identik).
func makeOrder(g *graph.Graph, gen *rand.Rand, t int64) (model.ExternalOrder, bool) {
	pickup := g.RandomPOI(gen)
	dropoff := -1
	for i := 0; i < 25; i++ {
		cand := g.RandomPOI(gen)
		if cand == pickup {
			continue
		}
		d := graph.HaversineM(g.NodeLat(pickup), g.NodeLon(pickup), g.NodeLat(cand), g.NodeLon(cand))
		if d >= 400 {
			dropoff = cand
			break
		}
		if dropoff < 0 {
			dropoff = cand
		}
	}
	if dropoff < 0 || dropoff == pickup {
		return model.ExternalOrder{}, false
	}
	return model.ExternalOrder{
		ID:         fmt.Sprintf("d%06d", t) + fmt.Sprintf("-%d", gen.Intn(1_000_000)),
		PickupLat:  g.NodeLat(pickup),
		PickupLon:  g.NodeLon(pickup),
		DropoffLat: g.NodeLat(dropoff),
		DropoffLon: g.NodeLon(dropoff),
		CreatedMs:  t,
		RiderID:    -1,
	}, true
}

func buildSide(name string, seconds int, m sim.Metrics, frames []model.Snapshot) *Side {
	sorted := append([]int64(nil), m.DeliveryDursMs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	s := &Side{
		Strategy:       name,
		Created:        m.Created,
		Delivered:      m.Delivered,
		Expired:        m.Expired,
		DeliveryP50Ms:  percentile(sorted, 50),
		DeliveryP95Ms:  percentile(sorted, 95),
		TaskKm:         m.TaskDistM / 1000,
		DeliveryDursMs: sorted,
		Frames:         frames,
	}
	if m.RiderMs > 0 {
		s.UtilizationPct = float64(m.BusyMs) / float64(m.RiderMs) * 100
	}
	if m.Delivered > 0 {
		s.CostPerOrderKm = m.TaskDistM / 1000 / float64(m.Delivered)
	}
	if minutes := float64(seconds) / 60; minutes > 0 {
		s.ThroughputPerMin = float64(m.Delivered) / minutes
	}
	return s
}

// buildHistogram: bin bersama (sama sumbu) untuk A & B — render statis frontend.
func buildHistogram(a, b []int64) *Histogram {
	all := append(append([]int64(nil), a...), b...)
	if len(all) == 0 {
		return nil
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	hi := percentile(all, 95)
	if hi <= 0 {
		hi = float64(all[len(all)-1])
	}
	edges := make([]float64, bins+1)
	for i := 0; i <= bins; i++ {
		edges[i] = hi * float64(i) / bins
	}
	return &Histogram{EdgesMS: edges, CountA: binCounts(a, edges), CountB: binCounts(b, edges)}
}

func binCounts(data []int64, edges []float64) []int {
	counts := make([]int, len(edges)-1)
	sorted := append([]int64(nil), data...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, d := range sorted {
		idx := sort.SearchFloat64s(edges, float64(d)) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(counts) {
			idx = len(counts) - 1
		}
		counts[idx]++
	}
	return counts
}

func percentile(sorted []int64, p float64) float64 {
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
