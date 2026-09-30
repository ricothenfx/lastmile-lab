// fixturegen — merekam dataset replay untuk fallback frontend (DoD Fase 1).
//
// Menjalankan engine simulasi yang sama secara full-speed (virtual clock,
// deterministik via seed) dan menulis snapshot berjadwal ke JSON:
//
//	go run ./tools/fixturegen -out ../web/public/fixtures/replay-sample.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/sim"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

type fixtureFile struct {
	Meta   fixtureMeta      `json:"meta"`
	Frames []model.Snapshot `json:"frames"`
}

type fixtureMeta struct {
	City       string    `json:"city"`
	Source     string    `json:"source"`
	BBox       []float64 `json:"bbox"`
	CapturedAt string    `json:"capturedAt"`
	Hz         int       `json:"hz"`
	Riders     int       `json:"riders"`
	RatePerMin float64   `json:"ratePerMin"`
	Seed       int64     `json:"seed"`
	Strategy   string    `json:"strategy"`
}

func main() {
	out := flag.String("out", "../web/public/fixtures/replay-sample.json", "output path")
	graphPath := flag.String("graph", "rider-sim/data/berlin_graph.json", "path graph JSON (relatif apps/services)")
	seconds := flag.Int("seconds", 45, "durasi simulasi (detik virtual)")
	hz := flag.Int("hz", 5, "frekuensi frame fixture")
	riders := flag.Int("riders", 60, "jumlah rider")
	rate := flag.Float64("rate", 30, "order per menit (beban uji 60fps DoD)")
	seed := flag.Int64("seed", 7, "seed RNG")
	flag.Parse()

	raw, err := os.ReadFile(*graphPath)
	if err != nil {
		log.Fatalf("read graph: %v", err)
	}
	g, err := graph.Load(bytes.NewReader(raw))
	if err != nil {
		log.Fatalf("load graph: %v", err)
	}
	cfg := sim.DefaultConfig()
	cfg.Riders = *riders
	cfg.OrderRatePerMin = *rate
	cfg.Seed = *seed
	engine, err := sim.New(g, cfg, sim.FIFO{})
	if err != nil {
		log.Fatalf("init sim: %v", err)
	}

	tickMs := int64(1000 / cfg.TickHz)
	everyTicks := (1000 / *hz) / int(tickMs)
	if everyTicks < 1 {
		everyTicks = 1
	}
	frames := make([]model.Snapshot, 0, (*seconds)*(*hz))
	total := (*seconds) * cfg.TickHz
	for i := 0; i < total; i++ {
		engine.Tick(tickMs)
		if (i+1)%everyTicks == 0 {
			frames = append(frames, engine.Snapshot())
		}
	}

	fx := fixtureFile{
		Meta: fixtureMeta{
			City:       g.Meta.City,
			Source:     g.Meta.Source,
			BBox:       g.Meta.BBox,
			CapturedAt: time.Now().UTC().Format(time.RFC3339),
			Hz:         *hz,
			Riders:     *riders,
			RatePerMin: *rate,
			Seed:       *seed,
			Strategy:   "fifo",
		},
		Frames: frames,
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(f)
	if err := enc.Encode(fx); err != nil {
		log.Fatal(err)
	}
	fi, _ := f.Stat()
	f.Close()

	last := frames[len(frames)-1]
	log.Printf("wrote %s (%d KB): %d frames, delivered=%d expired=%d active=%d",
		*out, fi.Size()/1024, len(frames), last.Stats.Delivered, last.Stats.Expired, last.Stats.Active)
}
