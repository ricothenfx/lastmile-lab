package duel

import (
	"math/rand"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/sim"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
)

func tinyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	// segiempat dengan diagonal: 0-1-3-2-0 + 1-2 (POI di 0 dan 3)
	meta := graph.Meta{City: "DuelTest", Source: "test", BBox: []float64{0, 0, 1, 1}}
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

func duelCfg(rate float64) sim.Config {
	c := sim.DefaultConfig()
	c.Seed = 7
	c.Riders = 8
	c.OrderRatePerMin = rate // laju generator duel
	c.PickupDwellSec = 0.2
	c.DropoffDwellSec = 0.1
	c.OrderTTLSec = 30
	c.MinTripM = 300
	return c
}

func TestDuelRunsAndDeliversIdenticalInput(t *testing.T) {
	g := tinyGraph(t)
	var prog atomic.Int64
	res, err := RunWithConfig(g, "lab-test", duelCfg(600), dispatch.FIFO{}, dispatch.NewZone(0, 0), 60, &prog)
	if err != nil {
		t.Fatal(err)
	}
	if res.A == nil || res.B == nil {
		t.Fatal("kedua sisi harus ada")
	}
	if res.A.Created != res.B.Created || res.A.Created == 0 {
		t.Fatalf("input duel harus identik & non-nol: %d vs %d", res.A.Created, res.B.Created)
	}
	if res.A.Delivered == 0 {
		t.Fatal("duel 60s (rate 600/min) harus menghasilkan delivery")
	}
	if len(res.A.Frames) == 0 || len(res.A.Frames) != len(res.B.Frames) {
		t.Fatalf("frames A/B harus kembar: %d vs %d", len(res.A.Frames), len(res.B.Frames))
	}
	if res.A.Strategy != "fifo" || res.B.Strategy != "zone" {
		t.Fatalf("nama strategi salah: %s vs %s", res.A.Strategy, res.B.Strategy)
	}
	if res.Histogram == nil || len(res.Histogram.EdgesMS) != bins+1 ||
		len(res.Histogram.CountA) != bins || len(res.Histogram.CountB) != bins {
		t.Fatal("histogram bin bersama harus terisi (bins+1 edge)")
	}
	if res.A.UtilizationPct <= 0 || res.A.UtilizationPct > 100 {
		t.Fatalf("utilization tidak wajar: %.2f", res.A.UtilizationPct)
	}
	if res.A.CostPerOrderKm <= 0 {
		t.Fatal("cost/order (km on-task per order) harus > 0")
	}
	if prog.Load() != 100 {
		t.Fatalf("progress harus 100, dapat %d", prog.Load())
	}
	if res.DurationWallMs <= 0 {
		t.Fatal("duration wall harus tercatat")
	}
}

func TestDuelDeterministicSameRequest(t *testing.T) {
	g := tinyGraph(t)
	run := func() *Result {
		res, err := RunWithConfig(g, "lab-det", duelCfg(600), dispatch.FIFO{}, dispatch.Optimal{}, 60, nil)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	a, b := run(), run()
	// Semua field deterministik KECUALI waktu dinding (CreatedAt/DurationWallMs).
	a.CreatedAt, b.CreatedAt = time.Time{}, time.Time{}
	a.DurationWallMs, b.DurationWallMs = 0, 0
	if !reflect.DeepEqual(a, b) {
		t.Fatal("duel request sama harus menghasilkan hasil identik (deterministik)")
	}
}

func TestDuelRejectsNilStrategyAndUnknownNames(t *testing.T) {
	g := tinyGraph(t)
	if _, err := RunWithConfig(g, "x", duelCfg(60), nil, dispatch.FIFO{}, 60, nil); err == nil {
		t.Fatal("strategi nil harus error")
	}
	if _, err := Run(g, "x", Request{StrategyA: "fifo", StrategyB: "gpt", Preset: "steady"}, nil); err == nil {
		t.Fatal("strategi tak dikenal harus error")
	}
	if _, err := Run(g, "x", Request{StrategyA: "fifo", StrategyB: "fifo", Preset: "nope"}, nil); err == nil {
		t.Fatal("preset tak dikenal harus error")
	}
}

func TestDuelClampsSeconds(t *testing.T) {
	if clampSeconds(1) != 60 || clampSeconds(99_999) != 1800 || clampSeconds(300) != 300 {
		t.Fatal("clamp seconds salah")
	}
}

func TestExpDelayMsPositive(t *testing.T) {
	gen := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		if d := expDelayMs(gen, 60); d < 1 {
			t.Fatalf("delay harus ≥ 1 ms, dapat %d", d)
		}
	}
}
