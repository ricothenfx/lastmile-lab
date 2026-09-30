// load-generator — trafik Poisson ke order-ingestion (Fase 2).
//
//	POST /orders ke order-ingestion dengan Idempotency-Key unik per pesan.
//	λ = RATE_PER_MIN × surge / 60 per detik (Poisson: jarak antar-datang Exp).
//	Skenario `spike` ×10 terjadwal (SPIKE_EVERY_SEC / SPIKE_DURATION_SEC).
//	POST /control {"surge":8} — override manual dari sim-control (Surge Console).
//
//	GET /metrics   sent / acked / replay / 429 / errors / latensi p50 p99
//	GET /healthz
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

type latWin struct {
	mu   sync.Mutex
	vals []float64 // ms, rolling
	sum  float64
}

func (l *latWin) add(ms float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vals = append(l.vals, ms)
	l.sum += ms
	if len(l.vals) > 10000 {
		l.sum -= l.vals[0]
		l.vals = l.vals[1:]
	}
}

func (l *latWin) percentile(p float64) (float64, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.vals) == 0 {
		return 0, 0
	}
	sorted := make([]float64, len(l.vals))
	copy(sorted, l.vals)
	for i := 1; i < len(sorted); i++ { // insertion sort ok utk n kecil
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx], len(sorted)
}

type loadgen struct {
	orderURL   string
	timeout    time.Duration
	seed       int64
	idemPrefix string

	mu            sync.Mutex
	manualSurge   float64 // override dari /control (0 = tidak ada)
	surgeNow      float64 // surge efektif saat ini (diagnostik)
	baseRate      float64 // per menit
	spikeFactor   float64
	spikeEverySec int
	spikeDurSec   int
	rng           *rand.Rand
	seq           uint64
	sent          uint64
	acked         uint64
	replay        uint64
	rejected      uint64
	errors        uint64
	inflight      int64
	startedAt     time.Time
	lat           latWin
	sema          chan struct{}
}

func (lg *loadgen) snapshot() map[string]interface{} {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	p50, n := lg.lat.percentile(50)
	p99, _ := lg.lat.percentile(99)
	elapsed := 0.0
	if !lg.startedAt.IsZero() {
		elapsed = time.Since(lg.startedAt).Seconds()
	}
	acked := lg.acked
	sent := lg.sent
	return map[string]interface{}{
		"service":         "loadgen",
		"sent":            sent,
		"acked_201":       acked,
		"replay_200":      lg.replay,
		"rejected_429":    lg.rejected,
		"errors":          lg.errors,
		"inflight":        lg.inflight,
		"rate_target":     lg.baseRate,
		"surge_now":       lg.surgeNow,
		"rate_effective":  lg.baseRate * lg.surgeNow,
		"spike_factor":    lg.spikeFactor,
		"spike_every_sec": lg.spikeEverySec,
		"spike_dur_sec":   lg.spikeDurSec,
		"elapsed_sec":     elapsed,
		"latency_p50_ms":  p50,
		"latency_p99_ms":  p99,
		"latency_samples": n,
		"manual_override": lg.manualSurge,
	}
}

func (lg *loadgen) surgeAt(now time.Time) float64 {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	if lg.manualSurge > 0 {
		lg.surgeNow = lg.manualSurge
		return lg.surgeNow
	}
	if lg.spikeEverySec > 0 {
		cycle := time.Duration(lg.spikeEverySec+lg.spikeDurSec) * time.Second
		phase := now.Sub(lg.startedAt) % cycle
		if phase < time.Duration(lg.spikeDurSec)*time.Second {
			lg.surgeNow = lg.spikeFactor
			return lg.surgeNow
		}
	}
	lg.surgeNow = 1
	return 1
}

func (lg *loadgen) nextID() string {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	lg.seq++
	return fmt.Sprintf("%s%d-%06d", lg.idemPrefix, lg.startedAt.Unix(), lg.seq)
}

func (lg *loadgen) randomOrder() model.OrderMsg {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	var pickup, dropoff [2]float64
	for tries := 0; tries < 25; tries++ {
		pickup = [2]float64{52.487 + lg.rng.Float64()*0.048, 13.375 + lg.rng.Float64()*0.08}
		dropoff = [2]float64{52.487 + lg.rng.Float64()*0.048, 13.375 + lg.rng.Float64()*0.08}
		if haversineM(pickup[0], pickup[1], dropoff[0], dropoff[1]) >= 400 {
			break
		}
	}
	return model.OrderMsg{
		PickupLat: pickup[0], PickupLon: pickup[1],
		DropoffLat: dropoff[0], DropoffLon: dropoff[1],
		Source: "loadgen",
	}
}

func (lg *loadgen) sendOne(ctx context.Context) {
	o := lg.randomOrder()
	o.ID = lg.nextID()
	o.CreatedMs = time.Now().UnixMilli()
	b, _ := json.Marshal(o)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, lg.orderURL+"/orders", strings.NewReader(string(b)))
	if err != nil {
		lg.count(&lg.errors)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", o.ID)
	lg.count(&lg.sent) // setiap attempt dihitung sent (hasil di aked/replay/429/error)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		lg.count(&lg.errors)
		return
	}
	defer resp.Body.Close()
	ms := float64(time.Since(start).Milliseconds())
	lg.lat.add(ms)
	switch {
	case resp.StatusCode == http.StatusCreated:
		lg.count(&lg.acked)
	case resp.StatusCode == http.StatusOK:
		lg.count(&lg.replay)
	case resp.StatusCode == http.StatusTooManyRequests:
		lg.count(&lg.rejected)
	default:
		lg.count(&lg.errors)
	}
}

func (lg *loadgen) count(dst *uint64) {
	lg.mu.Lock()
	*dst++
	lg.mu.Unlock()
}

// runPoisson membangkitkan kedatangan Poisson dengan λ sesuai surge saat ini.
func (lg *loadgen) runPoisson(ctx context.Context) {
	for {
		surge := lg.surgeAt(time.Now())
		lambda := lg.baseRate * surge / 60.0 // order/detik
		if lambda <= 0 {
			lambda = 0.001
		}
		dt := time.Duration(-math.Log(1-lg.rngFloat()) / lambda * float64(time.Second))
		if dt < time.Millisecond {
			dt = time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(dt):
		}
		select {
		case lg.sema <- struct{}{}:
		case <-ctx.Done():
			return
		}
		lg.mu.Lock()
		lg.inflight++
		lg.mu.Unlock()
		go func() {
			defer func() {
				<-lg.sema
				lg.mu.Lock()
				lg.inflight--
				lg.mu.Unlock()
			}()
			cctx, cancel := context.WithTimeout(ctx, lg.timeout)
			defer cancel()
			lg.sendOne(cctx)
		}()
	}
}

func (lg *loadgen) rngFloat() float64 {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	return lg.rng.Float64()
}

func haversineM(latA, lonA, latB, lonB float64) float64 {
	const r = 6371000.0
	dLat := (latB - latA) * math.Pi / 180
	dLon := (lonB - lonA) * math.Pi / 180
	la := latA * math.Pi / 180
	lb := latB * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

func main() {
	port := envStr("PORT", "4204")
	lg := &loadgen{
		orderURL:      envStr("ORDER_URL", "http://127.0.0.1:4202"),
		timeout:       time.Duration(envFloat("TIMEOUT_MS", 10000)) * time.Millisecond,
		seed:          int64(envInt("SEED", 7)),
		idemPrefix:    envStr("IDEM_PREFIX", "lg"),
		baseRate:      envFloat("RATE_PER_MIN", 300),
		spikeFactor:   envFloat("SPIKE_FACTOR", 10),
		spikeEverySec: envInt("SPIKE_EVERY_SEC", 90),
		spikeDurSec:   envInt("SPIKE_DURATION_SEC", 60),
		manualSurge:   envFloat("SURGE", 0),
		startedAt:     time.Now(),
	}
	if lg.manualSurge > 0 {
		// SURGE env tanpa spike: jadwalkan mati (manual jadi satu-satunya sumber)
		lg.spikeEverySec = 0
	}
	runSec := envInt("RUN_DURATION_SEC", 0)
	conc := envInt("MAX_CONCURRENCY", 64)
	lg.sema = make(chan struct{}, conc)
	lg.rng = rand.New(rand.NewSource(lg.seed))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go lg.runPoisson(ctx)

	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true, "service": "loadgen",
			"uptime_sec": int64(time.Since(start).Seconds()),
			"order_url":  lg.orderURL,
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, lg.snapshot())
	})
	mux.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var req struct {
			Surge float64 `json:"surge"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || math.IsNaN(req.Surge) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
			return
		}
		if req.Surge < 1 || req.Surge > 10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "surge_out_of_range"})
			return
		}
		lg.mu.Lock()
		lg.manualSurge = req.Surge
		lg.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]float64{"surge": req.Surge})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		lg.mu.Lock()
		s := lg.surgeNow
		lg.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]float64{"surge": s})
	})

	addr := "0.0.0.0:" + port
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("loadgen listening on %s → %s (rate %.0f/min, spike ×%.0f every %ds for %ds, seed %d)",
		addr, lg.orderURL, lg.baseRate, lg.spikeFactor, lg.spikeEverySec, lg.spikeDurSec, lg.seed)

	if runSec > 0 {
		go func() {
			<-time.After(time.Duration(runSec) * time.Second)
			final := lg.snapshot()
			b, _ := json.Marshal(final)
			log.Printf("LOADGEN_FINAL %s", b) // satu baris untuk direkap laporan
			cancel()
			_ = srv.Shutdown(context.Background()) // keluar bersih setelah run selesai
		}()
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Printf("loadgen selesai setelah %.1f s", time.Since(start).Seconds())
}

func envStr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
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
