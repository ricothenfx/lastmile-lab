// rider-sim — inti simulasi Fase 1.
//
// Road network Berlin (OSM) di-embed; N rider bergerak di graph dengan status
// idle/to_pickup/pickup/delivering; order Poisson + dispatch FIFO via interface
// Strategy. Tick internal 10 Hz (virtual clock), state expose via HTTP:
//
//	GET /healthz          liveness + ringkas statistik
//	GET /internal/state   snapshot penuh (dipakai ws-gateway & api-gateway)
//	GET /api/graph        graph JSON mentah (debugging)
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/sim"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

//go:embed data/berlin_graph.json
var graphJSON []byte

func main() {
	port := envStr("PORT", "4201")
	cfg := sim.DefaultConfig()
	cfg.Riders = envInt("RIDERS", cfg.Riders)
	cfg.OrderRatePerMin = envFloat("ORDER_RATE_PER_MIN", cfg.OrderRatePerMin)
	cfg.SpeedMPS = envFloat("SPEED_MPS", cfg.SpeedMPS)
	cfg.TickHz = envInt("TICK_HZ", cfg.TickHz)
	cfg.OrderTTLSec = envFloat("ORDER_TTL_SEC", cfg.OrderTTLSec)
	cfg.WeatherFactor = envFloat("WEATHER_FACTOR", cfg.WeatherFactor)
	cfg.Seed = int64(envInt("SEED", 42))

	g, err := graph.Load(strings.NewReader(string(graphJSON)))
	if err != nil {
		log.Fatalf("load graph: %v", err)
	}
	log.Printf("graph: %d nodes, %d POIs, bbox %v", g.NodeCount(), len(g.POIs()), g.Meta.BBox)

	engine, err := sim.New(g, cfg, sim.FIFO{})
	if err != nil {
		log.Fatalf("init sim: %v", err)
	}

	tickInterval := time.Second / time.Duration(cfg.TickHz)
	go func() {
		t := time.NewTicker(tickInterval)
		defer t.Stop()
		for range t.C {
			engine.Tick(int64(time.Second/time.Duration(cfg.TickHz)) / int64(time.Millisecond))
		}
	}()

	start := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		snap := engine.Snapshot()
		writeJSON(w, http.StatusOK, model.Health{
			OK: true, Service: "rider-sim",
			UptimeSec: int64(time.Since(start).Seconds()),
			Detail: fmt.Sprintf(`riders=%d active_orders=%d delivered=%d expired=%d strategy=%s tick_hz=%d`,
				len(snap.Riders), snap.Stats.Active, snap.Stats.Delivered, snap.Stats.Expired,
				snap.Stats.Strategy, cfg.TickHz),
		})
	})
	mux.HandleFunc("/internal/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, engine.FullSnapshot())
	})
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, engine.FullSnapshot())
	})
	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(graphJSON)
	})

	addr := "0.0.0.0:" + port
	log.Printf("rider-sim listening on %s (tick %v, riders %d, rate %.1f/min, seed %d)",
		addr, tickInterval, cfg.Riders, cfg.OrderRatePerMin, cfg.Seed)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
