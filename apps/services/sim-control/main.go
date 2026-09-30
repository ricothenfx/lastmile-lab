// sim-control — API kontrol simulasi (Fase 2, loopback :3013).
//
//	POST /control/surge   {"factor":8}    → rider-sim (+ loadgen bila hidup)
//	POST /control/weather {"factor":0.6}  → rider-sim (hujan melambatkan rider)
//	GET  /state           nilai surge/weather aktif + status target
//	GET  /healthz
//
// Dipakai Surge Console frontend via proxy api-gateway (/api/control/*).
package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type targets struct {
	riderSimURL string
	loadgenURL  string // boleh kosong — loadgen hanya jalan saat loadtest
	hc          *http.Client
}

type ctlResp struct {
	Surge   float64 `json:"surge"`
	Weather float64 `json:"weather"`
}

func postJSON(ctx context.Context, url string, body interface{}) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader(string(b)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// fanOutControl mengirim kontrol ke rider-sim (wajib) dan loadgen (best-effort,
// hanya surge — cuaca tidak mengubah laju demand).
func (t *targets) fanOut(ctx context.Context, payload map[string]float64, includeLoadgen bool) (map[string]string, error) {
	out := map[string]string{}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	code, err := postJSON(ctx, t.riderSimURL+"/internal/control", payload)
	if err != nil || code != http.StatusOK {
		out["rider-sim"] = "down"
		return out, err2("rider-sim unreachable")
	}
	out["rider-sim"] = "ok"
	if t.loadgenURL != "" {
		if !includeLoadgen {
			out["loadgen"] = "skipped"
			return out, nil
		}
		// async: rider-sim sudah menerapkan kontrol — respons jangan menunggu
		// loadgen (bila down/up) supaya feedback UI tetap < 1 detik.
		out["loadgen"] = "async"
		go func(url string) {
			lctx, lcancel := context.WithTimeout(context.Background(), time.Second)
			defer lcancel()
			code, err := postJSON(lctx, url+"/control", payload)
			switch {
			case err != nil:
				log.Printf("loadgen control: unreachable")
			case code != http.StatusOK:
				log.Printf("loadgen control: status %d", code)
			}
		}(t.loadgenURL)
	}
	return out, nil
}

func err2(msg string) error { return &simpleError{msg} }

type simpleError struct{ s string }

func (e *simpleError) Error() string { return e.s }

func getJSON(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &simpleError{"http " + strconv.Itoa(resp.StatusCode)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3013"
	}
	t := &targets{
		riderSimURL: envStr("RIDER_SIM_URL", "http://127.0.0.1:4201"),
		loadgenURL:  strings.TrimSpace(os.Getenv("LOADGEN_URL")),
		hc:          &http.Client{Timeout: 2 * time.Second},
	}

	var mu sync.Mutex // pelindung lastGood untuk /state fallback
	lastGood := ctlResp{Surge: 1, Weather: 1}

	mux := http.NewServeMux()
	start := time.Now()

	handleControl := func(kind string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
				return
			}
			var req struct {
				Factor float64 `json:"factor"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || math.IsNaN(req.Factor) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json"})
				return
			}
			var lo, hi float64
			switch kind {
			case "surge":
				lo, hi = 1, 10
			case "weather":
				lo, hi = 0.2, 2
			}
			if req.Factor < lo || req.Factor > hi {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "factor_out_of_range", "min": strconv.FormatFloat(lo, 'f', -1, 64),
					"max": strconv.FormatFloat(hi, 'f', -1, 64),
				})
				return
			}
			payload := map[string]float64{kind: req.Factor}
			res, err := t.fanOut(r.Context(), payload, kind == "surge")
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]interface{}{
					"error": "rider_sim_unreachable", "targets": res,
				})
				return
			}
			mu.Lock()
			if kind == "surge" {
				lastGood.Surge = req.Factor
			} else {
				lastGood.Weather = req.Factor
			}
			cur := lastGood
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				kind: req.Factor, "targets": res, "state": cur,
			})
		}
	}

	mux.HandleFunc("/control/surge", handleControl("surge"))
	mux.HandleFunc("/control/weather", handleControl("weather"))
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		var cur ctlResp
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		err := getJSON(ctx, t.riderSimURL+"/internal/control", &cur)
		if err != nil {
			mu.Lock()
			cur = lastGood
			mu.Unlock()
		}
		lg := "disabled"
		if t.loadgenURL != "" {
			lctx, lcancel := context.WithTimeout(context.Background(), time.Second)
			var lgCtl ctlResp
			if err := getJSON(lctx, t.loadgenURL+"/state", &lgCtl); err != nil {
				lg = "down"
			} else {
				lg = "ok"
			}
			lcancel()
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"surge": cur.Surge, "weather": cur.Weather,
			"targets": map[string]string{"rider-sim": boolStr(err == nil), "loadgen": lg},
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true, "service": "sim-control",
			"uptime_sec": int64(time.Since(start).Seconds()),
			"targets": map[string]string{
				"rider_sim": t.riderSimURL,
				"loadgen":   orDefault(t.loadgenURL, "disabled"),
			},
		})
	})

	addr := "0.0.0.0:" + port
	log.Printf("sim-control listening on %s (rider-sim %s, loadgen %s)",
		addr, t.riderSimURL, orDefault(t.loadgenURL, "disabled"))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func boolStr(b bool) string {
	if b {
		return "ok"
	}
	return "down"
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func envStr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
