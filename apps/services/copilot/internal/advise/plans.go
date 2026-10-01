// Package advise — Plan Advisor (Fase 7): metrik nyata → prompt terstruktur
// → 1 call LLM → parse plan JSON ketat (schema fixed). Plan yang gagal
// parse DIBUANG (bukan diperbaiki diam-diam); maksimal 3 plan. LLM tidak
// pernah mengeksekusi apa pun — dry-run dilakukan simulator (internal/dryrun).
package advise

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
)

// ActionKind jenis aksi yang bisa diusulkan LLM — enums tertutup.
type ActionKind string

const (
	KindSurge    ActionKind = "surge"
	KindWeather  ActionKind = "weather"
	KindStrategy ActionKind = "strategy"
	KindKill     ActionKind = "kill"
)

// Action satu langkah operasional dalam plan. Params longgar di wire,
// divalidasi ketat di Validate.
type Action struct {
	Kind   ActionKind        `json:"kind"`
	Params map[string]string `json:"params"`
}

// Plan usulan LLM — murni TEKS + parameter; tidak punya kredensial dan
// tidak pernah dieksekusi otomatis (eksekusi = klik manusia di UI).
type Plan struct {
	Name      string   `json:"name"`
	Rationale string   `json:"rationale"`
	Actions   []Action `json:"actions"`
}

// MaxPlans batas keras jumlah plan per advise.
const MaxPlans = 3

// systemPrompt instruksi schema — satu sumber kebenaran (versi di satu tempat).
const systemPrompt = `Kamu Ops Copilot platform last-mile "Pulse". Tugasmu MENGUSULKAN rencana,
BUKAN mengeksekusi. Semua angka yang kamu sebut WAJIB berasal dari konteks JSON yang diberikan —
dilarang mengarang angka. Jawab HANYA JSON objek dengan schema:

{"plans":[{"name":string,"rationale":string,"actions":[{"kind":"surge"|"weather"|"strategy"|"kill","params":object}]}]}

Aturan:
- Maksimal 3 plan, urut dari yang paling direkomendasikan.
- kind "surge": params {"factor":"<1..10>"} (ubah permintaan/demand; naikkan hanya untuk menguji kapasitas).
- kind "weather": params {"factor":"<0.3..1>"} (faktor cuaca; 1 = cerah).
- kind "strategy": params {"name":"fifo"|"batching"|"zone"|"optimal"} (strategi dispatch).
- kind "kill": params {"target":"<nama container lastmile>"} (chaos drill — dry-run tidak bisa mensimulasikannya).
- rationale ≤ 2 kalimat, menyebut angka konteks yang relevan.
- Tidak ada field lain, tidak ada penjelasan di luar JSON.`

// BuildUserPrompt merangkai konteks internal (JSON metrik nyata yang SUDAH
// ada) — prompt = data terstruktur, bukan prosa panjang.
func BuildUserPrompt(contextJSON string, strategy string) string {
	return fmt.Sprintf(`Konteks operasi SAAT INI (JSON dari sistem — satu-satunya sumber angka):
%s

Strategi dispatch aktif: %q.
Usulkan hingga 3 plan perbaikan/pengujiannya sesuai schema.`, contextJSON, strategy)
}

// ParsePlans mem-parsing keluaran LLM secara KETAT: harus JSON objek/array
// sesuai schema; field tak dikenal diabaikan; plan/action invalid dibuang;
// lebih dari MaxPlans dipotong. Hasil kosong = error (bukan fallback diam).
func ParsePlans(raw string) ([]Plan, error) {
	var doc struct {
		Plans []Plan `json:"plans"`
	}
	trimmed := strings.TrimSpace(raw)
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		// provider kadang membungkus code fence — ambil blok {…} terluar.
		i, j := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}")
		if i < 0 || j <= i || json.Unmarshal([]byte(trimmed[i:j+1]), &doc) != nil {
			return nil, fmt.Errorf("keluaran llm bukan JSON schema: %w", err)
		}
	}
	out := make([]Plan, 0, MaxPlans)
	for _, p := range doc.Plans {
		if len(out) == MaxPlans {
			break
		}
		if !validPlan(p) {
			continue // plan gagal parse dibuang — bukan diperbaiki diam-diam
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("llm tidak menghasilkan plan valid")
	}
	return out, nil
}

func validPlan(p Plan) bool {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Rationale) == "" || len(p.Actions) == 0 {
		return false
	}
	for _, a := range p.Actions {
		if !validAction(a) {
			return false
		}
	}
	return true
}

func validAction(a Action) bool {
	switch a.Kind {
	case KindSurge:
		f, ok := paramFloat(a.Params, "factor")
		return ok && f >= 1 && f <= 10
	case KindWeather:
		f, ok := paramFloat(a.Params, "factor")
		return ok && f >= 0.3 && f <= 1
	case KindStrategy:
		_, err := dispatch.ByName(a.Params["name"])
		return err == nil
	case KindKill:
		return strings.TrimSpace(a.Params["target"]) != ""
	default:
		return false
	}
}

// PlanError Deskripsi kenapa sebuah plan dibuang — untuk respons API agar
// pembuangan tidak diam-diam di sisi klien.
func PlanError(p Plan) string {
	if strings.TrimSpace(p.Name) == "" {
		return "plan tanpa nama"
	}
	if strings.TrimSpace(p.Rationale) == "" {
		return "plan tanpa rationale"
	}
	if len(p.Actions) == 0 {
		return "plan tanpa aksi"
	}
	for _, a := range p.Actions {
		if !validAction(a) {
			return fmt.Sprintf("aksi %q tidak valid (kind/params di luar schema)", a.Kind)
		}
	}
	return ""
}

func paramFloat(m map[string]string, key string) (float64, bool) {
	var f float64
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%g", &f); err != nil {
		return 0, false
	}
	return f, true
}

// ParamFloat helper publik untuk dryrun/UI mapping.
func ParamFloat(m map[string]string, key string) (float64, bool) {
	return paramFloat(m, key)
}

// SystemPrompt exposes instruksi schema (dipakai handler & test).
func SystemPrompt() string { return systemPrompt }
