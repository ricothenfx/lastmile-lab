// bench — harness p99 keputusan dispatch (Fase 3, manual — BUKAN gate CI).
//
// Mengukur waktu Strategy.Assign() per tick pada beban target (default
// ≥ 100 order aktif + 100 rider idle) untuk KEEMPAT strategi, memakai
// koordinat nyata dari graph Berlin (rider di node acak, pickup di POI acak).
//
// Jalankan (di VPS, via docker dengan cap — lihat reports/phase-03-bench.md):
//
//	go run ./tools/bench -orders 100 -riders 100 -ticks 400 -markdown
//	go run ./tools/bench -orders 300 -riders 300 -ticks 100 -markdown   # headroom
//
// Output markdown siap tempel ke reports/phase-03-bench.md.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// defaultGraphPaths dicari berurutan (CWD = apps/services atau repo root).
var defaultGraphPaths = []string{
	"rider-sim/data/berlin_graph.json",
	"../rider-sim/data/berlin_graph.json",
	"apps/services/rider-sim/data/berlin_graph.json",
	"/app/data/berlin_graph.json",
}

func main() {
	ordersN := flag.Int("orders", 100, "jumlah order aktif per tick")
	ridersN := flag.Int("riders", 100, "jumlah rider idle per tick")
	ticks := flag.Int("ticks", 400, "jumlah pemanggilan Assign terukur per strategi")
	warmup := flag.Int("warmup", 20, "pemanggilan pemanasan (tidak diukur)")
	seed := flag.Int64("seed", 7, "seed posisi/umur sintetis")
	large := flag.Bool("large", true, "jalankan juga skenario 3× beban target (headroom)")
	graphPath := flag.String("graph", "", "path berlin_graph.json (default: auto)")
	flag.Parse()

	g := loadGraph(*graphPath)
	fmt.Printf("<!-- bench dispatch: go %s GOMAXPROCS=%d graph=%d nodes/%d POIs seed=%d -->\n",
		runtime.Version(), runtime.GOMAXPROCS(0), g.NodeCount(), len(g.POIs()), *seed)

	rows := []scenario{
		{name: "target", orders: *ordersN, riders: *ridersN, ticks: *ticks},
	}
	if *large {
		rows = append(rows, scenario{name: "3× target", orders: *ordersN * 3, riders: *ridersN * 3, ticks: *ticks / 2})
	}

	fmt.Println()
	fmt.Println("| Skenario | Strategi | n order | n rider | mean ms | p50 ms | p95 ms | p99 ms | max ms |")
	fmt.Println("|---|---|---:|---:|---:|---:|---:|---:|---:|")
	for _, sc := range rows {
		for _, name := range dispatch.Names() {
			strat, err := dispatch.ByName(name)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			r := benchStrategy(strat, g, sc.orders, sc.riders, *ticks, *warmup, *seed)
			fmt.Printf("| %s | `%s` | %d | %d | %.3f | %.3f | %.3f | %.3f | %.3f |\n",
				sc.name, name, sc.orders, sc.riders, r.mean, r.p50, r.p95, r.p99, r.max)
		}
	}
}

type scenario struct {
	name   string
	orders int
	riders int
	ticks  int
}

type result struct {
	mean, p50, p95, p99, max float64 // ms
}

func benchStrategy(strat dispatch.Strategy, g *graph.Graph, nOrders, nRiders, ticks, warmup int, seed int64) result {
	gen := rand.New(rand.NewSource(seed))
	riders := makeRiders(g, gen, nRiders)
	orders := makeOrders(g, gen, nOrders)

	for i := 0; i < warmup; i++ { // pemanasan cache/alokasi
		strat.Assign(cloneOrders(orders), cloneRiders(riders))
	}

	durs := make([]float64, 0, ticks)
	for i := 0; i < ticks; i++ {
		o, r := cloneOrders(orders), cloneRiders(riders)
		// umur order bergulir per tick — pola realistis antrean berjalan
		for j := range o {
			o[j].CreatedMs += 100
			o[j].NowMs += 100
		}
		t0 := time.Now()
		strat.Assign(o, r)
		durs = append(durs, float64(time.Since(t0).Nanoseconds())/1e6)
	}
	sort.Float64s(durs)
	pct := func(p float64) float64 {
		idx := int(p/100*float64(len(durs))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(durs) {
			idx = len(durs) - 1
		}
		return durs[idx]
	}
	var sum float64
	for _, d := range durs {
		sum += d
	}
	return result{mean: sum / float64(len(durs)), p50: pct(50), p95: pct(95), p99: pct(99), max: durs[len(durs)-1]}
}

func makeRiders(g *graph.Graph, gen *rand.Rand, n int) []dispatch.RiderView {
	out := make([]dispatch.RiderView, n)
	for i := range out {
		node := gen.Intn(g.NodeCount())
		out[i] = dispatch.RiderView{
			ID: i, Status: model.RiderIdle,
			Lat: g.NodeLat(node), Lon: g.NodeLon(node),
		}
	}
	return out
}

func makeOrders(g *graph.Graph, gen *rand.Rand, n int) []dispatch.OrderView {
	pois := g.POIs()
	out := make([]dispatch.OrderView, n)
	now := time.Now().UnixMilli()
	for i := range out {
		p := pois[gen.Intn(len(pois))]
		out[i] = dispatch.OrderView{
			ID:        fmt.Sprintf("b%06d", i),
			CreatedMs: now - int64(gen.Intn(45_000)),
			PickupLat: g.NodeLat(p), PickupLon: g.NodeLon(p),
			NowMs: now,
		}
	}
	return out
}

func cloneOrders(src []dispatch.OrderView) []dispatch.OrderView {
	out := make([]dispatch.OrderView, len(src))
	copy(out, src)
	return out
}

func cloneRiders(src []dispatch.RiderView) []dispatch.RiderView {
	out := make([]dispatch.RiderView, len(src))
	copy(out, src)
	return out
}

func loadGraph(path string) *graph.Graph {
	paths := []string{path}
	if path == "" {
		paths = defaultGraphPaths
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		g, err := graph.Load(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load graph %s: %v\n", p, err)
			os.Exit(1)
		}
		return g
	}
	fmt.Fprintf(os.Stderr, "graph tidak ditemukan (coba: %s)\n", strings.Join(defaultGraphPaths, ", "))
	os.Exit(1)
	return nil
}
