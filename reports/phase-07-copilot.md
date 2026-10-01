# Fase 7 — AI Ops Copilot & Plan Advisor (laporan)

> Status: **selesai (engineering penuh, mode tanpa key terverifikasi)**.
> Bagian evaluasi kualitas jawaban live: **MENUNGGU API KEY PEMILIK** (lihat §6).
> Prinsip keras (ADR D8 diperkuat D24): core 100% deterministik tanpa LLM —
> LLM hanya plugin pengusul teks; **simulator yang menjadi judge; manusia yang mengeksekusi**.

## 1. Arsitektur

```
apps/services/copilot/            (:4207, profile compose `copilot` — OFF-by-default,
  main.go                           internal saja, mem_limit 128 MiB, healthcheck /healthz)
  internal/llm/provider.go        interface LLM { Complete(ctx, req) (resp, err) } + ErrNoLLM
  internal/llm/noop.go            tanpa OPENAI_API_KEY → selalu ErrNoLLM
  internal/llm/openai.go          OpenAI-compatible HTTP murni (OPENAI_BASE_URL, timeout 8 s,
                                  response_format json_object, temperature 0.2)
  internal/advise/plans.go        metrik → prompt (JSON terstruktur) → parse schema ketat
                                  [{name, rationale, actions:[{kind, params}]}] ≤3 plan;
                                  plan/action invalid DIBUANG (alasan dilaporkan, bukan
                                  diperbaiki diam-diam)
  internal/ask/ask.go             Q&A: jawaban WAJIB sources:[id]; tanpa sitasi / id tak
                                  dikenal → DITOLAK
  internal/opsctx/                context internal dari endpoint yang SUDAH ADA di
                                  api-gateway (/api/kpi, /api/metrics, /api/chaos/incidents)
                                  — TANPA akses DB langsung; fetch paralel ≤3 s, bagian
                                  gagal dilaporkan jujur (`_missing`), bukan dipalsukan
  internal/dryrun/dryrun.go       DRY-RUN = jalur duel existing `internal/duel` (ADR D18):
                                  baseline vs plan, SATU generator seed sama → dua engine
                                  identik; 120 s virtual; mapping aksi: strategy→strategi
                                  sisi plan, surge→skala laju generator, weather→WeatherFactor;
                                  kill TIDAK bisa disimulasikan → plan tanpa angka palsu + Note
  internal/eval/groundtruth.go    15 kasus ground truth (5 surge / 5 incident / 5 metrik) +
                                  penilaian benar/parsial/salah + cek area sitasi
```

- **Proxy api-gateway** (`copilot.go`): tanpa `COPILOT_URL` → `/api/copilot/capabilities`
  tetap menjawab `{"enabled":false}` (200 — UI tidak menebak dari 404); endpoint lain 503
  `copilot_disabled`. Dengan `COPILOT_URL` → passthrough method+body (client khusus 45 s —
  advise = LLM 8 s + dry-run duel).
- **Rate limit**: token bucket di memori, default 6/menit per endpoint (spec: sederhana,
  bukan auth besar — exposure ikut aturan D23).
- **Ram stack**: mode demo TIDAK berubah (copilot off) — tetap 1 888 MiB ≤ 2 GB.
  Profile `copilot` aktif menambah +128 MiB limit (copilot) → 2 016 MiB hanya bila
  pemilik menyalakannya secara sadar; demo default tidak pernah memuatnya.

## 2. "Tanpa key = fitur tersembunyi, aplikasi tetap utuh" — TERVERIFIKASI

Environment tanpa `OPENAI_API_KEY` (semua yang dijalankan di sesi ini):

| Cek | Hasil |
|---|---|
| `GET /api/copilot/capabilities` (gateway tanpa COPILOT_URL) | `200 {"enabled":false}` |
| `GET /capabilities` (service copilot asli, container smoke di network lastmile) | `{"enabled":false}` (healthz ok, graph 9 459 node) |
| `POST /advise` / `POST /ask` tanpa key | `503 {"error":"llm_disabled — copilot tanpa OPENAI_API_KEY"}` |
| UI headless: node `[data-testid="copilot-advisor"] / [data-testid="copilot-ask"]` | **0 node** — teks "ADVISOR"/"COPILOT" tidak ada di body |
| Strategy Lab | form duel persis baseline — **tanpa tab bar** (DOM identik) |
| System Health | 2 tab (PIPELINE/CHAOS) grid 2 kolom — **persis baseline** |
| Regresi 6 layar + Golden Demo + replay | **ALL HIDDEN CHECKS PASSED** — KPI Deck (angka nyata), Surge Console, Strategy Lab, System Health, Replay & Inspect (`SESI LIVE`), Golden Demo dinner-rush **7/7 langkah** (kill rider-sim → pulih), **console error 0** |
| Skrip | `reports/phase-07-ui-verify.mjs` (MODE default = hidden; MODE=stub = jalur enabled) |

## 3. Jalur enabled — diuji TANPA API key via route-stub Playwright

`MODE=stub` menyuntik `{"enabled":true}` + respons LLM stub di level jaringan browser
(tanpa menyentuh backend, tanpa secret):

- Tab **ADVISOR** muncul di Strategy Lab; GENERATE PLANS → 2 plan dirender dengan tabel
  **BASE | PLAN | Δ** (mono tabular, arah Δ diwarnai); plan kill-only tampil **tanpa
  angka prediksi** + Note jujur ("tidak tersimulasi dry-run (incident runtime)").
- **EXECUTE 2 langkah** (CONFIRM? pola chaos) → aksi dieksekusi via endpoint kontrol
  existing (`/api/control/surge` 200 stub; aksi `strategy` jujur ditandai **SKIP — butuh
  restart env `DISPATCH_STRATEGY`** karena tidak ada endpoint kontrol strategi live).
- Tab **COPILOT** muncul di System Health; ASK → jawaban + daftar sitasi (id + label +
  value); jawaban stub tanpa sitasi → **422 `answer_rejected`** tampil sebagai pesan
  penolakan, isi jawaban tidak pernah dirender.
- Console error 0 (di luar 422 expected dari uji penolakan).
- Bukti: `reports/phase07-advisor-enabled.png`, `reports/phase07-ask-enabled.png`.

## 4. Angka & performa

| Item | Nilai |
|---|---|
| Dry-run duel 120 s virtual ×2 engine, graph Berlin penuh (9 459 node) | **±0,22 s wall** (unit test `TestRunRealGraphWallTime`, host load ~6–8) — jauh di bawah budget interaktif; 3 plan ≈ <1 s + 1 call LLM ≤8 s |
| mem_limit copilot | 128 MiB (spec) — aktual saat smoke < 20 MiB |
| p99 keputusan dispatch | tidak berubah — core beku (`internal/sim`, `pkg/dispatch` TIDAK disentuh; determinisme duel tetap diuji) |
| CI | gofmt/vet/test **19 paket hijau** (termasuk ±30 test baru); web typecheck+build hijau |

**Perbaikan bug yang ditemukan saat verifikasi fase ini** (api-gateway, bukan core):
`/api/replay/*` memakai `http.Client` umum 4 s — dump ring penuh (±21 MB gzip, 4 500
frame) butuh >4 s di host berbeban → stream terpotong → browser jatuh ke fixture
("Unterminated string in JSON at position ~50 MB"). Fix: client khusus replay 30 s
(satu baris + komentar). Regresi replay diverifikasi ulang: `SESI LIVE` muncul (t+10 s).

## 5. Keamanan & batasan

- **Tidak ada secret di repo/git** — compose membaca `${LASTMILE_OPENAI_API_KEY:-}`
  (kosong = off) dari `.env` VPS; mengisi key = aksi pemilik (lihat runbook §4.2).
- LLM tidak pernah punya kredensial; ia hanya menghasilkan TEKS. Eksekusi = tombol
  manusia (konfirmasi 2 langkah) di endpoint kontrol existing.
- Provider bisa diganti (OpenAI-compatible apa pun) via `OPENAI_BASE_URL` tanpa refactor;
  HTTP murni, tanpa SDK.
- Jawaban Ask Ops tanpa sitasi / sitasi tak dikenal → 422 (tidak pernah ditampilkan).
- Port 4207 container-only, terdaftar di `/home/rico/PORTS.md`.

## 6. Evaluasi kualitas jawaban — ⏳ MENUNGGU API KEY PEMILIK

Kode + ground truth sudah lengkap (`internal/eval/groundtruth.go`, **15 kasus: 5 diag
surge, 5 diag incident, 5 tanya-metrik**, masing-masing dengan keywords ambang
benar/parsial dan prefix sitasi wajib); unit test penilaian hijau. Yang belum bisa
dijalankan adalah **eksekusi live evaluasi** (15 pertanyaan × LLM asli + penilaian
angka di laporan ini) karena environment tidak memiliki `OPENAI_API_KEY`.

**Prosedur eksekusi (pemilik, ±10 menit setelah key ada):**

1. Isi `LASTMILE_OPENAI_API_KEY` di `.env` VPS → `docker compose -p lastmile
   --profile sim --profile chaos --profile copilot up -d`.
2. Jalankan 15 kasus (loop `eval.Cases` → `POST /api/copilot/ask` → `ScoreAnswer` +
   `CitationsOK` — penilaian otomatis dari kode yang sama dengan unit test).
3. Laporkan di bagian ini: benar/parsial/salah per kasus + sitasi tepat/tidak + jumlah
   jawaban ditolak (tanpa sitasi). Hasil jujur termasuk yang salah — sesuai DoD.

Sampai langkah itu dijalankan, DoD "Evaluasi kualitas terdokumentasi" berstatus
**parsial-by-design**: infrastruktur + skoring teruji, angka live menunggu key.

## 7. Bukti (berkas)

| Berkas | Isi |
|---|---|
| `reports/phase07-hidden-*.png` (via `/out` = `/tmp/kilo/copilot-it`) | 6 layar mode hidden + setelah Golden Demo |
| `reports/phase07-advisor-enabled.png`, `reports/phase07-ask-enabled.png` | jalur enabled via stub |
| `reports/phase-07-ui-verify.mjs` | skrip verifikasi headless (hidden + stub) |
| Unit test `copilot/...`, `api-gateway` | 19 paket hijau (parser ketat, noop, dry-run determinisme + fairness + wall-time graph penuh, opsctx jujur, eval skoring, proxy disabled/enabled, rate limiter, handler 503/422) |
