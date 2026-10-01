package opsctx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stubGateway(t *testing.T, missing string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/kpi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"source":"live","sim":{"strategy":"fifo","surge":4,"weather":1,"orders_waiting":23,"orders_expired":7,"orders_delivered":300,"delivery_p50_ms":180000,"delivery_p95_ms":420000,"utilization_pct":81,"cost_per_order_km":1.2,"p99_dispatch_ms":11,"idle_riders":19,"riders":100},"orders_per_min":34,"incidents_open":1,"error_budget":{"availability_pct":99.8,"ok":false}}`))
	})
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"order_ingestion":{"counters":{"published":42}},"order_ingestion_ok":true}`))
	})
	mux.HandleFunc("/api/chaos/incidents", func(w http.ResponseWriter, r *http.Request) {
		if missing == "/api/chaos/incidents" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"incidents":[{"id":"abc","target":"lastmile-ws-gateway","kind":"chaos-kill","t_start":1000,"t_detect":1500,"t_recover":3500}],"summary":{"incidents_total":1,"mttd_avg_ms":500,"mttr_avg_ms":2000,"availability_pct":99.9}}`))
	})
	if missing == "/api/kpi" {
		// kpi distub gagal — timpa handler default.
		mux.Handle("/api/kpi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	}
	return httptest.NewServer(mux)
}

func TestFetchMergesAndBuildsSources(t *testing.T) {
	c := New(stubGateway(t, "").URL)
	cx := c.Fetch(context.Background())
	if len(cx.Missing) != 0 {
		t.Fatalf("tidak boleh ada missing: %v", cx.Missing)
	}
	for _, key := range []string{`"kpi":`, `"pipeline_metrics":`, `"incidents":`} {
		if !strings.Contains(cx.JSON, key) {
			t.Fatalf("JSON merged harus memuat %s", key)
		}
	}
	want := map[string]bool{
		"kpi.sim.surge": false, "kpi.sim.delivery_p95_ms": false,
		"kpi.orders_per_min": false, "kpi.incidents_open": false,
		"incidents.summary": false, "incident.abc": false, "pipeline.raw": false,
	}
	for _, s := range cx.Sources {
		if _, ok := want[s.ID]; ok {
			want[s.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("situs %s harus ada di sources", id)
		}
	}
}

func TestFetchReportsMissingHonestly(t *testing.T) {
	c := New(stubGateway(t, "/api/chaos/incidents").URL)
	cx := c.Fetch(context.Background())
	if len(cx.Missing) != 1 || cx.Missing[0] != "/api/chaos/incidents" {
		t.Fatalf("missing harus jujur dilaporkan: %v", cx.Missing)
	}
	if strings.Contains(cx.JSON, `"incidents":{`) {
		t.Fatal("payload gagal tidak boleh masuk JSON merged")
	}
	for _, s := range cx.Sources {
		if strings.HasPrefix(s.ID, "incident.") || s.ID == "incidents.summary" {
			t.Fatalf("sitasi incident tidak boleh muncul tanpa datanya: %s", s.ID)
		}
	}
}

func TestUnavailableGatewayLeavesEmptyContext(t *testing.T) {
	c := New("http://127.0.0.1:1") // port tertutup
	cx := c.Fetch(context.Background())
	if len(cx.Missing) != 3 {
		t.Fatalf("semua endpoint harus missing: %v", cx.Missing)
	}
	if len(cx.Sources) != 0 {
		t.Fatal("sources harus kosong — tidak ada data karangan")
	}
}
