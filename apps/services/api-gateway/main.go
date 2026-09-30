// api-gateway — REST minimal untuk fallback & debugging (Fase 1).
//
//	GET /healthz        liveness + status rider-sim (probe berkala)
//	GET /api/snapshot   snapshot penuh + keputusan dispatch (explainability)
//	GET /api/graph      graph JSON mentah
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
	client := &http.Client{Timeout: 3 * time.Second}

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

	handler := cors(mux)
	addr := "0.0.0.0:" + port
	log.Printf("api-gateway listening on %s (sim %s)", addr, simURL)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
