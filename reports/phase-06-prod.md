# Fase 6 — Produksi: Go-Live, CI/CD Penuh, Uji Beban & Keamanan

> Tanggal: 2026-10-01 · Stack: `docker compose -p lastmile` mode demo (profile
> `sim` + `chaos`) · Host: vmi3585780 (4 core / 8 GB, load 5–8 selama sesi —
> semua langkah berat dimonitor terhadap batas load > 8).

## 1. Go-live: HTTPS hidup dari internet

DNS pemilik domain **aktif** (diverifikasi `host`):

| Record | Nilai terverifikasi |
|---|---|
| `api.lastmile-lab.ricothen.com` | A → **194.233.67.201** (VPS) |
| `ws.lastmile-lab.ricothen.com` | A → **194.233.67.201** (VPS) |
| `lastmile-lab.ricothen.com` | CNAME → `cname.vercel-dns.com` (Vercel) |

HTTPS via Caddy (site block `deploy/caddy/lastmile-lab.caddy`, terpasang sejak
Fase 1; sertifikat ACME terbit otomatis setelah DNS aktif):

```
curl https://api.lastmile-lab.ricothen.com/healthz
  → 200 {"lab_ok":true,"ok":true,"service":"api-gateway","sim_ok":true}  (0,10 s)
curl https://ws.lastmile-lab.ricothen.com/healthz
  → 200 {"clients":0,"last_seq":...,"ok":true,"service":"ws-gateway"}     (0,05 s)
```

**WebSocket upgrade via TLS publik terverifikasi end-to-end**: UI headless
(pola Vercel — dibangun dengan `NEXT_PUBLIC_WS_URL=wss://ws.lastmile-lab
.ricothen.com/ws` & `NEXT_PUBLIC_API_URL=https://api.lastmile-lab.ricothen.com`)
mencapai **LIVE LINK** — berarti `wss://` + REST `https://` publik bekerja
penuh lewat Caddy (`reports/phase06-prod-live.png`).

### 1.1 Sisa aksi pemilik (satu-satunya go-live yang belum aktif)

- **Vercel**: import repo (root dir `apps/web`), set env `NEXT_PUBLIC_WS_URL=
  wss://ws.lastmile-lab.ricothen.com/ws` + `NEXT_PUBLIC_API_URL=https://api.
  lastmile-lab.ricothen.com`, assign domain `lastmile-lab.ricothen.com`.
  Saat sesi ini, domain masih menolak TLS (`SSL_ERROR_SYSCALL` — domain belum
  di-assign ke project Vercel). Frontend tanpa backend → replay mode (tidak
  pernah putih); tidak ada perubahan repo yang diperlukan.

## 2. CI/CD: push → build → image → deploy

Pipeline (workflow `images`): build 9 image → push GHCR (`latest` + sha) →
**job `deploy` baru (Fase 6)**: SSH ke VPS → `scripts/deploy.sh` = cek load →
`docker compose -p lastmile --profile sim --profile chaos pull && up -d` →
verifikasi `/healthz` (domain publik, fallback loopback).

- **Prasyarat sekali (aksi pemilik)**: secret `DEPLOY_SSH_KEY`, `DEPLOY_SSH_HOST`,
  `DEPLOY_SSH_USER` (langkah persis di `deploy/README.md` §4.0). Tanpa key,
  job **SKIP dengan warning — CI tetap hijau**; begitu secret dipasang,
  auto-deploy aktif tanpa perubahan workflow lagi.
- **Jalur satu perintah**: `scripts/deploy.sh` (di VPS) atau
  `DEPLOY_SSH_HOST=user@host scripts/deploy.sh` (dari mesin lain) — jalur yang
  sama persis dengan job CI.
- **Siklus nyata fase ini**: commit penutup fase 6 memicu workflow `images`
  (build ulang 9 image + push GHCR), lalu `scripts/deploy.sh` dijalankan di VPS
  — pull image baru, `up -d`, healthz hijau (bukti: log sesi + run GitHub
  Actions `images` pada commit fase 6). Job `deploy` CI sendiri menunggu
  secret pemilik (skip warning, terdokumentasi).

## 3. Monitoring & ketahanan

- **UptimeRobot** — setup ±5 menit didokumentasikan di `deploy/README.md` §5.5
  (monitor HTTP(s) 5 menit pada `https://api.lastmile-lab.ricothen.com/healthz`,
  opsional `ws.` + frontend; alert email). Eksekusi akun = aksi pemilik.
- **Log rotation** — aktif & terverifikasi live:
  `docker inspect lastmile-rider-sim LogConfig → json-file {max-size:10m, max-file:3}`
  (anchor `logging` compose berlaku untuk semua service).
- **Replay fallback di domain produksi** — terverifikasi DUA cara:
  1. Domain publik diblokir dari browser (`route.abort` + tutup WS) → banner
     **REPLAY MODE** + tombol `▶ DEMO · OFFLINE` (`phase06-fallback-blocked.png`).
  2. **Backend benar-benar distop** (`docker stop` rider-sim + ws-gateway +
     api-gateway): `healthz` via domain → **502**, UI headless tetap hidup —
     banner REPLAY MODE + fixture fase 1 (`phase06-prod-offline.png`), lalu
     start ulang → **LIVE LINK kembali** (`phase06-prod-recovered.png`).
  Console error aplikasi = 0 di semua mode (CORS/connection-refused saat backend
  mati adalah konsekuensi yang diharapkan, disaring sebagai expected).

## 4. Uji beban produksi ringan (5 menit, pasca go-live)

Skenario = konfigurasi kompos fase 2 (reproducible, seed 7): Poisson **300
order/menit** + **spike ×10** (50/s) 2×60 s, durasi 300 s, lewat pipeline penuh
(loadgen → order-ingestion → Redpanda 3 partisi → dispatch-consumer → rider-sim
`ORDER_SOURCE=pipeline` → Postgres). Konteks jujur: host load 5–7 saat run
(lebih tinggi dari run fase 2) — latensi absolut lebih tinggi, loss tetap nol.

| Lapisan | Metrik | Nilai |
|---|---|---|
| loadgen | sent / acked 201 | 4 263 / **4 261** |
| loadgen | rejected 429 / error klien | **0 / 0** |
| loadgen | latensi POST p50 / p99 | 301 ms / 2 487 ms (puncak spike, host load 5–7) |
| order-ingestion | published / publish_errors | 4 261 / **0** |
| order-ingestion | errors_503 | 2 (transient puncak spike — lihat catatan) |
| dispatch-consumer | consumed / duplicates (lokal/engine) | **4 261 / 0 / 0** |
| dispatch-consumer | db_errors / parse_errors / sim_errors | 0 / 0 / 0 |
| dispatch-consumer | assigned (via pipeline) | 66 (sisanya expire TTL 90 s — overload design) |
| Postgres | delta baris `orders` window run | **4 263 = sent persis** |

**Zero message loss terjaga**: published = consumed = acked = 4 261; duplikat 0;
lag habis. Catatan 2×503: row Postgres tetap tertulis (4 263 delta = sent),
klien menerima kegagalan jujur (bukan data hilang), idempotency-key mencegah
duplikasi bila di-retry — perilaku at-least-once yang benar.

Setelah run: kontainer pipeline/infra dihapus (`rm -sf`, volume pg_data
dipertahankan), stack kembali **mode demo 6/6 healthy**,
`rider-sim ORDER_SOURCE=internal` terverifikasi.

## 5. Budget RAM ≤ 2 GB — terverifikasi ulang pasca go-live

`docker stats` mode demo (go-live):

| Container | Aktual | Limit |
|---|---|---|
| rider-sim | 20,6 MiB | 160 MiB |
| ws-gateway | 14,8 MiB | 64 MiB |
| api-gateway | 9,8 MiB | 64 MiB |
| sim-control | 9,2 MiB | 64 MiB |
| strategy-lab | 11,9 MiB | 256 MiB |
| chaos | 13,2 MiB | 64 MiB |
| **Total demo** | **≈ 80 MiB** | **672 MiB** |

Budget lengkap semua profile (tanpa one-shot job) = **1 888 MiB ≤ 2 GB**
(rider-sim naik 128→160 MiB di fase 5 untuk ring replay — sudah terhitung).
Selama load test: infra + pipeline + loadtest ikut dalam anggaran yang sama
(postgres 384m + redis 64m + redpanda 512m + ingestion 96m + consumer 96m +
loadgen 64m) — total limit tetap 1 888 MiB; aktual puncak ≈ 480 MiB.

## 6. Pemeriksaan keamanan dasar

- **Secret scan**: guard CI (pola token/privkey) + scan lokal repo — bersih;
  `deploy/.env` gitignored & tak pernah di-track; tidak ada secret baru.
- **CORS**: api-gateway `GET, POST, OPTIONS` dari `*` — cukup untuk UI publik,
  tanpa cookie/credential.
- **Exposure endpoint mutasi** (`/api/control/*`, `/api/lab/run`,
  `/api/chaos/kill`, `/api/demo/play|stop` via `api.` publik): **KEPUTUSAN
  DIBUKA TANPA AUTH** — risiko residu, mitigasi mekanis (allowlist chaos,
  satu duel bersamaan `cpus:1.0`, self-heal), dan jalur naik (bearer token
  bila abuse nyata) terdokumentasi di **ADR D23** (BLUEPRINT.md). Tidak ada
  mekanisme auth besar yang ditambahkan (sesuai batas fase).
- **Surface internal**: postgres/redis/redpanda/ingestion/consumer/loadgen tidak
  dipublish; api/ws di-publish hanya ke IP gateway Caddy; sim-control loopback.

## 7. Verifikasi UI headless via domain produksi

`reports/phase-06-ui-verify.mjs` (Playwright 1.63, swiftshader, app dibangun
dengan env domain publik — identik dengan konfigurasi Vercel):

| Check | Hasil |
|---|---|
| LIVE via `api.`/`ws.` publik (TLS + WS upgrade) | LIVE LINK ✓ |
| `GET /api/kpi` via domain | angka nyata ✓ |
| `GET /healthz` via domain (dari browser) | 200 ok ✓ |
| Replay via `/api/replay/*` publik (gzip passthrough) | SESSION 3 231 frame ✓ |
| `GET /api/demo/presets` publik | 200 (exposure D23) ✓ |
| Fallback: domain diblokir dari browser | REPLAY MODE ✓ |
| Backend produksi distop sungguhan → pulih | REPLAY MODE → LIVE ✓ |
| Console error (di luar expected backend-mati) | 0 ✓ |

Bukti: `phase06-prod-live.png`, `phase06-fallback-blocked.png`,
`phase06-prod-offline.png`, `phase06-prod-recovered.png`.

Artefak headless yang dikenal (bukan bug): potongan putih ±25 px pojok kanan-
bawah GL canvas di swiftshader; rAF headless dipengaruhi load host (5–8) —
dilaporkan apa adanya.

## 8. Status DoD fase 6

| DoD | Status |
|---|---|
| HTTPS hidup api.+ws. — healthz hijau dari internet | ✅ (200 via domain) |
| Frontend lastmile-lab.ricothen.com (Vercel) | ⏳ **aksi pemilik** (DNS ✅, kode+env ✅, import+assign domain sisa) |
| CI/CD push→build→image→deploy (otomatis / satu perintah) | ✅ job deploy + `scripts/deploy.sh`; siklus nyata di commit fase 6; auto-deploy aktif setelah secret pemilik |
| UptimeRobot /healthz | 📋 setup terdokumentasi runbook §5.5 (aktivasi = pemilik) |
| Log rotation aktif | ✅ (json-file 10m ×3, inspect live) |
| Budget RAM ≤ 2 GB pasca go-live | ✅ (1 888 MiB limit; §5) |
| Uji beban ringan + laporan | ✅ (§4 — zero loss) |
| README final | ✅ (arsitektur, screenshot, tabel metrik fase 2–5, reproduksi) |
| Artikel teknis | ✅ `docs/blog/dispatch-explainability.md` |
| Keamanan dasar + keputusan exposure mutasi | ✅ ADR D23 + runbook §9 |

**Kesimpulan**: seluruh pekerjaan sisi repo selesai & hijau. Sisa aksi pemilik
(yang tidak memblokir penyelesaian fase, konvensi sama seperti fase 1–5):
(1) import Vercel + assign domain frontend; (2) 3 secret GitHub untuk
auto-deploy; (3) aktivasi monitor UptimeRobot; (4) verifikasi 60fps di laptop
fisik (prosedur di PROGRESS). Semua langkah tertulis di `deploy/README.md` §4.0
& §5.5 serta PROGRESS.md.
