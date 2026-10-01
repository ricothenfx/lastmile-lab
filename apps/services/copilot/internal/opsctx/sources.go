package opsctx

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
}

// buildSources mengekstrak sitasi bernomor dari payload — angka nyata
// dari sistem, bukan karangan. Nilai "null"/kosong tidak dijadikan sitasi
// (UI menampilkan "—" untuk metrik yang tak terukur — bukan data).
func buildSources(docs []doc) []Source {
	sources := []Source{}
	for _, d := range docs {
		switch d.name {
		case "/api/kpi":
			sources = append(sources, kpiSources(d.raw)...)
		case "/api/chaos/incidents":
			sources = append(sources, incidentSources(d.raw)...)
		default:
			// pipeline_metrics: sertakan ringkas seluruh JSON sebagai satu sitasi.
			sources = append(sources, Source{
				ID:    "pipeline.raw",
				Label: "Metrik pipeline (JSON)",
				Value: compact(string(d.raw)),
			})
		}
	}
	return sources
}

// kpiExtract subset /api/kpi yang relevan untuk Q&A — struktur mengikuti
// api-gateway kpi.go (sim, orders_per_min, incidents_open, error_budget).
func kpiExtract(raw json.RawMessage) (out map[string]interface{}, ok bool) {
	var doc struct {
		Source        string   `json:"source"`
		Sim           *simKPI  `json:"sim"`
		OrdersPerMin  *float64 `json:"orders_per_min"`
		DelivPerMin   *float64 `json:"delivered_per_min"`
		IncidentsOpen *int     `json:"incidents_open"`
		ErrorBudget   *struct {
			Availability float64 `json:"availability_pct"`
			OK           bool    `json:"ok"`
		} `json:"error_budget"`
		Queue struct {
			WaitingOrders int `json:"waiting_orders"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Sim == nil {
		return nil, false
	}
	m := map[string]interface{}{
		"source":        doc.Source,
		"sim":           doc.Sim,
		"queue_waiting": doc.Queue.WaitingOrders,
	}
	if doc.OrdersPerMin != nil {
		m["orders_per_min"] = *doc.OrdersPerMin
	}
	if doc.DelivPerMin != nil {
		m["delivered_per_min"] = *doc.DelivPerMin
	}
	if doc.IncidentsOpen != nil {
		m["incidents_open"] = *doc.IncidentsOpen
	}
	if doc.ErrorBudget != nil {
		m["availability_pct"] = doc.ErrorBudget.Availability
	}
	return m, true
}

type simKPI struct {
	Reachable      bool    `json:"reachable"`
	Riders         int     `json:"riders"`
	IdleRiders     int     `json:"idle_riders"`
	OrdersActive   int     `json:"orders_active"`
	OrdersWaiting  int     `json:"orders_waiting"`
	Created        int     `json:"orders_created"`
	Delivered      int     `json:"orders_delivered"`
	Expired        int     `json:"orders_expired"`
	Surge          float64 `json:"surge"`
	Weather        float64 `json:"weather"`
	Strategy       string  `json:"strategy"`
	UtilizationPct float64 `json:"utilization_pct"`
	CostPerOrderKm float64 `json:"cost_per_order_km"`
	DeliveryP50Ms  float64 `json:"delivery_p50_ms"`
	DeliveryP95Ms  float64 `json:"delivery_p95_ms"`
	P99DispatchMs  float64 `json:"p99_dispatch_ms"`
}

// kpiSources: satu sitasi per KPI penting + sitasi ringkas keseluruhan.
func kpiSources(raw json.RawMessage) []Source {
	m, ok := kpiExtract(raw)
	if !ok {
		return nil
	}
	sim := m["sim"].(*simKPI)
	out := []Source{}
	add := func(id, label, value string) {
		out = append(out, Source{ID: id, Label: label, Value: value})
	}
	add("kpi.sim.strategy", "Strategi dispatch aktif", sim.Strategy)
	add("kpi.sim.surge", "Faktor surge aktif", fmt.Sprintf("%.1f", sim.Surge))
	add("kpi.sim.weather", "Faktor cuaca aktif", fmt.Sprintf("%.1f", sim.Weather))
	add("kpi.sim.orders_waiting", "Order menunggu di antrean", fmt.Sprintf("%d", sim.OrdersWaiting))
	add("kpi.sim.orders_expired", "Order expired (lifetime)", fmt.Sprintf("%d", sim.Expired))
	add("kpi.sim.orders_delivered", "Order terkirim (lifetime)", fmt.Sprintf("%d", sim.Delivered))
	add("kpi.sim.delivery_p50_ms", "Delivery p50 (ring live)", fmt.Sprintf("%.0f ms", sim.DeliveryP50Ms))
	add("kpi.sim.delivery_p95_ms", "Delivery p95 (ring live)", fmt.Sprintf("%.0f ms", sim.DeliveryP95Ms))
	add("kpi.sim.utilization_pct", "Utilisasi armada", fmt.Sprintf("%.1f%%", sim.UtilizationPct))
	add("kpi.sim.cost_per_order_km", "Biaya per order", fmt.Sprintf("%.2f km", sim.CostPerOrderKm))
	add("kpi.sim.p99_dispatch_ms", "p99 keputusan dispatch", fmt.Sprintf("%.1f ms", sim.P99DispatchMs))
	add("kpi.sim.idle_riders", "Rider idle / total", fmt.Sprintf("%d/%d", sim.IdleRiders, sim.Riders))
	if v, ok := m["orders_per_min"]; ok {
		add("kpi.orders_per_min", "Laju order (rolling 60 s)", fmt.Sprintf("%.1f/menit", v.(float64)))
	}
	if v, ok := m["delivered_per_min"]; ok {
		add("kpi.delivered_per_min", "Laju terkirim (rolling 60 s)", fmt.Sprintf("%.1f/menit", v.(float64)))
	}
	if v, ok := m["incidents_open"]; ok {
		add("kpi.incidents_open", "Incident terbuka", fmt.Sprintf("%d", v.(int)))
	}
	if v, ok := m["availability_pct"]; ok {
		add("kpi.availability_pct", "Availability (window chaos)", fmt.Sprintf("%.2f%%", v.(float64)))
	}
	add("kpi.raw", "Payload /api/kpi lengkap", compact(string(raw)))
	return out
}

// incidentSources: satu sitasi per incident (maks 10 terbaru).
func incidentSources(raw json.RawMessage) []Source {
	var doc struct {
		Incidents []struct {
			ID       string `json:"id"`
			Target   string `json:"target"`
			Kind     string `json:"kind"`
			TStart   int64  `json:"t_start"`
			TDetect  int64  `json:"t_detect"`
			TRecover int64  `json:"t_recover"`
		} `json:"incidents"`
		Summary struct {
			MTTDAvgMs       float64 `json:"mttd_avg_ms"`
			MTTRAvgMs       float64 `json:"mttr_avg_ms"`
			AvailabilityPct float64 `json:"availability_pct"`
			IncidentsTotal  int     `json:"incidents_total"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	out := []Source{{
		ID:    "incidents.summary",
		Label: "Ringkasan incident chaos",
		Value: fmt.Sprintf("total=%d mttd_avg=%.0fms mttr_avg=%.0fms availability=%.2f%%",
			doc.Summary.IncidentsTotal, doc.Summary.MTTDAvgMs, doc.Summary.MTTRAvgMs, doc.Summary.AvailabilityPct),
	}}
	n := len(doc.Incidents)
	if n > 10 {
		n = 10
	}
	for i := len(doc.Incidents) - n; i < len(doc.Incidents); i++ {
		inc := doc.Incidents[i]
		status := "RECOVERED"
		if inc.TRecover <= 0 {
			status = "OPEN"
		}
		out = append(out, Source{
			ID:    "incident." + inc.ID,
			Label: fmt.Sprintf("Incident %s di %s", inc.Kind, inc.Target),
			Value: fmt.Sprintf("status=%s t_start=%d mttd=%dms mttr_ms=%s",
				status, inc.TStart, inc.TDetect-inc.TStart,
				fmtInt(inc.TRecover-inc.TDetect)),
		})
	}
	return out
}

func fmtInt(v int64) string {
	if v <= 0 {
		return "?"
	}
	return fmt.Sprintf("%d", v)
}

// compact memangkas whitespace JSON agar prompt hemat token.
func compact(s string) string {
	var buf bytes.Buffer
	_ = json.Compact(&buf, []byte(s))
	if buf.Len() == 0 {
		return s
	}
	if buf.Len() > 4096 {
		return buf.String()[:4096] + "…"
	}
	return buf.String()
}
