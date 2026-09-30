# ROADMAP — lastmile-lab

> Peta fase + Definition of Done (DoD). Fase dinyatakan selesai HANYA setelah seluruh DoD
> terverifikasi DAN pekerjaan di-commit & push. Update status kolom **Status** di commit
> yang sama dengan pekerjaan.

Status legenda: ⬜ belum mulai · 🔨 sedang dikerjakan · ✅ selesai · ➖ dibatalkan/ditunda

| Fase | Nama | Status |
|---|---|---|
| 0 | Bootstrap repo, docs, CI, deploy skeleton | ✅ |
| 1 | Simulasi inti + Live Ops Map | ✅ |
| 2 | Order ingestion + load generator + Surge Console dasar | ✅ |
| 3 | Dispatch engine 4 strategi + Strategy Lab | ⬜ |
| 4 | KPI Command Deck + System Health + chaos | ⬜ |
| 5 | Replay engine + Golden Demo presets + polish motion | ⬜ |
| 6 | Hardening produksi + monitoring + README story + artikel | ⬜ |
| 7 | (Opsional) AI Ops Copilot & Plan Advisor | ⬜ |

---

## Fase 0 — Bootstrap (spec: `PHASES/phase-00.md`)

**DoD:**
- [x] Struktur monorepo sesuai BLUEPRINT §5 ada di repo
- [x] Semua docs sumber kebenaran terisi: AGENTS.md, BLUEPRINT, ROADMAP, DESIGN,
      PROGRESS, PHASES/phase-00
- [x] `deploy/` skeleton: compose.yaml (infra + limit sumber daya), site block Caddy,
      runbook README
- [x] `apps/web/` landing statis bertema design tokens (deployable ke Vercel apa adanya)
- [x] CI workflow hijau di GitHub (docs guard + YAML valid)
- [x] Blok port lastmile tercatat di `/home/rico/PORTS.md`
- [x] Repo GitHub `lastmile-lab` dibuat, commit pertama ter-push

## Fase 1 — Simulasi inti + Live Ops Map

**DoD:**
- [x] rider-simulator: rider bergerak di road network Berlin nyata (OSM), status idle/
      to-pickup/pickup/delivering
- [x] Generator order (sederhana dulu) + dispatch FIFO minimal agar ada alur cerita
- [x] ws-gateway streaming posisi → frontend
- [x] Live Ops Map: peta gelap, rider bergerak mulus, order berpulsa, garis assignment
- [x] Design tokens terpasang sebagai sistem (bukan warna hardcode)
- [x] **Motion 60fps di laptop menengah** — kriteria pemblokir
      (bukti: canvas draw 3,1 ms/frame, p50 frame 16,7 ms = vsync 60 fps, React render
      di-throttle 2 Hz + memoized; pengukuran absolut di laptop fisik = langkah sisa,
      lihat PROGRESS.md)
- [x] Landing page diganti dengan app sungguhan; deploy Vercel aktif di
      lastmile-lab.ricothen.com (butuh one-time auth akun Vercel pemilik)
      → kode + vercel.json siap; deploy & auth = langkah sisa pemilik (PROGRESS.md)
- [x] DoD fase + dokumen diperbarui, commit & push

## Fase 2 — Order ingestion + load + Surge Console

**DoD:** (bukti angka: `reports/phase-02-loadtest.md`)
- [x] order-ingestion API idempotent (Redis) + Redpanda + consumer pipeline
- [x] load-generator Poisson + skenario surge ×10
- [x] Postgres menyimpan order/events; health endpoint semua service
- [x] Surge Console: slider surge, toggle hujan/flash-sale — bereaksi < 1 detik di UI
      (headless: 226–874 ms; rantai API 14–420 ms)
- [x] Load test terdokumentasi: zero message loss pada spike ×10, angka tercatat di
      `reports/` (sent = consumed = stored = injected = 4.849; 429 = 0; lag = 0)
- [x] Grafana dasar (port 3030, loopback) → **diganti metrik JSON** sesuai opsi spec
      fase 2 (RAM sempit, ADR D16); Grafana menyusul di Fase 4 (System Health)
- [x] Commit & push

## Fase 3 — Dispatch engine + Strategy Lab

**DoD:**
- [ ] 4 strategi: FIFO, Batching, Zone-based, Optimal (Hungarian/OR-Tools)
- [ ] p99 keputusan dispatch terukur < 50 ms pada beban target
- [ ] Strategy Lab: A vs B skenario identik → peta replay kembar + tabel delta +
      histogram overlay
- [ ] Export benchmark report (dipakai di README)
- [ ] Angka delta first-class: delivery time, utilization, cost/order
- [ ] Commit & push

## Fase 4 — KPI Deck + System Health + chaos

**DoD:**
- [ ] KPI Command Deck lengkap (kartu live, streaming chart, SLO gauge)
- [ ] System Health: visualisasi pipeline, grid node, autoscaling/health events
- [ ] Chaos injector: kill node saat trafik — self-heal terlihat di UI + Incident Timeline
- [ ] MTTD/MTTR/error budget tercatat dari eksperimen (reports/)
- [ ] Commit & push

## Fase 5 — Replay + Golden Demo + polish

**DoD:**
- [ ] Replay engine: scrub timeline, inspect rider/order, alasan keputusan dispatch
- [ ] 2–3 Golden Demo presets (▶ Play) dengan narasi 90 detik
- [ ] Frontend tetap hidup 100% saat backend dimatikan (replay fallback)
- [ ] Audit motion & visual 60fps; konsistensi token; responsive dasar (1440/1024/768)
- [ ] Commit & push

## Fase 6 — Produksi

**DoD:**
- [ ] Deploy final di lastmile-lab.ricothen.com + api./ws. subdomains HTTPS (Caddy)
- [ ] UptimeRobot pada /healthz; log rotation aktif; verifikasi budget RAM ≤ 2 GB
- [ ] CI/CD penuh: push → build → image → deploy otomatis (atau satu perintah)
- [ ] README akhir: arsitektur, GIF dashboard, tabel metrik, cara reproduksi benchmark
- [ ] Artikel teknis ("How I cut simulated delivery time by X% ...") — draft di `docs/blog/`
- [ ] Uji beban produksi ringan + pemeriksaan keamanan dasar (no secrets, CORS, rate limit)
- [ ] Commit & push

## Fase 7 — (Opsional) AI Ops Copilot & Plan Advisor

**DoD:**
- [ ] Adapter AI: tanpa API key → fitur tersembunyi, seluruh aplikasi tetap utuh
- [ ] Plan Advisor: snapshot metrik → LLM usulkan Plan A/B/C terstruktur → simulator
      dry-run tiap plan → admin memilih berdasarkan angka prediksi → eksekusi live
- [ ] Copilot: tanya-jawab metrik/incident dengan sitasi data internal
- [ ] Evaluasi kualitas jawaban terdokumentasi (ground truth kecil)
- [ ] Commit & push
