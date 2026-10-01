// Package eval — ground truth kecil untuk menilai kualitas jawaban
// copilot (Fase 7, spec §Evaluasi). 15 kasus: 5 diagnosis surge,
// 5 diagnosis incident, 5 tanya-metrik. Penilaian MURNI fungsi — harness
// eksekusinya butuh API key (pemilik), sehingga tanpa key bagian ini
// tertes unit-testnya dan laporannya menandai evaluasi live "menunggu
// API key pemilik".
package eval

import "strings"

// Kind kategori kasus.
type Kind string

const (
	KindSurgeDiag    Kind = "surge_diag"
	KindIncidentDiag Kind = "incident_diag"
	KindMetric       Kind = "metric"
)

// Case satu ground truth. Keywords dinilai bertingkat:
//   - correct: ≥ CorrectMin keyword muncul di jawaban
//   - partial: ≥ 1 keyword
//   - wrong: 0 keyword
//
// RequiredSourcePrefix: sitasi jawaban wajib menyentuh area data ini
// (mis. "kpi.sim.surge" → prefix "kpi.sim.surge" atau "kpi.raw").
type Case struct {
	ID                   string   `json:"id"`
	Kind                 Kind     `json:"kind"`
	Question             string   `json:"question"`
	Keywords             []string `json:"keywords"`               // kata/framen yang wajib muncul pada jawaban benar
	CorrectMin           int      `json:"correct_min"`            // ambang correct (default 2)
	RequiredSourcePrefix []string `json:"required_source_prefix"` // salah satu prefix ini wajib dikutip
}

// Verdict nilai kata kunci jawaban.
type Verdict string

const (
	Correct Verdict = "benar"
	Partial Verdict = "parsial"
	Wrong   Verdict = "salah"
)

// Cases ground truth (15 kasus, spec DoD ≥ 15).
var Cases = []Case{
	// ---- 5 diagnosis surge ----
	{
		ID: "s1", Kind: KindSurgeDiag,
		Question:   "Kenapa antrean order membengkak sekarang?",
		Keywords:   []string{"surge", "orders_per_min", "flash", "demand", "waiting"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.surge", "kpi.orders_per_min", "kpi.sim.orders_waiting", "kpi.raw"},
	},
	{
		ID: "s2", Kind: KindSurgeDiag,
		Question:   "Apakah p95 delivery naik karena flash sale?",
		Keywords:   []string{"surge", "p95", "delivery_p95_ms", "kapasitas", "rider"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.delivery_p95_ms", "kpi.sim.surge", "kpi.raw"},
	},
	{
		ID: "s3", Kind: KindSurgeDiag,
		Question:   "Berapa laju order saat ini dibanding kapasitas armada?",
		Keywords:   []string{"orders_per_min", "riders", "utilis", "menit"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.orders_per_min", "kpi.sim.utilization_pct", "kpi.raw"},
	},
	{
		ID: "s4", Kind: KindSurgeDiag,
		Question:   "Kenapa banyak order expired dalam jam terakhir?",
		Keywords:   []string{"expired", "ttl", "antre", "surge", "waiting"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.orders_expired", "kpi.sim.orders_waiting", "kpi.raw"},
	},
	{
		ID: "s5", Kind: KindSurgeDiag,
		Question:   "Kalau demand naik 2× apa yang harus disiapkan?",
		Keywords:   []string{"surge", "kapasitas", "rider", "strategi", "p95"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.surge", "kpi.sim.utilization_pct", "kpi.orders_per_min", "kpi.raw"},
	},

	// ---- 5 diagnosis incident ----
	{
		ID: "i1", Kind: KindIncidentDiag,
		Question:   "Apakah ada service yang mati barusan?",
		Keywords:   []string{"incident", "kill", "chaos", "recover", "mttd"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"incident.", "incidents.summary"},
	},
	{
		ID: "i2", Kind: KindIncidentDiag,
		Question:   "Berapa MTTR rata-rata dari eksperimen chaos?",
		Keywords:   []string{"mttr", "ms", "rata"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"incidents.summary"},
	},
	{
		ID: "i3", Kind: KindIncidentDiag,
		Question:   "Kenapa rider menghilang sesaat dari peta?",
		Keywords:   []string{"kill", "rider-sim", "restart", "incident", "self-heal"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"incident.", "incidents.summary"},
	},
	{
		ID: "i4", Kind: KindIncidentDiag,
		Question:   "Incident mana yang masih terbuka?",
		Keywords:   []string{"open", "terbuka", "recover", "incident"},
		CorrectMin: 1, RequiredSourcePrefix: []string{"incident.", "incidents.summary", "kpi.incidents_open"},
	},
	{
		ID: "i5", Kind: KindIncidentDiag,
		Question:   "Bagaimana dampak chaos terhadap availability?",
		Keywords:   []string{"availability", "budget", "%", "downtime"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"incidents.summary", "kpi.availability_pct"},
	},

	// ---- 5 tanya-metrik ----
	{
		ID: "m1", Kind: KindMetric,
		Question:   "Berapa p50 delivery saat ini?",
		Keywords:   []string{"p50", "ms"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.delivery_p50_ms", "kpi.raw"},
	},
	{
		ID: "m2", Kind: KindMetric,
		Question:   "Strategi dispatch apa yang aktif?",
		Keywords:   []string{"fifo", "batching", "zone", "optimal"},
		CorrectMin: 1, RequiredSourcePrefix: []string{"kpi.sim.strategy", "kpi.raw"},
	},
	{
		ID: "m3", Kind: KindMetric,
		Question:   "Berapa utilisasi armada sekarang?",
		Keywords:   []string{"utilisasi", "%", "busy"},
		CorrectMin: 1, RequiredSourcePrefix: []string{"kpi.sim.utilization_pct", "kpi.raw"},
	},
	{
		ID: "m4", Kind: KindMetric,
		Question:   "Berapa biaya per order yang terkirim?",
		Keywords:   []string{"km", "cost", "biaya", "per order"},
		CorrectMin: 1, RequiredSourcePrefix: []string{"kpi.sim.cost_per_order_km", "kpi.raw"},
	},
	{
		ID: "m5", Kind: KindMetric,
		Question:   "Berapa p99 keputusan dispatch — masih di bawah 50 ms?",
		Keywords:   []string{"p99", "dispatch", "ms", "50"},
		CorrectMin: 2, RequiredSourcePrefix: []string{"kpi.sim.p99_dispatch_ms", "kpi.raw"},
	},
}

// ScoreAnswer menilai teks jawaban terhadap kasus (tanpa memandang sitasi).
func ScoreAnswer(c Case, answerText string) Verdict {
	min := c.CorrectMin
	if min <= 0 {
		min = 2
	}
	text := strings.ToLower(answerText)
	hits := 0
	for _, k := range c.Keywords {
		if strings.Contains(text, strings.ToLower(k)) {
			hits++
		}
	}
	switch {
	case hits >= min:
		return Correct
	case hits >= 1:
		return Partial
	default:
		return Wrong
	}
}

// CitationsOK memeriksa jawaban mengutip area data yang diharapkan kasus.
func CitationsOK(c Case, citedIDs []string) bool {
	for _, id := range citedIDs {
		for _, prefix := range c.RequiredSourcePrefix {
			if strings.HasPrefix(strings.TrimSpace(id), prefix) {
				return true
			}
		}
	}
	return false
}
