// Package dryrun — simulator sebagai judge untuk plan Advisor (Fase 7).
//
// Setiap plan di-dry-run di jalur duel EXISTING (internal/duel, ADR D18):
// dua engine identik seed sama — baseline (kondisi live saat ini) vs plan
// (strategi/surge/cuaca dari usulan LLM). SATU generator order men-Pipe
// order yang sama ke kedua engine — keadilan dijamin konstruksi. LLM tidak
// pernah mengeksekusi apa pun; yang dieksekusi adalah simulator deterministik.
//
// Pemetaan aksi plan → engine (satu-satunya tempat mapping ini):
//   - strategy {name}      → strategi dispatch sisi plan (dispatch.ByName)
//   - surge {factor}       → skala laju generator: rate_live × factor/surge_live
//   - weather {factor}     → WeatherFactor engine sisi plan
//   - kill {target}        → TIDAK bisa disimulasikan deterministik (kill =
//     incident runtime) → plan dilewati dengan Note jujur, bukan angka palsu.
package dryrun

import (
	"context"
	"fmt"
	"math"

	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/advise"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/duel"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/sim"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
)

// LiveState kondisi live yang menjadi baseline (dari /api/kpi; nilai
// fallback deterministik bila metrik tak terukur).
type LiveState struct {
	Strategy   string
	Surge      float64
	Weather    float64
	RatePerMin float64 // rolling orders/menit terukur; ≤0 → default 20
}

// Metrics subset angka duel yang dilaporkan (tanpa frame — payload kecil).
type Metrics struct {
	Created          int     `json:"created"`
	Delivered        int     `json:"delivered"`
	Expired          int     `json:"expired"`
	DeliveryP50Ms    float64 `json:"delivery_p50_ms"`
	DeliveryP95Ms    float64 `json:"delivery_p95_ms"`
	CostPerOrderKm   float64 `json:"cost_per_order_km"`
	UtilizationPct   float64 `json:"utilization_pct"`
	ThroughputPerMin float64 `json:"throughput_per_min"`
}

// PlanResult hasil dry-run satu plan.
type PlanResult struct {
	Plan      advise.Plan `json:"plan"`
	Baseline  *Metrics    `json:"baseline"`            // identik antar plan (seed sama)
	Predicted *Metrics    `json:"predicted,omitempty"` // nil untuk plan kill-only
	Note      string      `json:"note,omitempty"`      // mis. aksi kill tak tersimulasi
}

// Result keseluruhan satu panggilan advise.
type Result struct {
	Seed        int64        `json:"seed"`
	Seconds     int          `json:"seconds"`
	Baseline    *Metrics     `json:"baseline"`
	BaseRate    float64      `json:"base_rate_per_min"`
	PlanResults []PlanResult `json:"plans"`
}

const (
	// durasi duel pendek agar interaktif (spec fase 7: 60–120 s virtual).
	virtualSeconds = 120
	minRate        = 1.0
	maxRate        = 120.0
)

// defaultBaseCfg config engine baseline dari kondisi live: armada 100 rider,
// TTL 90 s (konfigurasi demo), laju generator = laju order terukur.
// seed di-set pemanggil (SATU seed untuk baseline & semua plan).
func defaultBaseCfg(live LiveState) sim.Config {
	cfg := sim.DefaultConfig()
	cfg.Riders = 100
	cfg.OrderTTLSec = 90
	cfg.OrderRatePerMin = clampRate(live.RatePerMin) // laju GENERATOR (engine dibekukan duel)
	cfg.SurgeFactor = live.Surge
	cfg.WeatherFactor = live.Weather
	return cfg
}

// Run mengeksekusi dry-run semua plan (duel per plan, seed sama).
// Murni terhadap engine — tanpa I/O network; graph dibawa dari startup.
func Run(ctx context.Context, g *graph.Graph, live LiveState, seed int64, plans []advise.Plan) (*Result, error) {
	if g == nil {
		return nil, fmt.Errorf("graph nil — dry-run tidak bisa jalan")
	}
	if seed == 0 {
		seed = 42
	}
	baseStrat, err := dispatch.ByName(live.Strategy)
	if err != nil {
		baseStrat, err = dispatch.ByName("fifo") // fallback deterministik
		if err != nil {
			return nil, err
		}
	}
	baseRate := clampRate(live.RatePerMin)
	if live.Surge <= 0 {
		live.Surge = 1
	}
	if live.Weather <= 0 {
		live.Weather = 1
	}

	baseCfg := defaultBaseCfg(live)
	baseCfg.Seed = seed

	out := &Result{Seed: seed, Seconds: virtualSeconds, BaseRate: baseRate}
	var baseline *Metrics
	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pr := PlanResult{Plan: p}
		strat, cfg, simNote, simErr := planEngine(baseCfg, live, p)
		pr.Note = simNote
		if simErr != nil {
			pr.Note = joinNote(pr.Note, simErr.Error())
			out.PlanResults = append(out.PlanResults, pr)
			continue
		}
		res, err := duel.RunWithConfig(g, "copilot", cfg, baseStrat, strat, virtualSeconds, nil)
		if err != nil {
			return nil, fmt.Errorf("dry-run plan %q: %w", p.Name, err)
		}
		if baseline == nil {
			baseline = toMetrics(res.A, virtualSeconds)
			out.Baseline = baseline
		}
		pr.Baseline = baseline
		pr.Predicted = toMetrics(res.B, virtualSeconds)
		out.PlanResults = append(out.PlanResults, pr)
	}
	if out.Baseline == nil {
		return nil, fmt.Errorf("tidak ada plan yang bisa di-dry-run")
	}
	return out, nil
}

// planEngine memetakan aksi plan → config engine sisi plan.
// Plan tanpa aksi tersimulasi (mis. hanya kill) → simErr menjelaskan.
func planEngine(base sim.Config, live LiveState, p advise.Plan) (dispatch.Strategy, sim.Config, string, error) {
	cfg := base
	stratName := live.Strategy
	notes := []string{}
	rate := base.OrderRatePerMin
	simulatable := false

	for _, a := range p.Actions {
		switch a.Kind {
		case advise.KindStrategy:
			if _, err := dispatch.ByName(a.Params["name"]); err == nil {
				stratName = a.Params["name"]
				simulatable = true
			}
		case advise.KindSurge:
			if f, ok := advise.ParamFloat(a.Params, "factor"); ok && f > 0 {
				// surge menskalakan demand — live surge menjadi penyebut agar
				// perbandingan baseline vs plan merepresentasikan DELTA surge.
				rate = clampRate(base.OrderRatePerMin * f / live.Surge)
				simulatable = true
			}
		case advise.KindWeather:
			if f, ok := advise.ParamFloat(a.Params, "factor"); ok && f >= 0.3 && f <= 1 {
				cfg.WeatherFactor = f
				simulatable = true
			}
		case advise.KindKill:
			notes = append(notes, fmt.Sprintf("aksi kill %q tidak tersimulasi dry-run (incident runtime) — nilai eksekusinya lihat Incident Timeline", a.Params["target"]))
		}
	}
	if !simulatable {
		return nil, cfg, joinNote(notes...), fmt.Errorf("tidak ada aksi tersimulasi")
	}
	strat, err := dispatch.ByName(stratName)
	if err != nil {
		return nil, cfg, joinNote(notes...), err
	}
	cfg.OrderRatePerMin = rate
	return strat, cfg, joinNote(notes...), nil
}

func clampRate(r float64) float64 {
	if math.IsNaN(r) || r < minRate {
		return minRate
	}
	if r > maxRate {
		return maxRate
	}
	return r
}

func toMetrics(s *duel.Side, seconds int) *Metrics {
	if s == nil {
		return nil
	}
	m := &Metrics{
		Created:        s.Created,
		Delivered:      s.Delivered,
		Expired:        s.Expired,
		DeliveryP50Ms:  s.DeliveryP50Ms,
		DeliveryP95Ms:  s.DeliveryP95Ms,
		CostPerOrderKm: s.CostPerOrderKm,
		UtilizationPct: s.UtilizationPct,
	}
	if seconds > 0 {
		m.ThroughputPerMin = float64(s.Delivered) / (float64(seconds) / 60)
	}
	return m
}

func joinNote(notes ...string) string {
	out := ""
	for i, n := range notes {
		if n == "" {
			continue
		}
		if out != "" {
			out += " · "
		}
		out += fmt.Sprintf("%d) %s", i+1, n)
	}
	return out
}
