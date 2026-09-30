// api-gateway — REST publik (loopback/Caddy) untuk snapshot, kontrol & metrik.
//
//	GET  /healthz             liveness + status rider-sim (probe berkala)
//	GET  /api/snapshot        snapshot penuh + keputusan dispatch (explainability)
//	GET  /api/graph           graph JSON mentah
//	POST /api/control/surge   → sim-control (Surge Console, Fase 2)
//	POST /api/control/weather → sim-control
//	GET  /api/control/state   nilai surge/weather aktif
//	GET  /api/metrics         agregasi counters pipeline (JSON, Fase 2)
//	*    /api/lab/*           → strategy-lab (Strategy Lab, Fase 3)
//	GET  /api/kpi             KPI + SLO + grid + budget (Fase 4, kpi.go)
//	*    /api/chaos/*         → chaos injector (Fase 4)
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	port := envStr("PORT", "3010")
	simURL := envStr("RIDER_SIM_URL", "http://127.0.0.1:4201")
	controlURL := envStr("SIM_CONTROL_URL", "http://127.0.0.1:3013")
	ingestionURL := envStr("ORDER_INGESTION_URL", "http://127.0.0.1:4202")
	consumerURL := envStr("DISPATCH_CONSUMER_URL", "http://127.0.0.1:4203")
	wsURL := envStr("WS_GATEWAY_URL", "")   // kosong = grid ws-gateway standby
	loadgenURL := envStr("LOADGEN_URL", "") // hanya saat loadtest — kosong = skip
	labURL := envStr("LAB_URL", "")         // kosong = /api/lab/* → 503 (lab opsional)
	chaosURL := envStr("CHAOS_URL", "")     // kosong = /api/chaos/* → 503 (chaos opsional)
	client := &http.Client{Timeout: 4 * time.Second}

	var simMu sync.Mutex
	simUp, simCheckedAt := false, time.Time{}
	probe := func() bool {
		simMu.Lock()
		defer simMu.Unlock()
		if time.Since(simCheckedAt) < 3*time.Second {
			return simUp
		}
		resp, err := client.Get(simURL + "/healthz")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			simUp = resp.StatusCode == http.StatusOK
		} else {
			simUp = false
		}
		simCheckedAt = time.Now()
		return simUp
	}

	proxy := func(path string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			resp, err := client.Get(simURL + path)
			if err != nil {
				http.Error(w, `{"error":"sim unreachable"}`, http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}
	}

	// proxy kontrol: meneruskan method+body apa adanya ke sim-control.
	proxyControl := func(path string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			req, err := http.NewRequestWithContext(r.Context(), r.Method, controlURL+path, r.Body)
			if err != nil {
				http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				http.Error(w, `{"error":"sim-control unreachable"}`, http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}
	}

	// /api/metrics: agregasi counters semua service pipeline (best-effort).
	fetchJSON := func(url string, out interface{}) bool {
		if url == "" {
			return false
		}
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		return json.NewDecoder(resp.Body).Decode(out) == nil
	}
	metrics := func(w http.ResponseWriter, r *http.Request) {
		var ing, con, lg map[string]interface{}
		ingOK := fetchJSON(ingestionURL+"/metrics", &ing)
		conOK := fetchJSON(consumerURL+"/metrics", &con)
		lgOK := loadgenURL != "" && fetchJSON(loadgenURL+"/metrics", &lg)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"order_ingestion":      ing,
			"order_ingestion_ok":   ingOK,
			"dispatch_consumer":    con,
			"dispatch_consumer_ok": conOK,
			"loadgen":              lg,
			"loadgen_ok":           lgOK,
		})
	}

	// proxy lab: meneruskan method+body apa adanya ke strategy-lab
	// (POST /api/lab/run → /run; GET /api/lab/results[/id] → /results[...]).
	proxyLab := func(w http.ResponseWriter, r *http.Request) {
		if labURL == "" {
			http.Error(w, `{"error":"lab_disabled"}`, http.StatusServiceUnavailable)
			return
		}
		target := labURL + strings.TrimPrefix(r.URL.Path, "/api/lab")
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// duel bisa jalan menit-menit dinding — klien polling, bukan menunggu.
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"strategy-lab unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}

	// proxy chaos: sama polanya dengan lab (Fase 4).
	proxyChaos := func(w http.ResponseWriter, r *http.Request) {
		if chaosURL == "" {
			http.Error(w, `{"error":"chaos_disabled"}`, http.StatusServiceUnavailable)
			return
		}
		target := chaosURL + strings.TrimPrefix(r.URL.Path, "/api/chaos")
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		if err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"chaos unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}

	// /api/kpi — KPI agregat + SLO + grid + error budget (Fase 4, kpi.go).
	// Semua upstream diambil paralel; payload di-cache 400 ms agar polling
	// UI 2 Hz tidak menghajar service internal.
	var agg kpiAggregator
	gridCache := &sync.Map{}
	kpiHandler := func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		if p, ok := agg.cached(now, 400*time.Millisecond); ok {
			writeJSON(w, http.StatusOK, p)
			return
		}
		var (
			wg           sync.WaitGroup
			eng          simEngineKPI
			simOK        bool
			ing, con     map[string]interface{}
			ingOK, conOK bool
			chaos        chaosIncidents
			chaosOK      bool
		)
		fetch := func(url string, out interface{}, ok *bool) {
			defer wg.Done()
			*ok = fetchJSON(url, out)
		}
		wg.Add(4)
		go fetch(simURL+"/internal/metrics", &struct {
			Engine *simEngineKPI `json:"engine"`
		}{&eng}, &simOK)
		go fetch(ingestionURL+"/metrics", &ing, &ingOK)
		go fetch(consumerURL+"/metrics", &con, &conOK)
		if chaosURL != "" {
			go fetch(chaosURL+"/incidents", &chaos, &chaosOK)
		} else {
			wg.Done()
		}

		var grid []gridNode
		wg.Add(1)
		go func() {
			defer wg.Done()
			grid = probeGrid([]gridTarget{
				{name: "rider-sim", group: "core", url: simURL},
				{name: "ws-gateway", group: "core", url: wsURL},
				{name: "api-gateway", group: "core", url: ""}, // self — selalu up dari sudut pandang handler
				{name: "strategy-lab", group: "lab", url: labURL},
				{name: "order-ingestion", group: "pipeline", url: ingestionURL},
				{name: "dispatch-consumer", group: "pipeline", url: consumerURL},
				{name: "sim-control", group: "pipeline", url: controlURL},
				{name: "chaos", group: "chaos", url: chaosURL},
			}, client, gridCache, 2*time.Second)
		}()
		wg.Wait()
		for i := range grid { // self ditandai up (endpoint ini buktinya hidup)
			if grid[i].Name == "api-gateway" {
				grid[i].Status = "up"
			}
		}

		resp := assembleKPI(kpiInput{
			now: now, eng: eng, simReachable: simOK,
			ing: ing, ingOK: ingOK, con: con, conOK: conOK,
			chaos: &chaos, chaosReachable: chaosOK,
			grid: grid,
		})
		// laju per menit perlu riwayat sampel — catat lalu hitung.
		agg.store(resp, now)
		resp.OrdersPerMin, resp.DelivPerMin = agg.rates(now)
		writeJSON(w, http.StatusOK, resp)
	}

	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"service":    "api-gateway",
			"uptime_sec": int64(time.Since(start).Seconds()),
			"sim_ok":     probe(),
			"lab_ok":     labURL != "",
		})
	})
	mux.HandleFunc("/api/snapshot", proxy("/api/snapshot"))
	mux.HandleFunc("/api/graph", proxy("/api/graph"))
	mux.HandleFunc("/api/metrics", metrics)
	mux.HandleFunc("/api/kpi", kpiHandler)
	mux.HandleFunc("/api/control/surge", proxyControl("/control/surge"))
	mux.HandleFunc("/api/control/weather", proxyControl("/control/weather"))
	mux.HandleFunc("/api/control/state", proxyControl("/state"))
	mux.HandleFunc("/api/lab/", proxyLab)
	mux.HandleFunc("/api/lab", proxyLab)
	mux.HandleFunc("/api/chaos/", proxyChaos)
	mux.HandleFunc("/api/chaos", proxyChaos)

	handler := cors(mux)
	addr := "0.0.0.0:" + port
	log.Printf("api-gateway listening on %s (sim %s, control %s)", addr, simURL, controlURL)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}
