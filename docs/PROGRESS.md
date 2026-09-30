# PROGRESS — lastmile-lab

> Log status kerja. **Sesi AI/developer baru: baca file ini PERTAMA setelah AGENTS.md.**
> Update setiap selesai pekerjaan berarti, di commit yang sama dengan kodenya.

## Status Saat Ini

- **Fase aktif:** 4 — KPI Command Deck + System Health + chaos
  (spec: `docs/PHASES/phase-04.md`)
- **Kondisi:** Fase 3 SELESAI — 4 strategi dispatch deterministik di `pkg/dispatch`
  (FIFO baseline + Batching/Zone/Optimal Hungarian murni Go, ADR D17), p99 keputusan
  **2,6–13,1 ms** @ 100×100 (target < 50 ms, `reports/phase-03-bench.md`), Strategy
  Lab duel A/B adil (satu generator → dua engine, ADR D18) via service
  `strategy-lab` (:4205 internal) + API `/api/lab/*` + panel UI (peta kembar,
  tabel delta, histogram bin bersama, export JSON). 8 image backend di GHCR.
  VPS berjalan mode demo (profile `sim`), strategy-lab healthy.
- **Langkah berikutnya:** kick-off Fase 4 (baca AGENTS → PROGRESS → ROADMAP →
  `PHASES/phase-04.md`): KPI Command Deck + System Health + chaos injector;
  tinjau keputusan Grafana/Prometheus (D16) di awal fase.
- **Blokir/tergantung user:** none untuk koding. GO-LIVE publik tetap langkah
  pemilik domain/akun (DNS `ws.`/`api.` → IP VPS; auth Vercel) — bukan blokir fase.

## Langkah Sisa Go-Live (butuh akses pemilik — bukan blokir fase)

1. **DNS** (pemilik domain ricothen.com): `ws.lastmile-lab` → A record IP VPS;
   `api.lastmile-lab` → A record IP VPS; `lastmile-lab` → CNAME `cname.vercel-dns.com`.
2. **Caddy:** blok `deploy/caddy/lastmile-lab.caddy` SUDAH dipasang + reload
   (lihat log). Verifikasi ulang setelah DNS aktif:
   `curl https://ws.lastmile-lab.ricothen.com/healthz`.
3. **Vercel:** import repo (root dir `apps/web`), set env `NEXT_PUBLIC_WS_URL=
   wss://ws.lastmile-lab.ricothen.com/ws` & `NEXT_PUBLIC_API_URL=https://api.lastmile-
   lab.ricothen.com`, lalu domain custom. Frontend tanpa WS → otomatis replay mode
   (tidak pernah putih). Surge Console + Strategy Lab otomatis ikut via `api.` yang sama.
4. **Verifikasi 60fps di laptop fisik** (kriteria DoD fase 1 — terpenuhi secara
   struktural; angka final di hardware target): buka app → DevTools Performance →
   CPU 4× throttle → rekam 15 s → harapkan p50 frame ≤ 16,7 ms. Fase 3 menambah dua
   canvas lab kecil yang render ON-DEMAND (tanpa rAF saat idle) — budget idle
   tidak berubah; playback lab hanya rAF saat PLAY ditekan.

## Log

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
