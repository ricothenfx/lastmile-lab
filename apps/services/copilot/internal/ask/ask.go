// Package ask — Ops Copilot tanya-jawab (Fase 7): pertanyaan + context
// internal (KPI, incident, metrik) sebagai sitasi bernomor. Jawaban LLM
// WAJIB menyertakan field sources:[id]; jawaban tanpa sitasi atau dengan
// id tak dikenal DITOLAK — copilot tidak boleh menjawab karangan.
package ask

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Source satu potongan data internal yang boleh dikutip jawaban.
type Source struct {
	ID    string `json:"id"`    // stabil, mis. "kpi.sim.delivery_p95_ms"
	Label string `json:"label"` // ringkasan manusiawi
	Value string `json:"value"` // angka/isi mentah dari sistem
}

// Answer keluaran LLM yang diharapkan (JSON ketat).
type Answer struct {
	Text    string   `json:"text"`
	Sources []string `json:"sources"`
}

const systemPrompt = `Kamu Ops Copilot "Pulse" untuk operasi last-mile. Jawab pertanyaan operator HANYA
berdasarkan konteks data internal yang diberikan (daftar sumber bernomor dengan id).
Dilarang memakai pengetahuan luar atau mengarang angka. Jawab HANYA JSON objek:

{"text":"<jawaban ≤ 4 kalimat, sebut angka dari sumber>","sources":["<id sumber yang dipakai>"]}

Aturan:
- sources WAJIB berisi minimal 1 id dari konteks; tanpa sitasi jawaban akan DITOLAK.
- Sebut angka apa adanya dari field value sumber.
- Bila konteks tidak memuat jawaban: text = "Data internal tidak mencakup pertanyaan ini." + sources berisi sumber terdekat yang kamu periksa.`

// SystemPrompt exposes instruksi (dipakai handler & test).
func SystemPrompt() string { return systemPrompt }

// BuildUserPrompt menyusun pertanyaan + konteks sitasi bernomor.
func BuildUserPrompt(question string, sources []Source) string {
	var b strings.Builder
	b.WriteString("Sumber data internal (kutip via id):\n")
	for _, s := range sources {
		fmt.Fprintf(&b, "- id=%s | %s | %s\n", s.ID, s.Label, s.Value)
	}
	fmt.Fprintf(&b, "\nPertanyaan operator: %s\nJawab sesuai schema (JSON saja).", question)
	return b.String()
}

// Validate menolak jawaban tanpa sitasi / dengan id tak dikenal.
// Murni — tertes tanpa LLM.
func Validate(raw string, known []Source) (*Answer, error) {
	var a Answer
	trimmed := strings.TrimSpace(raw)
	if err := json.Unmarshal([]byte(trimmed), &a); err != nil {
		// provider kadang membungkus code fence — ambil blok {…} terluar.
		i, j := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}")
		if i < 0 || j <= i || json.Unmarshal([]byte(trimmed[i:j+1]), &a) != nil {
			return nil, fmt.Errorf("jawaban llm bukan JSON schema")
		}
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, fmt.Errorf("jawaban llm tanpa teks")
	}
	if len(a.Sources) == 0 {
		return nil, fmt.Errorf("jawaban tanpa sitasi ditolak")
	}
	knownIDs := make(map[string]bool, len(known))
	for _, s := range known {
		knownIDs[s.ID] = true
	}
	for _, id := range a.Sources {
		if !knownIDs[strings.TrimSpace(id)] {
			return nil, fmt.Errorf("sitasi %q tidak dikenal — jawaban ditolak", id)
		}
	}
	return &a, nil
}
