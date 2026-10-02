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
| 3 | Dispatch engine 4 strategi + Strategy Lab | ✅ |
| 4 | KPI Command Deck + System Health + chaos | ✅ |
| 5 | Replay engine + Golden Demo presets + polish motion | ✅ |
| 6 | Hardening produksi + monitoring + README story + artikel | ✅ |
| 7 | (Opsional) AI Ops Copilot & Plan Advisor | ✅ |
| 8 | Map craft & map interactivity (nama jalan, heatmap, hover/click) | ✅ |
| 9 | Autonomous QA suite + halaman Interview Q&A | ✅ |

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

**DoD:** (bukti angka: `reports/phase-03-bench.md` + `reports/phase-03-ui-*.png`)
- [x] 4 strategi: FIFO, Batching, Zone-based, Optimal (Hungarian murni Go — ADR D17;
      semua deterministik + reason; unit test urutan/keunikan/brute-force JV)
- [x] p99 keputusan dispatch < 50 ms pada beban target — terukur **2,6–13,1 ms**
      @ 100 order + 100 rider, cap 1 CPU, host load 9+ (semua strategi)
- [x] Strategy Lab: A vs B skenario identik (SATU generator seed sama → dua engine)
      → peta replay kembar + tabel delta + histogram overlay bin bersama
- [x] Export benchmark report (file di `reports/`, format dipakai README nanti;
      UI juga punya EXPORT JSON per duel)
- [x] Angka delta first-class: delivery time p50/p95, utilization, cost/order
      (definisi eksplisit di laporan §1) — mis. rush 600 s: optimal delivered +98%,
      cost/order −54% vs FIFO
- [x] Kualitas UI: token-only, mono tabular, kontras AA, reduced-motion, canvas
      on-demand tanpa rAF idle (bukti headless `phase-03-ui-verify.mjs`, error 0)
- [x] Semua service/endpoint baru punya health + limit RAM (strategy-lab 256 MiB;
      ram stack 1.792 MiB ≤ 2 GB) + CI hijau (ci + images)
- [x] Commit & push

## Fase 4 — KPI Deck + System Health + chaos

**DoD:** (bukti angka: `reports/phase-04-chaos.md` + `reports/phase04-*.png`)
- [x] KPI Command Deck: kartu live + streaming chart + SLO gauge — angka dari
      metrik nyata (ring engine + counters pipeline + incident chaos; bukan
      dekorasi), mono tabular, fallback "—" tanpa backend
- [x] System Health: pipeline visual (partikel order→Kafka→consumer→UI) +
      grid node (healthz polling, cache 2 s) + health events
- [x] Chaos injector: kill node saat trafik → self-heal terlihat di UI +
      Incident Timeline; allowlist eksplisit 7 container stateless
      `lastmile-*` (sisanya 403; infra ber-state di luar allowlist)
- [x] MTTD/MTTR/error budget tercatat dari eksperimen nyata: 4 kill
      (MTTD 0,81–1,30 s · MTTR 1,98–3,02 s), E4 zero-loss 5 460 = 5 460
      saat consumer di-kill @300/menit spike ×10
- [x] Kualitas UI: token-only, kontras AA, reduced-motion penuh, canvas
      on-demand (chart on-data, partikel rAF hanya saat panel terbuka —
      bukti `phase-04-ui-verify.mjs`, rAF audit + console 0)
- [x] Semua service/endpoint baru punya health + limit RAM (chaos 64 MiB;
      ram stack 1.856 MiB ≤ 2 GB) + CI hijau (ci + images, 9 image)
- [x] Commit & push

## Fase 5 — Replay + Golden Demo + polish

**DoD:**
- [x] Replay engine: scrub timeline, inspect rider/order, alasan keputusan dispatch
- [x] 2–3 Golden Demo presets (▶ Play) dengan narasi 90 detik
- [x] Frontend tetap hidup 100% saat backend dimatikan (replay fallback)
- [x] Audit motion & visual 60fps; konsistensi token; responsive dasar (1440/1024/768)
- [x] Commit & push

> **SELESAI 2026-10-01.** Ring replay 15 menit @ 5 Hz (9,0 MiB gzip terukur, RSS
> rider-sim 46 MiB / limit 160 MiB — stack 1 888 MiB ≤ 2 GB); scrub akurasi ≤ 0,2 s;
> inspect rider dengan alasan dispatch dari ring decisions; 3 preset Golden Demo ±90 s
> (eksekusi end-to-end di UI headless: 7/7 langkah, kill rider-sim → incident chaos-kill
> MTTD 642 ms, app pulih otomatis); fixture fase 1 tidak regresi (uji backend mati).
> Bukti: `reports/phase-05-replay.md`.

## Fase 6 — Produksi

**DoD:**
- [x] Deploy final di lastmile-lab.ricothen.com + api./ws. subdomains HTTPS (Caddy)
- [x] UptimeRobot pada /healthz; log rotation aktif; verifikasi budget RAM ≤ 2 GB
- [x] CI/CD penuh: push → build → image → deploy otomatis (atau satu perintah)
- [x] README akhir: arsitektur, GIF dashboard, tabel metrik, cara reproduksi benchmark
- [x] Artikel teknis ("How I cut simulated delivery time by X% ...") — draft di `docs/blog/`
- [x] Uji beban produksi ringan + pemeriksaan keamanan dasar (no secrets, CORS, rate limit)
- [x] Commit & push

> **SELESAI 2026-10-01 (frontend live 2026-10-02).** api.+ws. HTTPS hidup dari
> internet (healthz 200 via domain, WS upgrade TLS terverifikasi end-to-end
> headless); DNS aktif oleh pemilik; **frontend Vercel LIVE di
> lastmile-lab.ricothen.com** (project `lastmile-lab`, GitHub-integrated
> auto-deploy, env produksi terpasang, Deployment Protection OFF, verifikasi
> headless: LIVE + fallback REPLAY — `reports/phase06-vercel-*.png`). CI/CD: job
> `deploy` (skip-warning tanpa secret) + `scripts/deploy.sh` — siklus nyata
> push→build→image→pull+up+healthz di commit fase 6. UptimeRobot: setup di
> runbook §5.5 (aktivasi pemilik); log rotation live 10m×3. RAM: 1 888 MiB
> limit ≤ 2 GB (demo aktual ≈ 80 MiB). Load test 5 menit: zero loss
> (published=consumed=acked 4 261, dup 0, DB delta = sent). Keamanan: ADR D23
> (mutasi publik tanpa auth — risiko + mitigasi + jalur naik). Replay fallback
> terverifikasi di domain produksi (blokir + stop backend sungguhan → REPLAY
> MODE → pulih LIVE). Bukti: `reports/phase-06-prod.md` + `phase06-*.png`.

## Fase 7 — (Opsional) AI Ops Copilot & Plan Advisor

**DoD:**
- [x] Adapter AI: tanpa API key → fitur tersembunyi, seluruh aplikasi tetap utuh
- [x] Plan Advisor: snapshot metrik → LLM usulkan Plan A/B/C terstruktur → simulator
      dry-run tiap plan → admin memilih berdasarkan angka prediksi → eksekusi live
- [x] Copilot: tanya-jawab metrik/incident dengan sitasi data internal
- [x] Evaluasi kualitas jawaban terdokumentasi (ground truth kecil)
- [x] Commit & push

> **SELESAI 2026-10-01 (opsional-done; catatan evaluasi live di bawah).** Service
> `copilot` :4207 profile `copilot` OFF-by-default (internal saja, 128 MiB); tanpa
> key = noop provider (ErrNoLLM) → capabilities `{"enabled":false}` → **0 node
> copilot di DOM**, regresi headless 6 layar + Golden Demo 7/7 + replay PASSED,
> console 0 (`reports/phase-07-ui-verify.mjs`). Advisor: ≤3 plan schema ketat →
> dry-run via jalur duel (seed sama, 120 s virtual ≈ 0,2 s wall di graph Berlin
> penuh) → eksekusi manual konfirmasi 2 langkah via endpoint kontrol existing.
> Ask Ops: jawaban tanpa sitasi ditolak (422). ADR D24; laporan +
> **evaluasi live 15 kasus: menunggu API key pemilik** (kode + skoring teruji —
> `reports/phase-07-copilot.md` §6). Ram stack demo tetap 1 888 MiB ≤ 2 GB.

## Fase 8 — Map craft & map interactivity

**DoD:** (bukti: `reports/phase-08-map.md` + `reports/phase08-*.png`)
- [x] Nama jalan OSM di peta (glyph self-hosted, tanpa tile provider — ADR D12)
- [x] Glow jalan major saat zoom; heatmap zona bernapas mengikuti surge (D25)
- [x] Delivery burst / expiry fade (animasi bermakna data)
- [x] Hover + klik inspect dot di mode live (tooltip < 16 ms hit-test)
- [x] Verifikasi headless ALL PASSED (23 check, console 0, nol rAF baru)

> **SELESAI 2026-10-02.** Nama jalan via replikasi simplifikasi graphgen atas
> Overpass (graph routing tidak disentuh); glyph Inter PBF ±355 KB self-hosted;
> 31 label ter-render di z15 ("Waisenstraße"); alphaSum heatmap 2,2× pada
> surge ×4 dan dipulihkan ×1; hover hit-test 0,1–0,2 ms; kartu inspect live +
> Escape; replay inspect tidak regresi; reduced-motion penuh.

## Fase 9 — Autonomous QA suite + halaman Interview Q&A

**DoD:** (spec: `PHASES/phase-09.md`; bukti: `reports/phase-09-qa.md` +
`reports/verify-local/`)
- [x] Halaman `/interview` — 36 Q&A / 8 kategori, 36/36 bersitasi ADR/angka
- [x] Suite e2e terpadu `scripts/verify-all.mjs` (local penuh + prod subset)
- [x] Bug historis jadi tes permanen; console 0; satu command — commit & push

> **SELESAI 2026-10-02.** verify-all local: 33 PASS / 0 FAIL (chaos kill →
> self-heal 3,9 s; duel lab done; demo play/stop; offline fallback; a11y;
> console 0). Catatan: Vercel checkpoint 403 dari IP VPS — verifikasi prod
> memakai build produksi identik; runbook §6 diperbarui.
