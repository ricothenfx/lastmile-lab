package advise

import (
	"strings"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
)

const validPlansJSON = `{"plans":[
  {"name":"P1 optimal","rationale":"p95 12000 ms di atas SLO (kpi.raw)","actions":[{"kind":"strategy","params":{"name":"optimal"}}]},
  {"name":"P2 surge test","rationale":"uji kapasitas saat idle 60/100","actions":[{"kind":"surge","params":{"factor":"2"}},{"kind":"weather","params":{"factor":"0.6"}}]},
  {"name":"P3 chaos drill","rationale":"latih self-heal","actions":[{"kind":"kill","params":{"target":"lastmile-ws-gateway"}}]}
]}`

func TestParsePlansValid(t *testing.T) {
	plans, err := ParsePlans(validPlansJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("harus 3 plan, dapat %d", len(plans))
	}
	if plans[0].Actions[0].Kind != KindStrategy || plans[0].Actions[0].Params["name"] != "optimal" {
		t.Fatalf("plan 1 salah: %+v", plans[0])
	}
}

func TestParsePlansDropsInvalidSilentlyNotFixed(t *testing.T) {
	raw := `{"plans":[
		{"name":"","rationale":"tanpa nama dibuang","actions":[{"kind":"strategy","params":{"name":"fifo"}}]},
		{"name":"bad action","rationale":"kind tak dikenal dibuang","actions":[{"kind":"restart","params":{"x":"1"}}]},
		{"name":"bad surge","rationale":"factor 99 di luar rentang","actions":[{"kind":"surge","params":{"factor":"99"}}]},
		{"name":"bad weather","rationale":"factor 1.5 di luar rentang","actions":[{"kind":"weather","params":{"factor":"1.5"}}]},
		{"name":"bad strategy","rationale":"strategi tak dikenal","actions":[{"kind":"strategy","params":{"name":"magic"}}]},
		{"name":"OK","rationale":"satu-satunya valid","actions":[{"kind":"strategy","params":{"name":"zone"}}]}
	]}`
	plans, err := ParsePlans(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Name != "OK" {
		t.Fatalf("harus tersisa 1 plan valid 'OK', dapat %+v", plans)
	}
}

func TestParsePlansCapsAtThree(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"plans":[`)
	for i := 0; i < 6; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"name":"p","rationale":"r","actions":[{"kind":"strategy","params":{"name":"fifo"}}]}`)
	}
	sb.WriteString(`]}`)
	plans, err := ParsePlans(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != MaxPlans {
		t.Fatalf("maks %d plan, dapat %d", MaxPlans, len(plans))
	}
}

func TestParsePlansRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"halo dunia", `{"plans":[]}`, `{"plans":[{}]}`, "no brace here"} {
		if _, err := ParsePlans(raw); err == nil {
			t.Fatalf("input %q harus error", raw)
		}
	}
}

func TestParsePlansToleratesCodeFence(t *testing.T) {
	raw := "```json\n" + validPlansJSON + "\n```"
	plans, err := ParsePlans(raw)
	if err != nil || len(plans) != 3 {
		t.Fatalf("code fence harus ditoleransi: %v %d", err, len(plans))
	}
}

func TestPlanErrorExplainsDrops(t *testing.T) {
	if msg := PlanError(Plan{Name: "x", Rationale: "y", Actions: []Action{{Kind: "weird"}}}); msg == "" {
		t.Fatal("aksi tak dikenal harus punya alasan")
	}
	if msg := PlanError(Plan{Name: "x", Rationale: "y"}); msg == "" {
		t.Fatal("plan tanpa aksi harus punya alasan")
	}
}

func TestValidActionStrategyUsesRegistry(t *testing.T) {
	for _, name := range dispatch.Names() {
		if !validAction(Action{Kind: KindStrategy, Params: map[string]string{"name": name}}) {
			t.Fatalf("strategi resmi %q harus valid", name)
		}
	}
}

func TestBuildUserPromptEmbedsContext(t *testing.T) {
	p := BuildUserPrompt(`{"kpi":{"sim":{"p99_dispatch_ms":9.9}}}`, "fifo")
	if !strings.Contains(p, "p99_dispatch_ms") || !strings.Contains(p, `"fifo"`) {
		t.Fatal("prompt harus memuat konteks JSON + strategi aktif")
	}
	if !strings.Contains(SystemPrompt(), `"surge"|"weather"|"strategy"|"kill"`) {
		t.Fatal("system prompt harus memuat schema aksi")
	}
}
