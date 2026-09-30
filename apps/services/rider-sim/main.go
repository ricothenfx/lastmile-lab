// rider-sim — inti simulasi (Fase 1 + pipeline Fase 2).
//
// Road network Berlin (OSM) di-embed; N rider bergerak di graph dengan status
// idle/to_pickup/pickup/delivering; order Poisson + dispatch FIFO via interface
// Strategy. Tick internal 10 Hz (virtual clock), state expose via HTTP:
//
//	GET /healthz             liveness + ringkas statistik
//	GET /internal/state      snapshot penuh (dipakai ws-gateway & api-gateway)
//	GET /api/graph           graph JSON mentah (debugging)
//	POST /internal/orders    injeksi order pipeline (hanya ORDER_SOURCE=pipeline)
//	GET|POST /internal/control  baca/ubah surge & weather live (sim-control)
//
// ORDER_SOURCE=internal (default) → generator Poisson internal, demo tidak
// pernah mati tanpa infra. ORDER_SOURCE=pipeline → generator mati, order hanya
// dari dispatch-consumer via Kafka.
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
	cfg.SurgeFactor = envFloat("SURGE_FACTOR", cfg.SurgeFactor)
	cfg.Seed = int64(envInt("SEED", 42))
	source := envStr("ORDER_SOURCE", "internal")
	if source != "internal" && source != "pipeline" {
		log.Fatalf("ORDER_SOURCE harus internal|pipeline, dapat %q", source)
	}
	if source == "pipeline" {
		cfg.OrderRatePerMin = 0 // generator internal mati — order hanya via pipeline
	}

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
			Detail: fmt.Sprintf(`source=%s riders=%d active_orders=%d delivered=%d expired=%d strategy=%s tick_hz=%d surge=%.1f weather=%.2f`,
				source, len(snap.Riders), snap.Stats.Active, snap.Stats.Delivered, snap.Stats.Expired,
				snap.Stats.Strategy, cfg.TickHz, snap.Stats.SurgeFactor, snap.Stats.WeatherFactor),
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

	// Injeksi order pipeline — aktif hanya saat sumber = pipeline (409 bila
	// internal, supaya salah konfigurasi terlihat, bukan diam-diam duplikat).
	mux.HandleFunc("/internal/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if source != "pipeline" {
			http.Error(w, `{"error":"order_source_internal"}`, http.StatusConflict)
			return
		}
		var o model.ExternalOrder
		if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
			http.Error(w, `{"error":"bad_json"}`, http.StatusBadRequest)
			return
		}
		result, riderID := engine.InjectExternal(o)
		writeJSON(w, http.StatusOK, map[string]interface{}{"result": result, "rider": riderID})
	})

	mux.HandleFunc("/internal/control", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			surge, weather := engine.Controls()
			writeJSON(w, http.StatusOK, map[string]float64{"surge": surge, "weather": weather})
		case http.MethodPost:
			var c model.ControlRequest
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				http.Error(w, `{"error":"bad_json"}`, http.StatusBadRequest)
				return
			}
			engine.SetControls(c.Surge, c.Weather)
			surge, weather := engine.Controls()
			writeJSON(w, http.StatusOK, map[string]float64{"surge": surge, "weather": weather})
		default:
			http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	addr := "0.0.0.0:" + port
	log.Printf("rider-sim listening on %s (source=%s, tick %v, riders %d, rate %.1f/min, seed %d)",
		addr, source, tickInterval, cfg.Riders, cfg.OrderRatePerMin, cfg.Seed)
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
