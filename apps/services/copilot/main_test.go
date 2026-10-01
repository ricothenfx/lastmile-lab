package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/llm"
	"github.com/ricothenfx/lastmile-lab/apps/services/copilot/internal/opsctx"
	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
)

func tinyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	meta := graph.Meta{City: "CopilotTest", Source: "test", BBox: []float64{0, 0, 1, 1}}
	nodes := [][2]float64{
		{0.000, 0.000}, {0.000, 0.010}, {0.010, 0.000}, {0.010, 0.010},
	}
	edges := [][3]float64{{0, 1, 800}, {0, 2, 800}, {1, 3, 800}, {2, 3, 800}, {1, 2, 1100}}
	g, err := graph.New(meta, nodes, edges, nil, []int{0, 3})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// stubGateway api-gateway mini untuk context copilot.
func stubGateway(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/kpi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"source":"live","sim":{"strategy":"fifo","surge":4,"weather":1,"orders_waiting":23,"orders_expired":7,"orders_delivered":300,"delivery_p50_ms":180000,"delivery_p95_ms":420000,"utilization_pct":81,"cost_per_order_km":1.2,"p99_dispatch_ms":11,"idle_riders":19,"riders":100},"orders_per_min":34}`))
	})
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"order_ingestion_ok":false}`))
	})
	mux.HandleFunc("/api/chaos/incidents", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"incidents":[],"summary":{"incidents_total":0,"mttd_avg_ms":0,"mttr_avg_ms":0}}`))
	})
	return httptest.NewServer(mux)
}

// stubLLM provider palsu — memeriksa prompt lalu membalas skrip.
type stubLLM struct {
	gotUser  string
	response string
	err      error
}

func (s *stubLLM) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	s.gotUser = req.User
	if s.err != nil {
		return nil, s.err
	}
	return &llm.Response{Text: s.response, Model: "stub"}, nil
}

func newTestServer(t *testing.T, prov llm.Provider) *httptest.Server {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "test-key") // handler mengecek Enabled()
	s := &server{
		prov: prov, gw: opsctx.New(stubGateway(t).URL), g: tinyGraph(t),
		limAdvise: newLimiter(30), limAsk: newLimiter(30),
	}
	return httptest.NewServer(s.mux())
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	var buf strings.Builder
	var m map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&m)
	_ = json.NewEncoder(&buf).Encode(m)
	return buf.String()
}

func TestCapabilitiesMirrorsKey(t *testing.T) {
	ts := newTestServer(t, &stubLLM{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body(t, resp), `"enabled":true`) {
		t.Fatal("dengan key capabilities harus enabled")
	}
}

func TestDisabledWithoutKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	s := &server{prov: llm.Noop{}, gw: opsctx.New("http://127.0.0.1:1"), limAdvise: newLimiter(5), limAsk: newLimiter(5)}
	ts := httptest.NewServer(s.mux())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/capabilities")
	if !strings.Contains(body(t, resp), `"enabled":false`) {
		t.Fatal("tanpa key capabilities harus disabled")
	}
	resp2 := post(t, ts.URL+"/advise", `{}`)
	if resp2.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body(t, resp2), "llm_disabled") {
		t.Fatal("advise tanpa key harus 503 llm_disabled")
	}
	resp3 := post(t, ts.URL+"/ask", `{"question":"p95?"}`)
	if resp3.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body(t, resp3), "llm_disabled") {
		t.Fatal("ask tanpa key harus 503 llm_disabled")
	}
}

func TestAdviseFlowEndToEnd(t *testing.T) {
	stub := &stubLLM{response: `{"plans":[{"name":"optimal","rationale":"p95 420000 ms (kpi.sim.delivery_p95_ms)","actions":[{"kind":"strategy","params":{"name":"optimal"}}]}]}`}
	ts := newTestServer(t, stub)
	defer ts.Close()
	resp := post(t, ts.URL+"/advise", `{"seed":7}`)
	out := body(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("advise harus 200, dapat %d: %s", resp.StatusCode, out)
	}
	for _, want := range []string{`"baseline"`, `"predicted"`, `"seed":7`, `"plans"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("respons advise harus memuat %s: %s", want, out)
		}
	}
	if !strings.Contains(stub.gotUser, `"surge":4`) {
		t.Fatal("prompt harus memuat konteks live (surge 4)")
	}
}

func TestAdviseInvalidPlansRejected(t *testing.T) {
	stub := &stubLLM{response: `{"plans":[]}`}
	ts := newTestServer(t, stub)
	defer ts.Close()
	resp := post(t, ts.URL+"/advise", `{}`)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body(t, resp), "plan_invalid") {
		t.Fatal("llm tanpa plan valid harus 422 plan_invalid")
	}
}

func TestAskFlowAndCitationEnforcement(t *testing.T) {
	ok := &stubLLM{response: `{"text":"p95 420000 ms","sources":["kpi.sim.delivery_p95_ms"]}`}
	ts := newTestServer(t, ok)
	defer ts.Close()
	resp := post(t, ts.URL+"/ask", `{"question":"Kenapa p95 naik?"}`)
	out := body(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ask harus 200, dapat %d: %s", resp.StatusCode, out)
	}
	if !strings.Contains(out, `"kpi.sim.delivery_p95_ms"`) || !strings.Contains(out, "420000") {
		t.Fatal("respons ask harus memuat teks + sitasi ter-resolve")
	}

	bad := &stubLLM{response: `{"text":"karangan","sources":[]}`}
	ts2 := newTestServer(t, bad)
	defer ts2.Close()
	resp2 := post(t, ts2.URL+"/ask", `{"question":"Kenapa p95 naik?"}`)
	if resp2.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body(t, resp2), "answer_rejected") {
		t.Fatal("jawaban tanpa sitasi harus 422 answer_rejected")
	}
}

func TestAskValidatesQuestion(t *testing.T) {
	ts := newTestServer(t, &stubLLM{})
	defer ts.Close()
	if r := post(t, ts.URL+"/ask", `{"question":"x"}`); r.StatusCode != http.StatusBadRequest {
		t.Fatal("pertanyaan terlalu pendek harus 400")
	}
	if r := post(t, ts.URL+"/ask", `not json`); r.StatusCode != http.StatusBadRequest {
		t.Fatal("body rusak harus 400")
	}
}

func TestRateLimiter(t *testing.T) {
	l := newLimiter(1)
	if !l.allow() {
		t.Fatal("panggilan pertama harus diizinkan")
	}
	if l.allow() {
		t.Fatal("token habis — panggilan kedua harus ditolak")
	}
}
