// Proxy /api/replay/* (Fase 5) — meneruskan dump sesi rekaman dari
// rider-sim dengan Content-Encoding apa adanya. Dump disimpan sebagai
// anggota gzip per frame (~4,5× lebih kecil dari JSON mentah); Accept-
// Encoding diset eksplisit ke upstream supaya transport Go TIDAK
// mendekompresi otomatis dan blob bisa dialirkan tanpa re-marshal.
package main

import (
	"io"
	"net/http"
)

func replayProxy(simURL string, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := simURL + r.URL.Path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
		if err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		ae := r.Header.Get("Accept-Encoding")
		if ae == "" {
			ae = "gzip"
		}
		req.Header.Set("Accept-Encoding", ae)
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, `{"error":"sim unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		if ce := resp.Header.Get("Content-Encoding"); ce != "" {
			w.Header().Set("Content-Encoding", ce)
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}
