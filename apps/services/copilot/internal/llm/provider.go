// Package llm — adapter LLM untuk copilot (Fase 7, ADR D8 diperkuat D24).
//
// Prinsip keras: LLM HANYA plugin di luar engine. Tanpa API key service
// memakai provider noop yang selalu gagal dengan ErrNoLLM — seluruh fitur
// copilot menolak rapi dan aplikasi tetap utuh. Tidak ada SDK berat:
// provider OpenAI-compatible diakses HTTP murni.
package llm

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

// ErrNoLLM dikembalikan provider noop (tanpa API key) — pemanggil handler
// menerjemahkannya menjadi 503 {"error":"llm_disabled"}.
var ErrNoLLM = errors.New("llm_disabled — OPENAI_API_KEY tidak diset")

// Request satu panggilan completion. JSONMode meminta keluaran JSON ketat
// (response_format json_object pada provider OpenAI-compatible).
type Request struct {
	System   string
	User     string
	JSONMode bool
}

// Response teks completion mentah (validasi schema dilakukan pemanggil).
type Response struct {
	Text     string
	Model    string
	Duration time.Duration
}

// Provider interface LLM — satu metode, mudah di-stub di test.
type Provider interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

// New memilih provider dari env: OPENAI_API_KEY kosong → noop (fitur
// tersembunyi); terisi → OpenAI-compatible via OPENAI_BASE_URL (default
// https://api.openai.com/v1). Timeout ketat per call (default 8 s).
func New() Provider {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		return Noop{}
	}
	return NewOpenAI(envStr("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		os.Getenv("OPENAI_API_KEY"), envStr("OPENAI_MODEL", "gpt-4o-mini"), 8*time.Second)
}

// Enabled melaporkan ketersediaan LLM — sumber kebenaran /capabilities.
func Enabled() bool { return strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" }

func envStr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
