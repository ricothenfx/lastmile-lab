package eval

import "testing"

func TestCaseCountAndKinds(t *testing.T) {
	if len(Cases) < 15 {
		t.Fatalf("DoD: ≥15 kasus, dapat %d", len(Cases))
	}
	count := map[Kind]int{}
	for _, c := range Cases {
		count[c.Kind]++
		if len(c.Keywords) == 0 || len(c.RequiredSourcePrefix) == 0 {
			t.Fatalf("kasus %s tidak lengkap", c.ID)
		}
	}
	if count[KindSurgeDiag] != 5 || count[KindIncidentDiag] != 5 || count[KindMetric] != 5 {
		t.Fatalf("komposisi harus 5/5/5: %v", count)
	}
}

func TestScoreAnswerLevels(t *testing.T) {
	c := Cases[0] // s1: surge diag, CorrectMin 2
	if v := ScoreAnswer(c, "Antrean membengkak karena surge 4× — orders_per_min 34 melampaui kapasitas."); v != Correct {
		t.Fatalf("harus benar, dapat %s", v)
	}
	if v := ScoreAnswer(c, "Kemungkinan surge."); v != Partial {
		t.Fatalf("harus parsial, dapat %s", v)
	}
	if v := ScoreAnswer(c, "Tidak tahu."); v != Wrong {
		t.Fatalf("harus salah, dapat %s", v)
	}
}

func TestCitationsOK(t *testing.T) {
	c := Cases[5] // i1: incident diag
	if !CitationsOK(c, []string{"incident.abc"}) {
		t.Fatal("sitasi incident harus diterima")
	}
	if !CitationsOK(c, []string{"incidents.summary"}) {
		t.Fatal("sitasi ringkasan harus diterima")
	}
	if CitationsOK(c, []string{"kpi.sim.surge"}) {
		t.Fatal("sitasi di luar area incident harus ditolak")
	}
}
