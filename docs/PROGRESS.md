# PROGRESS — lastmile-lab

> Log status kerja. **Sesi AI/developer baru: baca file ini PERTAMA setelah AGENTS.md.**
> Update setiap selesai pekerjaan berarti, di commit yang sama dengan kodenya.

## Status Saat Ini

- **Fase aktif:** 3 — Dispatch engine 4 strategi + Strategy Lab
  (spec: `docs/PHASES/phase-03.md`)
- **Kondisi:** Fase 2 SELESAI — pipeline order lengkap (ingestion idempotent →
  Redpanda → dispatch-consumer → rider-sim `ORDER_SOURCE=pipeline|internal`),
  Surge Console live (< 1 s), load test spike ×10 zero message loss
  (`reports/phase-02-loadtest.md`), RAM stack 280 MiB aktual / 1.536 MiB limit.
  7 image backend di GHCR. VPS berjalan dalam **mode demo** (profile `sim`,
  generator internal — pipeline hidup kapan pun lewat perintah di
  `deploy/README.md §4.1`).
- **Langkah berikutnya:** kick-off Fase 3 (baca AGENTS → PROGRESS → ROADMAP →
  `PHASES/phase-03.md`), mulai dari interface Strategy di `pkg/dispatch` yang
  sudah siap diisi Batching/Zone/Optimal.
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
   (tidak pernah putih). Surge Console otomatis ikut via `api.` yang sama.
4. **Verifikasi 60fps di laptop fisik** (kriteria DoD fase 1 — terpenuhi secara
   struktural; angka final di hardware target): buka app → DevTools Performance →
   CPU 4× throttle → rekam 15 s → harapkan p50 frame ≤ 16,7 ms. Fase 2 menambah
   panel Surge Console DOM kecil tanpa rAF baru — budget draw canvas berubah.

## Log

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
