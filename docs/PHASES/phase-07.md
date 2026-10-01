# PHASE 07 — (Opsional) AI Ops Copilot & Plan Advisor

> Spesifikasi fase 7 (ditulis akhir fase 6). Fase OPSIONAL — dikerjakan hanya bila ada
> waktu/semangat setelah fase 1–6 tuntas. Prinsip keras yang tidak bisa ditawar:
> **core 100% deterministik tanpa LLM** (ADR D8) — LLM hanya plugin adapter di luar
> engine, dan **tanpa API key → fitur tersembunyi, aplikasi tetap utuh**.

## Tujuan

Menunjukkan pola integrasi LLM yang sehat untuk operasi last-mile: LLM TIDAK pernah
menjadi sumber kebenaran dan TIDAK pernah menulis ke produksi sendiri. Ia hanya:

1. **Plan Advisor** — membaca metrik nyata, mengusulkan 2–3 plan terstruktur yang
   masing-masing **di-dry-run di simulator deterministik**; keputusan akhir tetap
   manusia (admin klik, melihat angka prediksi per plan, lalu eksekusi live).
2. **Ops Copilot** — tanya-jawab atas metrik & incident internal dengan **sitasi data**
   (bukan karangan), mis. "kenapa p95 delivery naik jam 18:00?" → jawaban mengutip
   incident chaos / perubahan surge / KPI ring.

## Arsitektur adapter (ADR D8 diperkuat)

```
apps/services/copilot/          (profile `copilot`, TIDAK di demo default)
  internal/llm/  provider.go    interface LLM { Complete(req) (resp, err) }
                 openai.go      implementasi 1: env OPENAI_API_KEY/OPENAI_BASE_URL
                 noop.go        tanpa key → selalu ErrNoLLM
  internal/advise/ plans.go     metrik → prompt → parse plan JSON ketat (schema fixed)
  internal/eval/  groundtruth.go 20 kasus kecil + penilaian jawaban (fase eval)
```

- **Tanpa `OPENAI_API_KEY` (atau kosong):**
  - service `copilot` tidak dideploy (profile `copilot` off — default);
  - UI menyembunyikan panel Copilot & tombol Advisor sepenuhnya (deteksi via
    `GET /api/copilot/capabilities` → 404/`{"enabled":false}`);
  - seluruh 6 layar Control Room, Golden Demo, Strategy Lab, Replay — **tetap
    berfungsi 100%** (uji regresi wajib).
- **Dengan key:** panel muncul; semua call LLM ber-timeout ketat (8 s), dibatasi
  (rate limit per menit di service), dan **gagal = tombol kembali nonaktif + pesan
  kecil**, bukan error layar.
- **Tidak ada secret di repo/git** — hanya env di VPS/CI secret; `OPENAI_BASE_URL`
  membuat provider bisa diganti (OpenAI-compatible mana pun) tanpa refactor.
- LLM tidak pernah punya kredensial: ia hanya menghasilkan TEKS plan; eksekusi
  tetap via endpoint kontrol existing (`sim-control`, `chaos`) dipicu manusia.

## Scope (kerjakan ini saja)

### Backend — service `copilot` (Go, :4207, internal saja)
1. `GET /api/copilot/capabilities` (via proxy api-gateway) → `{"enabled":bool}`.
2. `POST /api/copilot/advise` → ambil `GET /api/kpi` + `/api/metrics` + incident
   terakhir (data yang SUDAH ada — tanpa akses DB langsung) → 1 call LLM dengan
   schema keluaran ketat:
   `[{name, rationale, actions:[{kind: surge|weather|strategy|kill, params}]}]`
   — maks 3 plan; plan yang gagal parse dibuang (bukan diperbaiki diam-diam).
3. **Dry-run tiap plan**: jalankan plan di engine virtual (reuse jalur duel
   `internal/duel` — seed sama, baseline vs plan) → kembalikan metrik prediksi
   (p50/p95 delivery, cost/order, delivered) per plan. **LLM tidak dieksekusi —
   simulator yang dieksekusi.**
4. `POST /api/copilot/ask` (Copilot) → pertanyaan + context internal (KPI, incident,
   metrik) sebagai sitasi → jawaban wajib menyertakan referensi data yang dipakai
   (field `sources:[...]`); jawaban tanpa sitasi ditolak.
5. Healthz + limit RAM (≤ 128 MiB) + profile compose `copilot` + proxy api-gateway.

### Frontend
6. Panel **Plan Advisor** (di Strategy Lab atau System Health): daftar plan +
   metrik prediksi side-by-side + tombol "Execute" PER PLAN dengan konfirmasi
   2 langkah (pola chaos) — eksekusi memakai endpoint kontrol yang sudah ada.
   Tanpa LLM: panel tidak ada di DOM sama sekali.
7. Panel **Ask Ops**: input teks + jawaban + daftar sitasi (klik → buka panel
   sumber: KPI/incident). Tanpa LLM: tidak ada.

### Evaluasi & docs
8. **Evaluasi kualitas terdokumentasi** (`reports/phase-07-copilot.md`): ground
   truth kecil (≥ 15 kasus: 5 diag surging, 5 diag incident, 5 tanya-metrik) —
   jawaban dinilai: benar/parsial/salah + sitasi tepat/tidak; angka di laporan.
9. ADR baru bila ada keputusan arsitektur; PROGRESS + ROADMAP di commit penutup.

## Di luar scope (dilarang)

- Menyentuh `internal/sim`/`pkg/dispatch` (core beku); LLM menulis ke engine;
  auto-execution tanpa konfirmasi manusia; streaming UI mahal; vector DB / RAG
  infrastruktur besar; provider SDK berat (HTTP murni cukup); auth besar.

## Definition of Done

- [ ] Tanpa API key: fitur 100% tersembunyi, seluruh aplikasi utuh (uji regresi
      headless: 6 layar + Golden Demo + replay tidak berubah), CI hijau tanpa key
- [ ] Dengan API key: Advisor menghasilkan ≤ 3 plan → SEMUA di-dry-run simulator
      (angka prediksi tampil) → eksekusi hanya setelah konfirmasi manusia
- [ ] Copilot menjawab dengan sitasi data internal; jawaban tanpa sitasi ditolak
- [ ] Evaluasi kualitas terdokumentasi di `reports/phase-07-copilot.md` (ground
      truth ≥ 15 kasus, hasil jujur termasuk yang salah)
- [ ] Service baru: health + limit RAM + profile `copilot` off-by-default; budget
      RAM stack tetap ≤ 2 GB dalam mode demo (tanpa copilot)
- [ ] Commit & push (CI hijau); fase ditandai opsional-done bila DoD penuh

## Catatan Teknis

- Prompt = data terstruktur (JSON KPI/incident), bukan prosa panjang; output
  diparse ketat — satu parser, satu schema, versi di satu tempat.
- Dry-run plan memakai seed & preset yang sama dengan baseline (keadilan = D18);
  durasi duel pendek (60–120 s virtual) agar interaktif.
- Rate limit service-level sederhana (token bucket di memori) — bukan auth besar
  (lihat D23); endpoint `copilot` ikut aturan exposure yang sama.
- Jika tidak ada waktu: fase ini BOLEH dilewati — ROADMAP sudah menandainya
  opsional; jangan setengah-jalan (adapter tanpa evaluasi = tidak dirilis).
