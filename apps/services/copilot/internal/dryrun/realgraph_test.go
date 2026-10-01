package dryrun

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/advise"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
)

// TestRunRealGraphWallTime: dry-run di graph Berlin PENUH (9 459 node) —
// bukti interaktivitas (spec: duel pendek 60–120 s virtual harus nyaman
// dalam satu request). Dilewati bila file graph tidak ada (mis. checkout
// parsial) — graph kosong bukan kegagalan fitur.
func TestRunRealGraphWallTime(t *testing.T) {
	const path = "../../../rider-sim/data/berlin_graph.json"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("graph tidak ditemukan (%v) — skip", err)
	}
	defer f.Close()
	g, err := graph.Load(f)
	if err != nil {
		t.Fatal(err)
	}
	live := LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 34}
	start := time.Now()
	res, err := Run(context.Background(), g, live, 42, []advise.Plan{{
		Name:      "optimal",
		Rationale: "uji",
		Actions: []advise.Action{
			{Kind: advise.KindStrategy, Params: map[string]string{"name": "optimal"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wall := time.Since(start)
	t.Logf("dry-run 120s virtual ×2 engine di graph penuh: %s wall (delivered %d→%d, p95 %.0f→%.0f ms)",
		wall, res.Baseline.Delivered, res.PlanResults[0].Predicted.Delivered,
		res.Baseline.DeliveryP95Ms, res.PlanResults[0].Predicted.DeliveryP95Ms)
	if wall > 10*time.Second {
		t.Fatalf("dry-run harus interaktif (<10 s), dapat %s", wall)
	}
	if res.Baseline.Created != res.PlanResults[0].Predicted.Created {
		b, _ := json.Marshal(res)
		t.Fatalf("input tidak identik: %s", b)
	}
}
