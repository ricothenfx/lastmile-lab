// api-gateway — REST publik (loopback/Caddy) untuk snapshot, kontrol & metrik.
//
//	GET  /healthz             liveness + status rider-sim (probe berkala)
//	GET  /api/snapshot        snapshot penuh + keputusan dispatch (explainability)
//	GET  /api/graph           graph JSON mentah
//	POST /api/control/surge   → sim-control (Surge Console, Fase 2)
//	POST /api/control/weather → sim-control
//	GET  /api/control/state   nilai surge/weather aktif
//	GET  /api/metrics         agregasi counters pipeline (JSON, Fase 2)
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	port := envStr("PORT", "3010")
	simURL := envStr("RIDER_SIM_URL", "http://127.0.0.1:4201")
	controlURL := envStr("SIM_CONTROL_URL", "http://127.0.0.1:3013")
	ingestionURL := envStr("ORDER_INGESTION_URL", "http://127.0.0.1:4202")
	consumerURL := envStr("DISPATCH_CONSUMER_URL", "http://127.0.0.1:4203")
	loadgenURL := envStr("LOADGEN_URL", "") // hanya saat loadtest — kosong = skip
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

	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"service":    "api-gateway",
			"uptime_sec": int64(time.Since(start).Seconds()),
			"sim_ok":     probe(),
		})
	})
	mux.HandleFunc("/api/snapshot", proxy("/api/snapshot"))
	mux.HandleFunc("/api/graph", proxy("/api/graph"))
	mux.HandleFunc("/api/metrics", metrics)
	mux.HandleFunc("/api/control/surge", proxyControl("/control/surge"))
	mux.HandleFunc("/api/control/weather", proxyControl("/control/weather"))
	mux.HandleFunc("/api/control/state", proxyControl("/state"))

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
