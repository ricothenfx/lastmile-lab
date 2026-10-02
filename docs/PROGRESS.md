# PROGRESS — lastmile-lab

> Log status kerja. **Sesi AI/developer baru: baca file ini PERTAMA setelah AGENTS.md.**
> Update setiap selesai pekerjaan berarti, di commit yang sama dengan kodenya.

## Status Saat Ini

- **Fase aktif:** TIDAK ADA — **fase 0–9 SELESAI** (fase 9: Autonomous QA
  suite `scripts/verify-all.mjs` + halaman `/interview` 36 Q&A bersitasi —
  `reports/phase-09-qa.md`; 33 PASS / 0 FAIL mode local).
- **Kondisi:** **FASE 9 SELESAI 2026-10-02** (sesi 14): satu command
  memeriksa 16 suite (map/interact/heat/bursts/kpi/replay/offline/interview/
  copilot/a11y/perf/console + chaos/lab/demo khusus local) — 33 PASS / 0
  FAIL, mutasi dipulihkan (surge ×1, demo stop, strategy-lab self-heal
  3,9 s). Halaman `/interview` = 36 Q&A / 8 kategori, 36/36 sitasi ADR /
  angka terukur, statis tanpa backend, link INTERVIEW di TopBar. **Catatan
  lingkungan:** Vercel Security Checkpoint mulai 403 untuk IP VPS ini
  (browser manusia & UptimeRobot tetap lolos) — verifikasi prod memakai
  build produksi identik; runbook §6 diperbarui. Fase 8 (peta: nama jalan
  OSM, glow, heatmap D25, burst, hover/klik inspector — 23 check ALL
  PASSED) tetap utuh, detail di log di bawah.
- **Langkah berikutnya:** verifikasi pasca-deploy `/interview` di produksi
  dari IP non-VPS (lihat `reports/phase-09-qa.md` §4); sisa aksi pemilik:
  monitor keyword UptimeRobot, verifikasi 60fps laptop fisik.
- **Blokir/tergantung user:** none untuk koding. **GO-LIVE PRODUKSI TUNTAS**
  (backend + frontend LIVE publik).

## Langkah Sisa Go-Live (butuh akses pemilik — bukan blokir fase)

1. ~~**Vercel**: import repo + env + assign domain~~ — **✅ TUNTAS 2026-10-02**
   (project Vercel `lastmile-lab`, GitHub-integrated auto-deploy, env produksi
   terpasang, Deployment Protection OFF, domain `lastmile-lab.ricothen.com`
   200 publik — bukti `reports/phase-06-prod.md` §1.1 + `phase06-vercel-*.png`).
2. ~~**Secret GitHub** (Settings → Secrets → Actions): `DEPLOY_SSH_KEY`
   (keypair khusus deploy, publik ke authorized_keys VPS), `DEPLOY_SSH_HOST`,
   `DEPLOY_SSH_USER`~~ — **✅ TUNTAS 2026-10-02**: 3 secret terpasang + fix
   jalur SSH (`DEPLOY_SSH_KEY_FILE`) → job `deploy` CI SUKSES end-to-end
   (run `36956804558`: 4 langkah deploy.sh + "Deploy selesai"; log sesi 10).
   Auto-deploy kini aktif tiap push `apps/services/**`.
3. **UptimeRobot** — 🟡 SEBAGIAN (2026-10-02): akun aktif + monitor frontend
   `lastmile-lab.ricothen.com` UP (ID 804154229, dibuat manual). Monitor
   keyword `lastmile-api-healthz` tinggal dibuat manual di dashboard (±2
   menit, resep runbook §5.5) — **create via API = fitur berbayar** (bukti
   di runbook; key Main valid untuk read/edit).
4. **Verifikasi 60fps di laptop fisik** — **DITUNDA atas keputusan pemilik
   (2026-10-02)**; kriteria DoD fase 1 terpenuhi struktural (rAF tunggal per
   canvas, render on-demand, tanpa animasi idle — fase 1/3). Prosedur tetap
   tersedia di bawah bila nanti dijalankan: buka https://lastmile-lab
   .ricothen.com → DevTools Performance → CPU 4× throttle → rekam 15 s →
   harapkan p50 frame ≤ 16,7 ms. Fase 3 menambah dua canvas lab kecil yang
   render ON-DEMAND (tanpa rAF saat idle) — budget idle tidak berubah;
   playback lab hanya rAF saat PLAY ditekan.

## Log

### 2026-10-02 — Fase 9: Autonomous QA suite + halaman Interview (sesi 14)

- **`/interview`** (route statis): 36 Q&A / 8 kategori — posisi penguji:
  kenapa Go/Kafka/Redpanda/bukan RabbitMQ, at-least-once & dedupe 3 lapis,
  kenapa Hungarian manual bukan OR-Tools (p99 13,1 ms @100×100; rush +98%
  delivered, −54% cost/order), kapan optimal TIDAK worth it (303 ms
  @300×300), canvas 2D vs SVG/WebGL, replay fallback, war story z-index,
  chaos SIGTERM PID 1 (MTTR 2,0–3,0 s), zero loss 5.460=5.460, kenapa TANPA
  Prometheus/Grafana, endpoint mutasi publik D23, simulator-as-judge D24,
  "apa yang pecah di 10.000 rider" — 36/36 jawaban bersitasi ADR/angka.
  Server component + `<details>` (nol JS), token-only, AA, link INTERVIEW
  di TopBar.
- **`scripts/verify-all.mjs`**: 16 suite / 2 mode (--target=local|prod,
  --only filter). Local FULL: **33 PASS / 0 FAIL** — chaos kill strategy-lab
  (202 → self-heal 3,9 s), duel lab fifo vs optimal (status done), Golden
  Demo play→FLASH SALE→stop, offline fallback, a11y reduced-motion, rAF
  idle 39/2 s, console 0. Bug historis jadi check permanen (z-index peta,
  label `has`/casing glyph, innerText uppercase, dsb). Artifacts:
  `reports/verify-local/` (summary.json + 9 screenshot), laporan:
  `reports/phase-09-qa.md`.
- **Perbaikan skrip selama verifikasi**: parser arg `--k=v`, API default
  via domain publik (stack tidak dipublish ke loopback — sesuai arsitektur),
  urutan lab→chaos, gate kebenaran berbasis hasil akhir (self-heal /
  status done), diagnostik hydrate untuk host load 8+.
- **Catatan lingkungan**: Vercel Security Checkpoint 403 untuk IP VPS
  (curl + headless; manusia & monitor eksternal lolos) → verifikasi prod
  memakai build produksi identik; runbook §6 diperbarui.

### 2026-10-02 — Fase 8: Map craft & map interactivity (sesi 14)

Permintaan pemilik: peta tanpa nama jalan & kurang menarik; dot driver tidak
bisa di-hover/klik. Empat kelompok kerja, semuanya client-side (backend &
kontrak snapshot tidak berubah):

- **Nama jalan**: `scripts/add-road-names.mjs` mereplikasi logika
  simplifikasi graphgen (walk chain node derajat-2) atas data Overpass
  mentah (4 kuadran via mirror maps.mail.ru — overpass-api.de 406/kumi 429
  dari VPS), mayoritas nama segmen per chain → properti `n` di
  `roads.geojson` (2,3→2,5 MB; match 52,3% — sisanya service road tak
  bernama; 98,7% jalan major bernama; Friedrichstraße/Torstraße/Unter den
  Linden ✓). **Graph routing byte-identical.** Glyph PBF "Inter Regular"
  ±355 KB via fontnik (variable font → default instance 400; äöüß ✓) di
  `public/fonts/Inter/` — nol font/tile provider (ADR D12 utuh).
- **Peta hidup**: layer `roads-glow` (janji "glow halus saat zoom" DESIGN.md
  terealisasi), 2 symbol layer label (major z13 / minor z14, halo, fade-in);
  token baru `mapRoadGlow`/`mapLabel`/`mapLabelMajor` (AA 7,4× / 12,7×);
  heatmap zona bernapas = densitas order per ±500 m mengikuti `st.su`
  (**ADR D25**; alphaSum 0,64→1,39 = 2,2× saat surge ×4, dipulihkan ×1);
  delivery burst violet/cyan + expiry fade coral dengan guard scrub.
- **Interaktivitas**: hover dot → tooltip (rider: status, order dibawa,
  jarak haversine; order: rider, rute, umur) — hit-test 0,1–0,2 ms pada
  `pickData` per-frame yang sudah ada, tanpa re-render per gerakan mouse;
  klik di mode live → kartu inspect (1 Hz refresh, Escape/klik kosong
  menutup); klik replay → kartu reason fase 5 tidak regresi. `zone.hot`
  DESIGN.md kini terimplementasi.
- **Verifikasi**: `reports/phase-08-ui-verify.mjs` ALL PASSED — 23 check
  termasuk probe `queryRenderedFeatures` (31 label, "Waisenstraße"), font
  request 100% lokal, audit rAF idle 28/2 s (nol loop baru), burst counter,
  reduced-motion, responsive 1024/768, console **0**. Laporan + 8 screenshot:
  `reports/phase-08-map.md`, `reports/phase08-*.png`. Dijalankan terhadap
  build produksi ber-env `wss://ws.`+`https://api.` (pola fase 6) + backend
  produksi nyata; deploy Vercel mengikuti push.
- Bug yang ditangkap verifikasi (pelajaran permanen di laporan §3):
  filter `['has', ['get','n']]` salah (harus `['has','n']`), casing
  direktori glyph (`/fonts/Inter/` vs `inter/`) = 404, dan `next start`
  cache daftar public saat boot.

### 2026-10-02 — Fix peta kosong + UI 100% Inggris + panduan in-app (sesi 13)

Laporan pemilik (user pertama): peta polos tanpa aktivitas, masih ada teks
Indonesia, dan pengguna baru bingung cara memakai dashboard. Tiga perbaikan:

- **Bug peta kosong — AKAR MASALAH DITEMUKAN**: dot rider/order digambar di
  canvas overlay 2D, tapi maplibre meng-append canvas WebGL-nya SETELAH
  overlay (sama-sama `position:absolute`, tanpa z-index) → peta menutupi
  seluruh entitas. Bukti diagnosis: buffer canvas overlay berisi puluhan ribu
  piksel entitas (getImageData probe) sementara screenshot produksi menampilkan
  nol dot; WS sehat (49 frame/6 s, 100 riders), snapshot lengkap (r/o/l/st),
  proyeksi koordinat benar, console 0 error. Fix: `z-index:1` eksplisit pada
  overlay (`LiveMap.tsx`) + komentar penjelasan. Bug ini ada sejak fase 1 —
  screenshot lama (fase 5/6/7) ikut membuktikan dot tak pernah terlihat.
- **UI 100% bahasa Inggris**: sweep seluruh copy user-facing — komponen web
  (label, aria-label, placeholder, pesan error/empty state), teks Golden Demo
  dari backend (`api-gateway/demo.go`: nama/desc/step label), dan reason
  keputusan dispatch dari backend (`fifo/zone/batching/pipeline` — tampil di
  kartu inspect replay). Aturan bahasa ditulis di `docs/DESIGN.md` §7.
- **Panduan penggunaan in-app**: `HelpOverlay` baru — auto-buka sekali untuk
  pengunjung pertama (localStorage), tombol **? GUIDE** di TopBar, tutup via
  ESC/klik luar; 8 seksi menjelaskan peta+warna dot, KPI deck, surge console,
  strategy lab, system health, demo & replay. Ditambah tooltip `title` pada
  kontrol kunci (slider surge, RAIN/FLASH, DEMO, REPLAY, toggle panel, peta).
- **Verifikasi**: `tsc --noEmit` ✅, `next build` ✅ (docker node:22-alpine —
  node_modules memuat SWC musl; catatan: build web di VPS harus via alpine,
  bukan bookworm), `go test` dispatch+model+api-gateway ✅. Deploy: frontend
  otomatis via Vercel; backend images otomatis via CI (paths filter kena:
  demo.go + dispatch + engine berubah).
- **Hasil produksi (bukti `reports/phase13-map-dots-live.png` +
  `phase13-guide-firstrun.png`)**: peta penuh dot rider + garis assignment
  (kali pertama sejak fase 1!), guide auto-terbuka, 0 leftover Indonesia
  (probe innerText), 0 console error. Deploy backend pertama ditolak load
  guard (9,66 > 8 — perilaku benar, sesi desktop x2go sedang berat), rerun
  sukses saat load 7,3: demo preset produksi kini Inggris
  ("Dinner Rush in Berlin…"), healthz 200.

### 2026-10-02 — UptimeRobot sebagian aktif; 60fps ditunda pemilik (sesi 12)

- Akun UptimeRobot aktif (Main API key via `/tmp/kilo/ur_key`, perm 600, di
  luar git). Ternyata monitor frontend **sudah dibuat pemilik di dashboard**:
  `lastmile-lab.ricothen.com` (HTTP, 5 menit, timeout 30, status UP,
  ID 804154229) — `getMonitors` terverifikasi.
- Temuan: **create monitor via API = fitur berbayar**. Bukti: `newMonitor`
  minimal (`type=1` + url + nama, tanpa param lain) → `{"stat":"fail",
  "access_denied":"not allowed to use some settings with your current plan"}`,
  sementara `getAccountDetails`/`getMonitors`/`editMonitor` (no-op rename) →
  ok. Kesimpulan resep runbook §5.5 direvisi: monitor keyword healthz dibuat
  manual di dashboard (±2 menit), lalu diverifikasi via `getMonitors`.
- Verifikasi target monitor: `GET /healthz` api publik = 200 + body memuat
  keyword persis `"ok":true` (aman terhadap false-positive `lab_ok` — keyword
  diawali kutip ganda).
- **60fps laptop fisik: DITUNDA** atas keputusan pemilik; dicatat sebagai
  ditunda (bukan blokir) di Langkah Sisa + runbook.

### 2026-10-02 — AI Ops Copilot LIVE + evaluasi kualitas 15 kasus (sesi 11)

- **Copilot produksi LIVE**: key pemilik diisi `deploy/.env` (posisi dikoreksi
  sesi 10; file 600 + gitignored) → `pull` + `up -d` profile `copilot` →
  7/7 kontainer healthy (`lastmile-copilot` healthy, RAM aktual 6,4 MiB /
  limit 128 MiB) → `GET /api/copilot/capabilities` (api. publik) =
  `{"enabled":true}`. Budget RAM stack tetap sesi 7 (copilot sudah dihitung).
- **Panel UI terverifikasi di produksi** (Playwright headless, bukti
  `reports/phase07-copilot-live-advisor.png` + `-health.png`): buka panel
  Strategy Lab → tab ADVISOR muncul; System Health → tab COPILOT muncul;
  console error 0. (Catatan: tab HANYA dirender setelah panel dibuka —
  `useCopilotEnabled` dijalankan saat mount panel, cek awal tanpa buka panel
  wajar tidak melihatnya.)
- **Evaluasi live 15 kasus SELESAI** (prosedur §6 laporan fase 7 — runner
  `eval.Cases` → `POST /api/copilot/ask` publik → `ScoreAnswer`+`CitationsOK`,
  kode skoring identik unit test; model `gpt-4o-mini-2024-07-18`; pacing 11 s
  vs rate limit 6/menit; wall 3,2 menit): **metrik 5/5 benar + sitasi tepat**;
  diag surge/incident lemah — 6 benar / 1 parsial / 4 salah / **4 ditolak
  validator (422)** / 0 gagal; sitasi area tepat 10/11 jawaban tampil.
  Analisis jujur: konteks evaluasi = demo steady TANPA surge/incident aktif →
  sumber `incidents.summary` kosong → model mengarang sitasi → validator
  menolak (by design, tidak pernah tampil). Tindak lanjut opsional tercatat
  di laporan §6 (evaluasi ulang saat konteks aktif / enrich prompt).
- Runner evaluasi tidak masuk repo (one-shot di /tmp/kilo, pola sesi sebelumnya);
  skoring tetap satu sumber kebenaran di `internal/eval/groundtruth.go`.

### 2026-10-02 — Auto-deploy aktif: 3 secret GitHub + fix jalur SSH CI (sesi 10)

- **3 secret GitHub terpasang** (repo `ricothenfx/lastmile-lab`): `DEPLOY_SSH_KEY`
  (keypair ed25519 khusus deploy, `~/.ssh/lastmile_deploy` di VPS, pub ke
  `authorized_keys`, tes BatchMode loopback OK), `DEPLOY_SSH_HOST`
  (`194.233.67.201`), `DEPLOY_SSH_USER` (`rico`). Key privat tidak pernah
  menyentuh git/CI log.
- **Bug laten ditemukan & diperbaiki**: run `images` pertama pasca-secret —
  build 10/10 hijau tetapi job `deploy` GAGAL `Permission denied (publickey)`:
  workflow menulis key ke `~/.ssh/deploy_key` sementara `deploy.sh` memanggil
  `ssh` TANPA `-i` (runner tanpa identitas default; jalur SSH CI memang belum
  pernah teruji — job selalu skip sebelum secret ada). Fix: `deploy.sh` menerima
  `DEPLOY_SSH_KEY_FILE` opsional, workflow meng-export `$HOME/.ssh/deploy_key`.
  Terverifikasi dari VPS: `DEPLOY_SSH_HOST=rico@127.0.0.1 DEPLOY_SSH_KEY_FILE=…
  ./scripts/deploy.sh` → load 6.58<8 → pull → up -d → healthz 200 →
  "Deploy selesai" (6/6 kontainer healthy; image identik, tanpa recreate).
- **Runbook §4.4 + comment compose dikoreksi**: key copilot wajib ke
  `deploy/.env`, BUKAN root `.env` — project directory compose = `deploy/`;
  diverifikasi empiris via `docker compose config` (dummy probe: root `.env`
  → resolusi kosong; `deploy/.env` → terbaca; dummy dibersihkan setelahnya).
  `.gitignore` sudah mencakup kedua lokasi.
- **Verifikasi job `deploy` CI hingga hijau**: run pemicu fix (`36956681280`)
  GAGAL di **load guard** — loadavg 8.48 > 8, deploy ditunda (guard bekerja
  semestinya; SSH sudah lolos). Load turun → dispatch ulang → run
  **`36956804558` SUKSES**: loadavg 5.83 < 8 → pull → up -d → healthz 200 →
  "Deploy selesai" di log CI. VPS pasca-deploy: 6/6 kontainer healthy,
  `api./healthz` 200, frontend 200, `/api/replay/sessions` 200.
- **Regresi UI rantai publik PASSED** (`reports/phase-06-vercel-verify.mjs`
  MODE=live via image playwright 1.63): LIVE LINK via Vercel, KPI via api.
  publik 200, fallback REPLAY MODE saat api./ws. diblokir, console error 0.
- Bonus commit ikutan: `apps/web/.gitignore` dilengkapi `!.env.example`
  (sisa sesi 9 yang belum ter-commit; file sudah placeholder-only).

### 2026-10-02 — Go-live frontend Vercel (sesi 9)

- **Frontend produksi LIVE**: project Vercel `lastmile-lab` (team
  `ricothenfxs-projects`) dibuat via CLI v62 dari `apps/web` (device-flow login
  akun pemilik — satu-satunya langkah manual), **GitHub-integrated**: push
  `main` → auto-deploy production; jalur manual setara `vercel deploy --prod`.
- Env produksi scope Production: `NEXT_PUBLIC_WS_URL=wss://ws.…/ws`,
  `NEXT_PUBLIC_API_URL=https://api.…` (baked saat build); domain
  `lastmile-lab.ricothen.com` added+verified+auto-assigned → **200 publik
  0,7 s**. `apps/web/.gitignore` baru (`.vercel`, `.env*`, `!.env.example`)
  menjaga `.env.local` (VERCEL_OIDC_TOKEN lokal) tak pernah masuk git.
- **Deployment Protection dimatikan**: Vercel Authentication OFF via API
  (`ssoProtection: null`); Attack Challenge Mode OFF via dashboard (toggle
  dashboard-only — saat ON semua pengunjung termasuk browser asli terhalang
  "Vercel Security Checkpoint" 403 "Failed to verify your browser Code 21").
  Protected sourcemaps dibiarkan ON.
- **Verifikasi headless** `reports/phase-06-vercel-verify.mjs`
  (`phase06-vercel-live.png`, `phase06-vercel-replay.png`): LIVE LINK via
  domain frontend + KPI via api. publik 200 + blokir api./ws. → banner
  REPLAY MODE + console error 0. Rantai publik kini utuh:
  browser → Vercel → wss/https → Caddy → stack lastmile.
- Bonus: project Vercel sampingan `kilo` (efek samping `vercel curl` dari
  /tmp/kilo) langsung dihapus; tidak ada resource lain yang tertinggal.

### 2026-10-01 — Fase 7: AI Ops Copilot & Plan Advisor (sesi 8)

- **Service `copilot` :4207** (profile compose `copilot` OFF-by-default, jaringan
  internal saja, mem_limit 128 MiB, healthz): interface `LLM{Complete}` + **noop
  provider (`ErrNoLLM`) tanpa `OPENAI_API_KEY`** + OpenAI-compatible HTTP murni
  (`OPENAI_BASE_URL` bisa ganti provider, timeout ketat 8 s, tanpa SDK). Endpoint:
  `/capabilities` → `{"enabled":bool}`, `/advise` (≤3 plan schema JSON ketat, plan
  invalid DIBUANG dengan alasan), `/ask` (jawaban WAJIB sitasi `sources`; tanpa
  sitasi/id tak dikenal → 422 ditolak). Rate limit token bucket 6/menit/endpoint.
  Context internal dari endpoint existing (`/api/kpi`+`/api/metrics`+`/api/chaos/
  incidents` — TANPA akses DB); bagian fetch gagal dilaporkan `_missing`, jujur.
- **Dry-run = simulator, bukan LLM** (ADR D24): tiap plan lewat jalur duel existing
  `internal/duel` — baseline vs plan, SATU generator seed sama (fairness D18),
  120 s virtual ≈ **0,2 s wall di graph Berlin penuh** (unit test). Mapping:
  strategy→strategi sisi plan; surge→skala laju generator; weather→WeatherFactor;
  **kill tidak bisa disimulasikan → plan tanpa angka palsu + Note jujur**. Core
  `internal/sim` + `pkg/dispatch` TIDAK disentuh (determinisme tetap diuji).
- **Proxy api-gateway** (`copilot.go` + test): tanpa `COPILOT_URL` → capabilities
  tetap `{"enabled":false}` (200), lainnya 503 `copilot_disabled`; dengan URL →
  passthrough (client 45 s untuk LLM+dry-run).
- **UI**: tab ADVISOR (Strategy Lab) + COPILOT (System Health) HANYA ada di DOM
  saat enabled — tanpa key DOM persis baseline (0 node; tab bar tidak dirender).
  Advisor: tabel BASE|PLAN|Δ mono tabular + EXECUTE per plan **konfirmasi 2 langkah**
  → endpoint kontrol existing (surge/weather via sim-control, kill via chaos;
  strategy jujur "SKIP — butuh restart env"). Ask Ops: jawaban + daftar sitasi
  (id + label + value). Jalur enabled diuji **tanpa API key** via route-stub
  Playwright (advisor render, dry-run delta, kill-note jujur, execute 2 langkah,
  ask sitasi, reject 422) — console 0.
- **Regresi no-key PASSED** (`reports/phase-07-ui-verify.mjs` MODE hidden): 6 layar
  + Golden Demo dinner-rush **7/7 langkah** (kill rider-sim → pulih) + replay SESI
  LIVE + **0 node copilot** + console error 0. Bukti: `reports/phase07-*.png`,
  laporan `reports/phase-07-copilot.md`.
- **Bug replay ditemukan & diperbaiki**: `/api/replay/*` di api-gateway memakai
  client 4 s — dump ring penuh ±21 MB gzip butuh >4 s di host berbeban → stream
  terpotong ("Unterminated string at ~50 MB" di browser → jatuh fixture). Fix:
  client khusus 30 s; SESI LIVE terverifikasi ulang.
- **Infra/CI**: image GHCR +1 (`lastmile-copilot`, total 10); PORTS.md +4207
  (container-only); runbook §4.4 (aktivasi = aksi pemilik via `.env`, TIDAK
  menset secret apa pun di sesi ini); RAM stack demo tetap 1 888 MiB ≤ 2 GB.
- **Evaluasi kualitas**: ground truth 15 kasus (5 surge/5 incident/5 metrik) +
  skoring benar/parsial/salah + cek area sitasi — unit test hijau; **eksekusi
  live MENUNGGU API KEY PEMILIK** (prosedur ±10 menit di laporan §6).
- Verifikasi lokal sesuai batas keras: build/test via docker golang:1.26-alpine
  (cache lastmile-gomod/lastmile-gobuild; 19 paket hijau), web via node:20-alpine,
  headless mcr.microsoft.com/playwright:v1.63.0-noble (deps /tmp/kilo/node_modules);
  container uji di network lastmile_internal dihapus setelah verifikasi.

### 2026-10-01 — Fase 6: Produksi go-live + CI/CD penuh + README & artikel (sesi 7)

- **Go-live backend produksi**: DNS pemilik aktif (`api.`/`ws.` → A 194.233.67.201;
  `lastmile-lab` → CNAME Vercel). Caddy site block (dari fase 1) langsung hidup —
  sertifikat ACME terbit; **healthz 200 dari internet** via
  `https://api.lastmile-lab.ricothen.com/healthz` (0,10 s) & `ws.` (0,05 s).
- **Verifikasi end-to-end jalur publik**: web app dibangun headless dengan env
  domain produksi (`wss://ws.…/ws` + `https://api.…` — identik konfigurasi
  Vercel) → LIVE LINK di Chromium headless: WS upgrade TLS + REST publik +
  `/api/kpi` angka nyata + replay SESSION via `/api/replay/*` (3 231 frame) +
  `demo/presets` 200. **Replay fallback di domain produksi terverifikasi dua
  cara**: blokir domain dari browser (route.abort + routeWebSocket close) dan
  **stop backend sungguhan** (healthz 502) — banner REPLAY MODE + tombol DEMO
  OFFLINE, start ulang → LIVE kembali; console error 0 di luar expected.
  Skrip: `reports/phase-06-ui-verify.mjs`, bukti `phase06-*.png` (4).
- **CI/CD penuh**: workflow `images` + job `deploy` (SSH → `scripts/deploy.sh`
  = cek load → `compose pull` → `up -d` profile sim+chaos → curl healthz).
  Tanpa secret `DEPLOY_SSH_KEY` job SKIP-warning (CI hijau — terbukti di run
  images commit fase 6); setup 3 secret didokumentasikan (`deploy/README.md`
  §4.0). Siklus nyata dari VPS: load guard menolak saat load 8,19>8, jalan
  setelah turun — pull → up -d → healthz 200 via domain → "Deploy selesai"
  (fix `-f deploy/compose.yaml` di commit ikutan).
- **Uji beban ringan pasca go-live** (5 menit, kompos fase 2, seed 7, host
  load 5–7): sent/acked 4 263/4 261, 429 = 0, error klien 0, p50 301 ms /
  p99 2 487 ms; published = consumed = **4 261**, dup 0, DB delta window =
  4 263 (row 503 tetap tertulis — at-least-once jujur, idempotency siaga);
  66 assigned via pipeline (sisanya expire TTL — overload by design).
  Stack kembali mode demo (pipeline/infra di-rm, volume pg_data dipertahankan,
  ORDER_SOURCE=internal terverifikasi, 6/6 healthy).
- **RAM ≤ 2 GB terverifikasi ulang**: demo aktual ≈ 80 MiB / limit 672 MiB
  (6 kontainer); budget semua profile **1 888 MiB ≤ 2 GB** (docker stats +
  inspect limits di `reports/phase-06-prod.md` §5).
- **Monitoring & ketahanan**: UptimeRobot setup ±5 menit di runbook §5.5
  (aktivasi akun = pemilik); log rotation json-file 10m×3 terverifikasi live.
- **Keamanan**: scan bersih (CI guard + lokal), CORS GET/POST/OPTIONS `*`,
  surface internal tidak dipublish; **ADR D23** — endpoint mutasi publik tanpa
  auth dipertahankan (allowlist chaos, duel `cpus:1.0` satu-satu, self-heal;
  jalur naik bearer token bila abuse nyata) + runbook §9.
- **Konten etalase**: README final (arsitektur mermaid, screenshot hero,
  tabel metrik terukur fase 1–6, "Try it", reproduksi compose/loadtest/lab/
  chaos/replay, peta repo, prinsip) + artikel final
  `docs/blog/dispatch-explainability.md` (replay/inspect/golden demo + angka
  + 4 pelajaran) + spec `docs/PHASES/phase-07.md` (Copilot plugin adapter,
  tanpa key = tersembunyi, dry-run simulator sebagai judge).
- **Sisa aksi pemilik** (terdokumentasi, bukan blokir fase): import Vercel +
  assign domain; 3 secret GitHub; aktivasi UptimeRobot; cek 60fps laptop
  fisik. Semua di PROGRESS §"Langkah Sisa" & runbook.

### 2026-10-01 — Fase 5: Replay engine + Golden Demo + polish (sesi 6)

- **Replay engine (ADR D21)**: perekam sesi di rider-sim (`internal/replay`) — ring
  berbudget 15 menit @ 5 Hz (4 500 frame + pagar 24 MiB), tiap snapshot di-marshal JSON
  sekali lalu dikompresi gzip per frame (terukur ~2,5 KB/frame @ 100 rider; ring penuh
  ±11–12 MiB; RSS rider-sim 46,4 MiB / limit 160 MiB — naik dari 128 MiB, stack kini
  **1 888 MiB ≤ 2 GB**). Goroutine perekam terpisah — **engine tidak disentuh**,
  determinisme same-seed tetap diuji, kontrak `model.Snapshot` tidak berubah. Endpoint:
  `GET /api/replay/sessions` + `/api/replay/sessions/{id}` (dump meta + frames + ring
  keputusan dispatch, dedupe by seq, cap 8 000) via proxy gzip-passthrough api-gateway.
- **Dump = satu anggota gzip tunggal**: versi awal menggabungkan anggota gzip per frame
  (valid untuk curl/Go) — **Chromium menghentikan stream setelah anggota pertama** →
  frontend dapat JSON terpotong. Fix `65daada` (dekompresi-rekompresi streaming saat
  dilayani) + unit test assert tepat 1 anggota gzip.
- **Replay & Inspect UI (kriteria pemblokir terpenuhi, `reports/phase05-*.png`)**:
  scrub timeline 0–durasi buffer (akurasi ≤ 0,2 s @ 5 Hz, binary search pola MiniReplay),
  PLAY/PAUSE ×1/×4/×16, marker incident chaos di timeline (pemetaan wall→sim t),
  **klik rider → kartu status + order dibawa + pickup/dropoff + ALASAN keputusan dispatch
  dari ring decisions** (contoh terekam: "fifo: antrean tertua (umur 0s) → rider r40 idle
  terdekat (1011 m, haversine)"), klik order → status + umur + rider. **Tanpa rAF idle
  baru**: playhead dimajukan dari wall-clock di dalam loop rAF peta yang sudah ada;
  pause/scrub render on-demand (guard key frame + flag dirty); reduced-motion = PLAY
  hilang, scrub statis; responsive 1440/1024/768; console error 0.
- **Golden Demo (ADR D22)**: orchestrator di api-gateway (`demo.go` — narasi satu file)
  memakai kontrol yang SUDAH ada: surge/weather → sim-control, kill → chaos injector
  (ADR D19; incident chaos-kill tetap satu sumber kebenaran di Incident Timeline).
  3 preset ±90 s (dinner-rush 7 langkah incl. kill rider-sim, blackout-drill 3 kill,
  rain-commute tanpa kill). Endpoint `/api/demo/presets|play|stop|state`; UI launcher
  dock + banner narasi + STOP; tombol mati saat backend offline. **Eksekusi end-to-end
  di UI headless**: 7/7 langkah tampil, langkah tepat di detik narasi
  (t+0.3/12/36/54/64/74/86, selesai **tepat t+90.0s** — log api-gateway), kill → incident
  `chaos-kill` MTTD 642 ms → app pulih otomatis ke LIVE; pengukuran langsung API:
  dinner-rush 91,2 s, rain-commute 89,3 s.
- **Fixture fase 1 tidak regresi** (uji backend dimatikan sungguhan — stop rider-sim +
  ws-gateway + api-gateway): banner REPLAY MODE muncul, replay panel otomatis pakai
  fixture (scrub T+00:22/00:45 jalan), tombol `▶ DEMO · OFFLINE` (mati), console error
  0 aplikasi (connection-refused disaring sebagai expected).
- **Bug berat #2 ditemukan & diperbaiki**: peta hitam total saat verifikasi — CSS
  `.maplibregl-map { position: relative }` (maplibre-gl.css) menimpa utility Tailwind
  `absolute` setelah urutan chunk CSS bergeser (penambahan modul baru) → kontainer peta
  tinggi 0. Fix `668abf7`: `position: absolute` inline pada kontainer. Juga: ARIA
  `role="menu"` pada dropdown demo menghilangkan semantik button → diganti popover + aria-label.
- **Infra**: sim-control ikut profile `sim` (aktuator Golden Demo mode demo — RAM sudah
  terhitung; hanya profile diperluas). rider-sim 128→160 MiB. Tanpa port host baru.
- **CI/CD**: ci + images hijau di semua commit fase 5; deploy VPS: pull + up -d
  (profile sim + chaos) — 7/7 kontainer lastmile healthy.
- **Verifikasi**: `reports/phase-05-ui-verify.mjs` (mode utama + OFFLINE_MODE) —
  ALL CHECKS PASSED kedua mode; laporan lengkap `reports/phase-05-replay.md`.

### 2026-10-01 — Fase 4: KPI Command Deck + System Health + chaos (sesi 5)

- **KPI dari metrik nyata (ADR D20)**: engine dapat ring KPI fixed-cap
  (512 delivery terakhir + wall-clock tiap panggilan `Strategy.Assign`) —
  instrumentasi murni, determinisme same-seed tetap diuji; `model.Snapshot`
  dan `Engine.Metrics()` tidak disentuh. rider-sim expose `/internal/metrics`,
  api-gateway merangkai `GET /api/kpi` (p50/p95 delivery, p99 dispatch,
  utilisation, cost/order, orders/menit rolling, queue, lag pipeline, SLO,
  grid healthz cache 2 s, error budget dari chaos) dengan cache 400 ms agar
  polling UI 2 Hz tidak menghajar upstream. Nilai tak terukur = `null` → UI "—".
- **SLO eksplisit**: p95 delivery < 360 s; zero message loss (lag 0 + err 0);
  grid health core up; availability ≥ 99,9% (budget 0,1%). Kartu bergetar
  halus saat langgar (mati saat reduced-motion).
- **D16 ditinjau ulang**: Prometheus :9091 + Grafana :3030 TIDAK dipasang —
  +384 MiB limit mendorong stack ≥ 2,17 GB > anggaran keras 2 GB; metrik JSON
  dipertahankan, alasannya di ADR (UI Control Room adalah layer observability
  fase ini).
- **Chaos injector `chaos` :4206** (profile `chaos`, 64 MiB, internal saja):
  allowlist eksplisit 7 service stateless `lastmile-*` (infra ber-state &
  container lain → 403; unit test deny), monitor healthz 1 Hz + flap
  suppression 2 kegagalan, incident {t_start, t_detect, t_recover} persist ke
  volume. Kill = **SIGTERM ke PID 1 via Docker exec API** — dua fakta Docker
  29.8.1 diverifikasi di VPS: endpoint `/kill` TIDAK memicu restart policy dan
  PID 1 kebal SIGKILL dari dalam namespace (ADR D19 rev. 3). Self-heal = murni
  restart policy `unless-stopped` (chaos tidak pernah restart manual).
- **Eksperimen chaos nyata** (`reports/phase-04-chaos.md`): E1 rider-sim
  MTTD 811 ms / MTTR 2 035 ms · E2 ws-gateway 1 295/1 986 · E3 api-gateway
  1 139/3 017 · E4 dispatch-consumer saat 300/menit spike ×10: 1 073/1 982,
  **zero loss 5 460 ack == 5 460 baris DB**, topik habis, 0 error.
  Error budget jendela eksperimen: availability 99,69% vs SLO 99,9% →
  EXCEEDED (jujur — 4 kill dalam 13 menit; operasi normal = 100%).
- **UI (kriteria pemblokir terpenuhi, `reports/phase04-*.png`)**: KPI Command
  Deck (kartu mono tabular + area chart canvas on-data 2 Hz + SLO gauge),
  System Health (pipeline canvas — partikel hanya rAF saat panel terbuka,
  bukti rAF audit: 140/2 s terbuka vs 70-73/2 s baseline map — grid node +
  health events + tab CHAOS dengan tombol konfirmasi 2 langkah + incident
  timeline MTTD/MTTR), TopBar ticker incidents, responsive 1440/1024/768,
  reduced-motion penuh, console error 0, fallback "—" tanpa backend.
  Verifikasi headless: `reports/phase-04-ui-verify.mjs`.
- **CI/CD**: images GHCR +1 (total **9**, termasuk `lastmile-chaos`);
  gofmt/vet/test/race hijau; web typecheck+build hijau. Ram stack
  **1.856 MiB limit ≤ 2 GB** (chaos 64 MiB). PORTS.md: 4206 (container-only).
- **Bug ditemukan & diperbaiki saat verifikasi** (3 iterasi deploy→uji):
  API-kill Docker tidak memicu restart policy → exec SIGTERM PID 1 (ADR D19
  rev. 3); PID 1 kebal SIGKILL dari dalam namespace → SIGTERM (handler Go);
  probe in-flight saat kill menutup incident prematur → incident kill butuh
  bukti gagal→pulih (3 OK beruntun = pulih sub-probe). Semua masuk unit test.
- Stack VPS kembali mode demo (profile `sim` + `chaos`) setelah eksperimen;
  pipeline dihentikan; tidak ada stack lain yang disentuh.

### 2026-09-30 — Fase 3: Dispatch 4 strategi + Strategy Lab (sesi 4)

- **4 strategi deterministik** di `pkg/dispatch` di belakang `Strategy.Assign`
  yang sama, TANPA refactor engine (ADR D17): `fifo` (baseline, utuh);
  `batching` (window stateless 2 s dari `OrderView.NowMs` baru — field OPSIONAL,
  kontrak lama aman — lalu kluster per grid pickup, oldest-first dalam kluster);
  `zone` (grid 0,01° + cincin tetangga 1–2 + fallback global, locality bias);
  `optimal` (bipartite min-cost, Hungarian/Jonker-Volgenant murni Go O(n²m) —
  DILARANG OR-Tools/CGO; diverifikasi brute-force permutasi). Semua punya
  `Reason` + determinisme diuji. Registry `dispatch.ByName` dipakai rider-sim &
  dispatch-consumer via env `DISPATCH_STRATEGY` (default `fifo` — demo lama utuh).
- **p99 dispatch < 50 ms TERCATAT** (`reports/phase-03-bench.md` §3):
  batching 2,56 · fifo 3,05 · optimal 13,14 · zone 5,51 ms pada 100 order + 100
  rider (cap `--cpus 1`, GOMAXPROCS=1, host loadavg 9,5 — kondisi terburuk).
  Headroom 3× beban: optimal mulai mahal (p99 303 ms @ 300×300) — batas aman
  didokumentasikan.
- **Duel engine adil** (`internal/duel`, ADR D18): SATU generator order (seed sama)
  mem-pipe tiap order ke DUA engine identik per tick via `InjectExternal` — bukan
  dua generator terpisah; determinisme antar-run diuji (DeepEqual). Metrik engine
  baru (delivery dur, km on-task, rider-ms busy) via `sim.Engine.Metrics()` —
  kontrak `model.Snapshot` TIDAK disentuh sama sekali.
- **strategy-lab service** (:4205, internal saja): `POST /api/lab/run` → async
  (satu duel bersamaan, 409 bila sibuk), `GET /api/lab/results[/{id}]` (ringkasan
  + penuh: metrik, histogram bin bersama, ≤600 frame replay/sisi), `/healthz`;
  in-memory maks 8 hasil + persist opsional `LAB_DATA_DIR`; api-gateway proxy
  `/api/lab/*`. Duel 600 s virtual ≈ 1,4 s wall.
- **Angka duel first-class** (rush 45 order/menit, 600 s, seed 42): optimal
  delivered 255 vs FIFO 129 (+98%), expired 27 vs 46, cost/order 0,93 vs 2,04 km
  (−54%). Steady (70% kapasitas): semua strategi identik — pelajaran: pilihan
  strategi berbayar hanya di bawah tekanan. Batching: p50 −18% tapi expiry +48%
  (window vs TTL). Cost/order DIDEFINISIKAN eksplisit: km on-task per order
  terkirim, 1 unit = 1 km (laporan §1).
- **UI Strategy Lab** (kriteria pemblokir terpenuhi, `reports/phase-03-ui-*.png`):
  panel + form duel, tabel delta mono tabular (arah Δ diwarnai), histogram overlay
  bin bersama, peta replay KEMBAR 2 canvas kecil (latar jalan di-prerender,
  render on-demand — tanpa rAF saat idle), playback rAF berhenti sendiri,
  EXPORT JSON per duel, reduced-motion = frame akhir statis. Verifikasi headless
  Playwright: 0 console error. Bug ditemukan & diperbaiki saat verifikasi: polling
  duel tidak pernah start (effect deps pakai ref → diganti state), marker verifikasi
  salah cocok ticker TopBar, Legend tertimpa panel tinggi (disembunyikan saat lab
  terbuka), preset awal terlalu ringan (retune 20/30/45 order/menit — kapasitas
  armada 100 rider terukur ±25–30/menit).
- **CI/CD**: images GHCR +1 (total 8, termasuk `lastmile-strategy-lab`); gofmt/vet/
  test/race hijau; web typecheck+build hijau. Ram stack 1.792 MiB limit ≤ 2 GB
  (strategy-lab 256 MiB, aktual ~7 MiB).
- Stack VPS tetap mode demo (profile `sim`) — image baru di-pull, strategy-lab
  healthy, tidak ada stack lain yang disentuh.

### 2026-09-30 — Fase 2: Order ingestion + loadgen + Surge Console (sesi 3)

- **Pipeline order end-to-end** (ADR D15): `order-ingestion` (:4202, internal) —
  `POST /orders` validasi koordinat, idempotency Redis `SET NX` TTL 24h (header
  `Idempotency-Key` atau hash body), backpressure antrean 8192 (429), tulis
  Postgres + publish Kafka sync-ack → 201. `dispatch-consumer` (:4203) — konsumsi
  topic `orders` (3 partisi, RF 1), `Strategy` FIFO via **path bersama
  `pkg/dispatch`** (dipindah dari internal/sim, API lama tetap via alias),
  commit offset manual per batch (at-least-once), dedupe 3 lapis (LRU 100k,
  seen-set engine, unique index DB), injeksi assignment ke rider-sim
  (`POST /internal/orders`) dengan validasi ulang + fallback FIFO internal.
  `rider-sim` flag **`ORDER_SOURCE=internal|pipeline` (default internal — demo
  tidak pernah mati)**; injeksi men-clamp `created_ms` wall-clock ke jam virtual.
- **Load test spike ×10 ZERO MESSAGE LOSS** (`reports/phase-02-loadtest.md`):
  Poisson 300 order/menit, spike ×10 (50/s) 2×60 s dalam 300 s →
  **sent(acked) = consumed = stored = injected = 4.849**, 429 = 0, duplikat = 0,
  consumer lag = 0; p50 POST 175 ms / p99 1.903 ms. Spot-check SQL konsisten
  (4.849 orders + 4.908 events; 59 assigned).
- **Surge Console** (kriteria pemblokir < 1 s): slider ×1→×10, toggle RAIN
  (weather 0,6) & FLASH SALE (preset ×8); aksi → api-gateway `/api/control/*`
  (proxy + CORS POST) → `sim-control` (:3013, loopback) → rider-sim (+loadgen).
  Echo `su`/`we` field OPTIONAL di `model.Stats` (kontrak snapshot tidak breaking;
  frontend fase 1 aman). Terukur: rantai API 14–420 ms; UI headless echo
  226–874 ms — semua < 1 detik. Warna 100% token, tanpa rAF baru,
  `prefers-reduced-motion` aman, console error 0. Bukti: `reports/`
  (`phase-02-surge-console.png`, `phase-02-ui-verify.mjs`).
- **Postgres** (:5434): tabel `orders` + `order_events` (migration idempoten,
  job `db-migrate`); ingestion menulis `received`+event `ingested`, consumer
  update `assigned`+event (unique per order+type → replay aman).
- **loadgen** (:4204, profile `loadtest` saja): Poisson + skenario `spike` ×10
  terjadwal, kontrol live `/control`, metrik lengkap, exit bersih setelah run.
- **RAM** saat spike (profile sim+infra+pipeline+loadtest): **280 MiB aktual,
  1.536 MiB total limit ≤ 2 GB** — rincian per kontainer di laporan.
- **Grafana 3030** → diganti **metrik JSON** (`/metrics` per service + agregat
  `/api/metrics`) sesuai opsi spec fase 2 untuk RAM sempit (ADR D16); Grafana
  menyusul Fase 4.
- **CI/CD**: images GHCR +4 service (total 7: rider-sim, ws-gateway, api-gateway,
  order-ingestion, dispatch-consumer, sim-control, loadgen); go.mod naik ke
  go 1.26 (franz-go, go-redis, pgx, miniredis test-only); Dockerfile +BuildKit
  cache mount & GOMAXPROCS=2. CI backend/web hijau.
- **Bug ditemukan & diperbaiki saat verifikasi live** (detail di laporan §7):
  flag redpanda v24.2.7, healthcheck rpk (regex + broker addr), sintaks `-X`,
  franz-go idempotent acks=all, counter `sent` + exit loadgen, fan-out async
  sim-control, dan **clamp `created_ms`** (wall-clock vs jam virtual — tanpa ini
  antrean sim menumpuk tanpa TTL). Semua masuk unit test.
- Stack VPS dikembalikan ke mode demo (profile `sim`) setelah pengujian;
  pipeline bisa dinyalakan kapan pun (deploy/README.md §4.1).

### 2026-09-30 — Fase 1: Simulasi inti + Live Ops Map (sesi 2)
- **Data Berlin nyata dari OSM** (ADR D12): `tools/graphgen` (Overpass) → graph routing
  kompak `rider-sim/data/berlin_graph.json` (inner-city bbox: 9.459 node / 18.571 edge
  / 1.419 POI kuliner) + layer `roads.geojson`/`water.geojson` untuk peta. Filosofi:
  routing & visual dari data yang sama; tanpa tile provider/API key.
- **Core sim** (ADR D13): `internal/sim` — virtual clock deterministik (seed), rider
  state machine idle/to_pickup/pickup/delivering, order Poisson + TTL antrean, dispatch
  FIFO via interface `Strategy` (+ reason explainability, ring 25 keputusan). Unit test:
  routing, urutan FIFO, delivery end-to-end, TTL expiry, determinism same-seed.
- **3 service Go**: rider-sim (:4201 internal, graph embedded), ws-gateway (:3012,
  poll 10 Hz → WS fan-out, drop frame klien lambat), api-gateway (:3010, snapshot REST
  + CORS). Semua ada `/healthz`, Dockerfile multi-stage non-root, compose profile
  `sim` (128m+64m+64m, loopback bind).
- **Frontend Pulse** (`apps/web`): Next.js 14 + TS + Tailwind + Framer Motion; tokens
  tunggal `src/lib/tokens.ts` → CSS vars di-inject layout → Tailwind/canvas/MapLibre
  style — nol hex di komponen. Live Ops Map: style gelap self-hosted dari GeoJSON OSM;
  rider/order/assignment digambar overlay canvas 2D rAF — interpolasi 60 fps antar
  snapshot 10 Hz, pulsa radar order baru, garis dashed amber/violet, trail glow.
  `prefers-reduced-motion` dihormati penuh (MotionConfig user + canvas menonaktifkan
  pulsa/trail/dash/interpolasi). Kontras AA via pasangan token (teks kecil selalu
  primary/secondary; muted hanya dekoratif besar).
- **Replay fallback** (ADR D14): WS tidak hidup 3 detik / feed stall → fixture rekaman
  nyata (`tools/fixturegen`, seed 7, 45 s @5 Hz, 912 KB) diputar loop + banner REPLAY,
  auto-reconnect 6 s. Terverifikasi headless: 0 error halaman, banner muncul.
- **Perf 60fps** (kriteria pemblokir): diukur headless chromium di VPS —
  draw canvas EMA **3,1 ms/frame**, MapLibre hanya render saat interaksi (12 paint/20 s),
  React 2 Hz + memoized subtrees, p50 frame **16,7 ms** (= vsync 60 fps) di semua mode.
  Absolute avg fps di VPS terkontaminasi load host (5–9, software WebGL) — angka final
  di laptop fisik = langkah sisa (prosedur di atas).
- **CI/CD**: ci.yaml + job backend (gofmt/build/vet/test) & web (typecheck+build);
  `images.yaml` → GHCR `lastmile-{rider-sim,ws-gateway,api-gateway}` (latest + sha).
- **Deploy backend VPS**: image di-pull dari GHCR, stack `lastmile` profile `sim` hidup
  (rider-sim + ws-gateway + api-gateway healthy); Caddy site block api./ws. dipasang
  (reload tanpa restart) — menunggu DNS record pemilik domain untuk go-live publik.

### 2026-09-30 — Fase 0: Bootstrap
- Keputusan sudah dikunci sebelum repo (hasil brainstorming + riset Delivery Hero):
  lihat `BLUEPRINT.md` §5 (ADR D1–D11).
- Struktur monorepo dibuat; docs sumber kebenaran ditulis (AGENTS, BLUEPRINT, ROADMAP,
  DESIGN, PHASES/phase-00).
- `deploy/` skeleton: compose.yaml (infra dengan limit sumber daya), site block Caddy
  (api./ws. subdomain), runbook `deploy/README.md`.
- `apps/web/`: landing statis bertema design tokens (deployable ke Vercel tanpa build).
- CI: docs-guard + YAML valid (`.github/workflows/ci.yaml`).
- Blok port lastmile didaftarkan di `/home/rico/PORTS.md` (3010/3012/3013/3030,
  4201–4204, 5434, 6380, 9091, 19092).
- Repo GitHub `lastmile-lab` dibuat (account ricothenfx), commit pertama di-push.

### Keputusan menunggu / catatan
- Deploy Vercel butuh one-time auth akun Vercel pemilik (import repo atau CLI login).
- DNS records api./ws. → IP VPS dikelola pemilik domain; detail `deploy/README.md` §DNS.
