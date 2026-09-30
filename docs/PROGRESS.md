# PROGRESS — lastmile-lab

> Log status kerja. **Sesi AI/developer baru: baca file ini PERTAMA setelah AGENTS.md.**
> Update setiap selesai pekerjaan berarti, di commit yang sama dengan kodenya.

## Status Saat Ini

- **Fase aktif:** 2 — Order ingestion + load generator + Surge Console dasar
  (spec: `docs/PHASES/phase-02.md`)
- **Kondisi:** Fase 1 SELESAI — simulasi inti + Live Ops Map hidup end-to-end
  (Go backend 3 service + Next.js app + replay fallback + CI/GHCR). Detail di log
  2026-09-30 (sesi 2) di bawah.
- **Langkah berikutnya:** kick-off Fase 2 (baca AGENTS → PROGRESS → ROADMAP →
  `PHASES/phase-02.md`), mulai dari order-ingestion + Redpanda consumer.
- **Blokir/tergantung user:** none untuk koding. Untuk GO-LIVE publik (opsional, bukan
  blokir fase): (a) DNS `ws.lastmile-lab` & `api.lastmile-lab` → A record IP VPS,
  (b) one-time auth Vercel untuk deploy frontend — langkah tercantum di §Langkah sisa.

## Langkah Sisa Go-Live (butuh akses pemilik — bukan blokir fase 2)

1. **DNS** (pemilik domain ricothen.com): `ws.lastmile-lab` → A record IP VPS;
   `api.lastmile-lab` → A record IP VPS; `lastmile-lab` → CNAME `cname.vercel-dns.com`.
2. **Caddy:** blok `deploy/caddy/lastmile-lab.caddy` SUDAH dipasang + reload
   (lihat log). Verifikasi ulang setelah DNS aktif:
   `curl https://ws.lastmile-lab.ricothen.com/healthz`.
3. **Vercel:** import repo (root dir `apps/web`), set env `NEXT_PUBLIC_WS_URL=
   wss://ws.lastmile-lab.ricothen.com/ws` & `NEXT_PUBLIC_API_URL=https://api.lastmile-
   lab.ricothen.com`, lalu domain custom. Frontend tanpa WS → otomatis replay mode
   (tidak pernah putih).
4. **Verifikasi 60fps di laptop fisik** (kriteria DoD fase 1 — terpenuhi secara
   struktural; angka final di hardware target): buka app → DevTools Performance →
   CPU 4× throttle → rekam 15 s → harapkan p50 frame ≤ 16,7 ms. Bukti pengukuran
   headless + angka draw budget: lihat log sesi 2.

## Log

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
