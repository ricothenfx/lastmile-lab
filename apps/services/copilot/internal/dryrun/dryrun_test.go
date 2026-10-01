package dryrun

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/advise"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
)

func tinyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	meta := graph.Meta{City: "DryrunTest", Source: "test", BBox: []float64{0, 0, 1, 1}}
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

var testPlans = []advise.Plan{
	{Name: "ganti optimal", Rationale: "uji", Actions: []advise.Action{
		{Kind: advise.KindStrategy, Params: map[string]string{"name": "optimal"}},
	}},
	{Name: "chaos drill", Rationale: "uji", Actions: []advise.Action{
		{Kind: advise.KindKill, Params: map[string]string{"target": "lastmile-ws-gateway"}},
	}},
}

func TestRunDeterministicSameSeed(t *testing.T) {
	g := tinyGraph(t)
	live := LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 300}
	a, err := Run(context.Background(), g, live, 7, testPlans)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(context.Background(), g, live, 7, testPlans)
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("dry-run seed sama harus deterministik (D18)")
	}
}

func TestRunFairnessIdenticalInput(t *testing.T) {
	g := tinyGraph(t)
	res, err := Run(context.Background(), g, LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 300}, 7, testPlans[:1])
	if err != nil {
		t.Fatal(err)
	}
	if res.Baseline == nil || res.PlanResults[0].Predicted == nil {
		t.Fatal("baseline + predicted wajib ada")
	}
	if res.Baseline.Created != res.PlanResults[0].Predicted.Created {
		t.Fatalf("input duel harus identik: %d vs %d", res.Baseline.Created, res.PlanResults[0].Predicted.Created)
	}
	if res.Baseline != res.PlanResults[0].Baseline {
		t.Fatal("baseline harus pointer sama antar plan (satu seed per advise)")
	}
}

func TestRunKillOnlyPlanSkippedHonestly(t *testing.T) {
	g := tinyGraph(t)
	res, err := Run(context.Background(), g, LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 300}, 7, testPlans)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PlanResults) != 2 {
		t.Fatalf("harus 2 entri hasil, dapat %d", len(res.PlanResults))
	}
	kill := res.PlanResults[1]
	if kill.Predicted != nil {
		t.Fatal("plan kill-only TIDAK boleh punya angka prediksi palsu")
	}
	if kill.Note == "" || !json.Valid([]byte(`"x"`)) {
		t.Fatal("plan kill-only wajib dijelaskan di Note")
	}
}

func TestPlanEngineMapping(t *testing.T) {
	base := defaultBaseCfg(LiveState{Surge: 2, RatePerMin: 20})
	live := LiveState{Strategy: "fifo", Surge: 2, Weather: 1, RatePerMin: 20}
	p := advise.Plan{Actions: []advise.Action{
		{Kind: advise.KindSurge, Params: map[string]string{"factor": "4"}},
		{Kind: advise.KindWeather, Params: map[string]string{"factor": "0.6"}},
		{Kind: advise.KindStrategy, Params: map[string]string{"name": "zone"}},
	}}
	strat, cfg, note, err := planEngine(base, live, p)
	if err != nil || note != "" {
		t.Fatalf("mapping harus bersih: %v %q", err, note)
	}
	if strat.Name() != "zone" {
		t.Fatalf("strategi harus zone, dapat %s", strat.Name())
	}
	// surge 4× dari live 2× → laju generator 20 × 4/2 = 40/menit
	if cfg.OrderRatePerMin != 40 {
		t.Fatalf("rate harus 40 (20×4/2), dapat %.1f", cfg.OrderRatePerMin)
	}
	if cfg.WeatherFactor != 0.6 {
		t.Fatalf("weather harus 0.6, dapat %.2f", cfg.WeatherFactor)
	}
}

func TestPlanEngineKillOnlyError(t *testing.T) {
	base := defaultBaseCfg(LiveState{RatePerMin: 20})
	live := LiveState{Strategy: "fifo", Surge: 1, Weather: 1, RatePerMin: 20}
	_, _, note, err := planEngine(base, live, testPlans[1])
	if err == nil {
		t.Fatal("plan kill-only harus tidak tersimulasi")
	}
	if note == "" {
		t.Fatal("note harus menjelaskan aksi kill")
	}
}

func TestClampRate(t *testing.T) {
	for in, want := range map[float64]float64{0: 1, -5: 1, 5: 5, 1000: 120} {
		if got := clampRate(in); got != want {
			t.Fatalf("clampRate(%v) = %v, harap %v", in, got, want)
		}
	}
}

func TestRunNilGraph(t *testing.T) {
	if _, err := Run(context.Background(), nil, LiveState{RatePerMin: 20}, 7, testPlans); err == nil {
		t.Fatal("graph nil harus error")
	}
}
