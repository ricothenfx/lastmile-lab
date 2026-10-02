# PHASE 09 — Autonomous QA Suite + Halaman Interview Q&A

> Spesifikasi fase 9 (diminta pemilik 2026-10-02): (1) **pemeriksaan otonom
> semua fungsi & frontend** — satu command, dua mode target; (2) **halaman
> khusus tanya-jawab interviewer** — posisi sebagai penguji/reviewer: apa saja
> yang akan ditanyakan tentang aplikasi ini, lengkap dengan jawabannya.

## Prinsip yang tidak bisa ditawar

1. Halaman interview = **statis, tanpa dependensi backend** (filosofi
   anti-halaman-mati D9 berlaku juga untuk konten), 100% Inggris (DESIGN §7),
   token-only, kontras AA.
2. **Semua jawaban bersitasi** ke ADR (BLUEPRINT §5) atau angka terukur di
   `reports/` — tidak ada klaim tanpa bukti. Jawaban jujur soal trade-off &
   kelemahan (penguji selalu menggali kelemahan; kejujuran = nilai jual).
3. Suite QA = **skrip Playwright headless** (pola fase 2–8; docker image
   `mcr.microsoft.com/playwright`), dua mode: `local` (stack compose, boleh
   mutasi + restore) dan `prod` (read-only + surge probe dengan restore).
4. Verifikasi tidak boleh menghentikan produksi: prod mode = tidak ada kill,
   tidak ada duel, tidak ada demo play.

## Scope (kerjakan ini saja)

### A. Halaman `/interview` (route baru, static)

- Data: `apps/web/src/lib/interview.ts` — ≥25 Q&A dalam 8 kategori:
  1. Product & architecture (kenapa dibangun, kenapa Go, kenapa monorepo,
     kenapa Berlin & data OSM)
  2. Kafka & pipeline (kenapa Kafka/Redpanda vs RabbitMQ/Redis Streams/SQS;
     at-least-once vs exactly-once; idempotency 3 lapis; partisi; kenapa
     injeksi assignment via HTTP bukan consumer kedua — D15/D2)
  3. Dispatch & algorithms (kenapa core deterministik; FIFO vs batching vs
     zone vs Hungarian; kenapa bukan OR-Tools — D17; p99 2,6–13,1 ms; kapan
     optimal worth it — pelajaran "strategi berbayar hanya di bawah tekanan")
  4. Frontend craft (canvas 2D vs SVG/WebGL; 60fps + rAF on-demand; MapLibre
     self-hosted tanpa API key — D3/D12; replay mode D9/D14; reduced-motion;
     cerita bug z-index peta — war story engineering)
  5. Reliability & SRE (chaos design D19 — kenapa SIGTERM PID 1; MTTD/MTTR
     0,8–1,3 s / 2,0–3,0 s; zero loss 5.460=5.460; SLO & error budget)
  6. Ops & cost (RAM ≤2 GB; kenapa TANPA Prometheus/Grafana — D16; pipeline
     build GHCR → VPS pull — D7; split Vercel/VPS — D6)
  7. Security (kenapa endpoint mutasi publik tanpa auth — D23: risiko,
     mitigasi mekanis, jalur naik bearer token)
  8. AI copilot & scale (simulator-as-judge D24; tanpa key = tersembunyi;
     "apa yang pecah di 10.000 rider" — jawaban jujur: p99 optimal 303 ms
     @300×300, single VPS, RF=1, dsb.)
- UI: route `app/interview/page.tsx` (server component, `<details>/<summary>`
  — nol JS klien), link "INTERVIEW" di TopBar + rujukan di HelpOverlay.
- Setiap jawaban: ringkas (3–6 kalimat), angka nyata, dan label "evidence"
  yang menunjuk sumber (ADR # / reports/…).

### B. Suite QA terpadu — `scripts/verify-all.mjs`

- Dua mode: `--target=local|prod` (default prod). Local = jalankan penuh
  termasuk mutasi (surge → restore, kill chaos → pulih, duel lab, demo play
  → stop). Prod = subset read-only + surge probe (restore).
- Suites: smoke (healthz, live/replay), map (dots canvas probe, label,
  hover/click — warisan fase 8), kpi (angka non-null live / "—" offline),
  surge (<1 s echo, local), lab (duel berjalan, local), health (grid + chaos
  tab), replay (panel + inspect + fixture offline), copilot (0 node tanpa
  key), interview (halaman render + hitung Q&A), a11y (aria + reduced-motion),
  perf (rAF idle + console 0).
- Output: tabel pass/fail per check + screenshot ke `reports/verify-<ts>/`
  + exit code non-zero bila ada gagal. Bug historis wajib jadi check:
  z-index overlay peta (sesi 13), tinggi kontainer peta (fase 5), multi-member
  gzip replay (fase 5 — unit test backend, dirujuk saja), font 404 casing
  (fase 8), label `has` filter (fase 8).

## Di luar scope

CI job baru (build time VPS), perubahan backend, auth, i18n.

## DoD (kriteria pemblokir)

- [ ] `/interview` hidup di produksi (Vercel), ≥25 Q&A / 8 kategori, setiap
      jawaban bersitasi ADR/angka, nol console error, statis (tampak saat
      backend dimatikan), link dari TopBar.
- [ ] `node scripts/verify-all.mjs --target=prod` → ALL PASSED satu command;
      `--target=local` → ALL PASSED terhadap stack compose (mutasi dipulihkan).
- [ ] Screenshot bukti di `reports/verify-*/` + laporan `reports/phase-09-qa.md`.
- [ ] Docs: PROGRESS + ROADMAP (fase 9 ✅) — satu commit.
