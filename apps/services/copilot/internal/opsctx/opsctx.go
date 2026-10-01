// Package opsctx — context operasi internal untuk copilot (Fase 7).
//
// Semua data diambil dari endpoint yang SUDAH ADA di api-gateway
// (/api/kpi, /api/metrics, /api/chaos/incidents) — TANPA akses DB langsung.
// Payload JSON mentah dipakai sebagai prompt; versi terstruktur dijadikan
// daftar sitasi (sources) untuk jalur ask. Fetch best-effort paralel dengan
// timeout ketat: bagian yang gagal hilang dari context (prompt menyebutnya),
// tidak pernah dipalsukan.
package opsctx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Source satu sitasi internal — id stabil agar jawaban bisa divalidasi.
type Source struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// Context hasil fetch — JSON mentah + daftar sitasi.
type Context struct {
	JSON      string   // seluruh payload (dipakai prompt advise)
	Sources   []Source // sitasi bernomor (dipakai ask)
	Missing   []string // endpoint yang gagal diambil (jujur di prompt)
	FetchedAt time.Time
}

// Client fetch context dari api-gateway.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 2 * time.Second}}
}

type fetchOut struct {
	name string
	raw  json.RawMessage
	err  error
}

// doc satu payload upstream (kpi / metrics / incidents).
type doc struct {
	name string
	raw  json.RawMessage
}

// Fetch mengambil kpi + metrics + incidents paralel (best-effort, total ≤ 3 s).
func (c *Client) Fetch(ctx context.Context) *Context {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	targets := []string{"/api/kpi", "/api/metrics", "/api/chaos/incidents"}
	out := make(chan fetchOut, len(targets))
	for _, t := range targets {
		go func(t string) {
			raw, err := c.get(ctx, t)
			out <- fetchOut{name: t, raw: raw, err: err}
		}(t)
	}
	cx := &Context{FetchedAt: time.Now().UTC(), Sources: []Source{}}
	var docs []doc
	for range targets {
		r := <-out
		if r.err != nil {
			cx.Missing = append(cx.Missing, r.name)
			continue
		}
		docs = append(docs, doc{r.name, r.raw})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].name < docs[j].name })

	merged := map[string]json.RawMessage{}
	for _, d := range docs {
		merged[docKey(d.name)] = d.raw
	}
	if len(cx.Missing) > 0 {
		sort.Strings(cx.Missing)
		merged["_missing"] = mustJSON(cx.Missing)
	}
	cx.JSON = string(mustJSON(merged))
	cx.Sources = buildSources(docs)
	return cx
}

func docKey(endpoint string) string {
	switch endpoint {
	case "/api/kpi":
		return "kpi"
	case "/api/metrics":
		return "pipeline_metrics"
	default:
		return "incidents"
	}
}

func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", path, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}
