package ask

import (
	"strings"
	"testing"
)

const knownJSON = `{"text":"p95 delivery 12 000 ms (kpi.sim.delivery_p95_ms), melanggar SLO.","sources":["kpi.sim.delivery_p95_ms"]}`

func TestValidateAcceptsCitedAnswer(t *testing.T) {
	known := []Source{{ID: "kpi.sim.delivery_p95_ms", Label: "p95", Value: "12000 ms"}}
	a, err := Validate(knownJSON, known)
	if err != nil {
		t.Fatal(err)
	}
	if a.Text == "" || len(a.Sources) != 1 || a.Sources[0] != "kpi.sim.delivery_p95_ms" {
		t.Fatalf("jawaban salah: %+v", a)
	}
}

func TestValidateRejectsAnswerWithoutCitations(t *testing.T) {
	known := []Source{{ID: "kpi.raw", Label: "raw", Value: "{}"}}
	if _, err := Validate(`{"text":"karangan bebas","sources":[]}`, known); err == nil {
		t.Fatal("jawaban tanpa sitasi HARUS ditolak")
	}
	if _, err := Validate(`{"text":"karangan bebas"}`, known); err == nil {
		t.Fatal("jawaban tanpa field sources HARUS ditolak")
	}
}

func TestValidateRejectsUnknownCitation(t *testing.T) {
	known := []Source{{ID: "kpi.raw", Label: "raw", Value: "{}"}}
	if _, err := Validate(`{"text":"x","sources":["hallucinated.id"]}`, known); err == nil {
		t.Fatal("sitasi tak dikenal HARUS ditolak")
	}
}

func TestValidateRejectsGarbageAndEmpty(t *testing.T) {
	known := []Source{{ID: "kpi.raw", Label: "raw", Value: "{}"}}
	if _, err := Validate("belum tentu", known); err == nil {
		t.Fatal("bukan JSON harus ditolak")
	}
	if _, err := Validate(`{"sources":["kpi.raw"]}`, known); err == nil {
		t.Fatal("jawaban tanpa teks harus ditolak")
	}
}

func TestBuildUserPromptListsSources(t *testing.T) {
	p := BuildUserPrompt("kenapa p95 naik?", []Source{
		{ID: "kpi.sim.delivery_p95_ms", Label: "p95", Value: "12000 ms"},
		{ID: "incident.x1", Label: "Incident", Value: "status=OPEN"},
	})
	for _, want := range []string{"id=kpi.sim.delivery_p95_ms", "id=incident.x1", "kenapa p95 naik?"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt harus memuat %q", want)
		}
	}
	if !strings.Contains(SystemPrompt(), "sources") {
		t.Fatal("system prompt harus mensyaratkan sources")
	}
}
