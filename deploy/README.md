# Deploy & Runbook VPS — lastmile-lab

> Runbook operasional. Host: VPS vmi3585780 (4 core / 8 GB RAM / 96 GB disk).
> Registry port host: `/home/rico/PORTS.md` — sumber kebenaran di luar repo ini.

## 1. Topologi

```
Internet
  │
  ├─ lastmile-lab.ricothen.com ──────► VERCEL (frontend statis/Next.js)
  │
  └─ api.lastmile-lab.ricothen.com ──┐
     ws.lastmile-lab.ricothen.com ───┤
                                     ▼
                        Caddy (container turnaround-prod, 80/443)
                                     │ reverse_proxy ke 127.0.0.1
         ┌────────────────────────────┴───────────────────────────┐
         │  Docker compose project: lastmile  (network internal)  │
         │  3010 api-gateway · 3012 ws-gateway · 3013 sim-control │
         │  3030 grafana (opsional) · 5434 postgres · 6380 redis  │
         │  4201 rider-sim · 4202 order-ingestion · 4203 dispatch │
         │  4204 loadgen (profile loadtest saja)                  │
          │  4205 strategy-lab (Fase 3, internal network saja)     │
          │  4206 chaos (Fase 4, internal + docker.sock allowlist) │
          │  redpanda 19092 (internal network saja)                │
         └────────────────────────────────────────────────────────┘
```

Pipeline order (Fase 2): `loadgen/klien → POST /orders (order-ingestion :4202,
idempotency Redis + Postgres + ack Kafka sync) → topic `orders` (Redpanda, 3
partisi) → dispatch-consumer :4203 (FIFO via pkg/dispatch, commit offset manual,
dedupe by order id) → injeksi ke rider-sim :4201 (validasi ulang + FIFO fallback)
→ Postgres `orders`/`order_events`. Kontrol live: api-gateway `/api/control/*` →
sim-control :3013 → rider-sim (+loadgen saat hidup).

Strategy Lab (Fase 3): UI → api-gateway `/api/lab/*` → strategy-lab :4205
(internal saja) → duel dua engine identik (satu generator order seed sama → dua
engine, strategi beda) — hasil metrik + histogram + frame replay kembar. Strategi
dispatch selectable per service via env `DISPATCH_STRATEGY`
(`fifo|batching|zone|optimal`, default `fifo`).

KPI + System Health + chaos (Fase 4): UI polling `GET /api/kpi` (2 Hz) — KPI
agregat dari metrik nyata (rider-sim `/internal/metrics` → p50/p95 delivery,
p99 dispatch, utilisation, cost/order; counters pipeline → lag/backlog; chaos
`/incidents` → MTTD/MTTR/error budget) + SLO eksplisit + grid healthz (cache
2 s). Chaos injector :4206 (internal, profile `chaos`) — `POST /api/chaos/kill
{"target":"rider-sim"}` = SIGTERM ke PID 1 di dalam container (exec API;
runtime Go keluar → restart policy jalan — endpoint `/kill` Docker TIDAK
memicu policy dan PID 1 kebal SIGKILL dari dalam namespace) dari **allowlist eksplisit 7 service
stateless `lastmile-*`** (postgres/redis/redpanda/chaos sendiri DI LUAR
allowlist; container lain DITOLAK 403). Pemulihan = restart policy
`unless-stopped` Docker (self-heal yang diukur — chaos tidak restart manual).
Incident {t_start, t_detect, t_recover} persist ke volume `chaos_data`
(`incidents.json`); MTTD = t_detect−t_start, MTTR = t_recover−t_detect.
Monitor 1 Hz + flap suppression 2 kegagalan beruntun (probe timeout 1,5 s).

## 2. Blok Port (terdaftar di /home/rico/PORTS.md)

| Port | Pemilik | Bind | Catatan |
|---|---|---|---|
| 3010 | lastmile-api-gateway | 127.0.0.1 | via Caddy api.*; proxy /api/control/* + /api/metrics |
| 3012 | lastmile-ws-gateway | 127.0.0.1 | via Caddy ws.* (WebSocket) |
| 3013 | lastmile-sim-control | 127.0.0.1 | kontrol surge/weather (dipakai UI via api-gateway) |
| 3030 | lastmile-grafana (opsional) | 127.0.0.1 | Fase 4+ (Fase 2: metrik JSON /api/metrics — ADR D16) |
| 4201 | rider-sim | tidak dipublish | jaringan docker internal |
| 4202 | order-ingestion | tidak dipublish | POST /orders internal |
| 4203 | dispatch-consumer | tidak dipublish | healthz/metrics internal |
| 4204 | loadgen | tidak dipublish | hanya profile loadtest |
| 4205 | strategy-lab | tidak dipublish | Fase 3 — duels via api-gateway /api/lab/* |
| 4206 | chaos | tidak dipublish | Fase 4 — kill allowlist via api-gateway /api/chaos/*; profile `chaos` |
| 5434 | lastmile-postgres | 127.0.0.1 | instance sendiri, BUKAN 5433 aviation |
| 6380 | lastmile-redis | 127.0.0.1 | instance sendiri, BUKAN 6379 aviation |
| 9091 | lastmile-prometheus (reserved) | 127.0.0.1 | Fase 4+ |
| 19092 | redpanda (Kafka API) | internal saja | menghindari zona ambigu 9090+ monitoring |

## 3. Aturan Emas Operasional

1. **Selalu** `docker compose -p lastmile ...` — project name eksplisit.
2. Semua service wajib punya `mem_limit`/`cpus` + `healthcheck` + `restart: unless-stopped`.
3. Total RAM stack ≤ 2 GB. VPS hanya punya ~1–2 GB sisa — jangan ditambah tanpa rencana.
4. **Build di GitHub Actions → GHCR → VPS pull.** Jangan build berat di VPS
   (load sudah 2x oversubscribed; produksi lain ikut terseret).
5. Bind host port ke `127.0.0.1`; hanya yang di-proxy Caddy yang diakses publik.
   Pengecualian mekanis: Caddy hidup di bridge network stack lain, jadi service yang
   di-proxy di-publish ke **IP gateway jaringan Caddy** (`LASTMILE_BIND_IP` di
   `deploy/.env`, saat ini 172.19.0.1) — tetap tidak publik ke internet.
6. Tidak menyentuh container/stack lain (turnaround-prod, aviation, ro-botriv).
7. Update `/home/rico/PORTS.md` SEBELUM memakai port baru.

## 4. Alur Deploy

```
git push main
  └─► GitHub Actions: build image Go/web → push ghcr.io/ricothenfx/lastmile-*
        └─► VPS: docker compose -p lastmile pull && docker compose -p lastmile up -d
```

- Rollback: `docker compose -p lastmile pull` tag sebelumnya + `up -d` (image lama
  diretas oleh tag; simpan 3 tag terakhir).
- Zero-downtime target: satu service diganti satu waktu (`up -d --no-deps <svc>`).

### 4.0 Deploy otomatis & satu perintah (Fase 6)

```
git push main (apps/services/** berubah)
  └─► workflow `images`: build 9 image → push ghcr.io/ricothenfx/lastmile-*
        └─► job `deploy`: SSH ke VPS → scripts/deploy.sh
              = cek load → compose pull → up -d (profile sim + chaos) → curl /healthz
```

**Prasyarat sekali (aksi pemilik akun GitHub) — ✅ TERPASANG 2026-10-02:**

```bash
# 1. Buat keypair khusus deploy (JANGAN pakai key personal) + pasang public key
#    di ~/.ssh/authorized_keys VPS.
# 2. Set secret repo (repo lastmile-lab → Settings → Secrets and variables → Actions):
gh secret set DEPLOY_SSH_KEY  < ~/.ssh/<file key privat deploy>
gh secret set DEPLOY_SSH_HOST < hostname / IP publik VPS>
gh secret set DEPLOY_SSH_USER < rico>
```

- ✅ Status: `DEPLOY_SSH_KEY` (keypair `~/.ssh/lastmile_deploy`), `DEPLOY_SSH_HOST`
  (`194.233.67.201`), `DEPLOY_SSH_USER` (`rico`) terpasang; job `deploy` CI
  SUKSES end-to-end (run `36956804558` — log PROGRESS sesi 10). Workflow mengisi
  `DEPLOY_SSH_KEY_FILE` otomatis — job deploy butuh fix itu karena runner CI
  tidak punya identitas default (ssh tanpa `-i` → Permission denied).
- Tanpa `DEPLOY_SSH_KEY`, job `deploy` **SKIP dengan warning — CI tetap hijau**
  (auto-deploy tinggal aktif begitu secret dipasang; tidak ada perubahan workflow lagi).
- Deploy manual satu perintah (jalur yang sama persis dengan job deploy):

```bash
scripts/deploy.sh                                # dijalankan di VPS
DEPLOY_SSH_HOST=rico@<host> scripts/deploy.sh    # dari mesin lain (SSH)
```

- Job deploy juga bisa dipicu manual: Actions → images → Run workflow.

### 4.1 Mode operasi stack (Fase 2)

```bash
# DEMO (aman 24/7 — tanpa infra, generator internal, demo tidak pernah mati):
docker compose -p lastmile --profile sim up -d

# PIPELINE PENUH (order via Kafka; rider-sim ORDER_SOURCE=pipeline):
docker compose -p lastmile -f deploy/compose.yaml -f deploy/compose.pipeline.yaml \
  --profile infra --profile sim --profile pipeline up -d

# LOAD TEST (sementara — matikan lagi setelah selesai):
... --profile loadtest up -d loadgen     # hasil: docker logs lastmile-loadgen
```

### 4.2 Strategy Lab & benchmark (Fase 3)

- Duel A/B: UI (panel Strategy Lab) atau `curl -s localhost:3010/api/lab/run
  -d '{"strategy_a":"fifo","strategy_b":"optimal","preset":"rush","seconds":600}'`;
  hasil: `localhost:3010/api/lab/results[/id]`. Satu duel bersamaan (409 bila sibuk).
- Benchmark p99 dispatch: manual, BUKAN gate CI — lihat `reports/phase-03-bench.md`
  (perintah repro + kondisi hardware di §6 laporan).
- Ganti strategi live demo: env `DISPATCH_STRATEGY` pada `rider-sim`/`dispatch-consumer`
  (override compose atau `docker compose -p lastmile --profile sim up -d` setelah edit).

### 4.3 KPI + chaos (Fase 4)

- Stack hidup dengan chaos: `docker compose -p lastmile --profile sim --profile chaos up -d`
  (tanpa profile `chaos`, UI menampilkan chaos STANDBY — KPI tetap hidup).
- KPI agregat: `curl localhost:3010/api/kpi` (SLO + grid + budget ikut di payload).
- Chaos kill (HANYA target allowlist): `curl -s localhost:3010/api/chaos/kill
  -X POST -d '{"target":"rider-sim"}'` → 202; timeline: `GET /api/chaos/incidents`.
  Target di luar allowlist → 403. Eksperimen jangan saat load host > 8
  (`cat /proc/loadavg` dulu) — bukti angka: `reports/phase-04-chaos.md`.
- Kembali aman: `docker compose -p lastmile --profile sim up -d` (chaos mati).

### 4.4 AI Ops Copilot (Fase 7 — profile `copilot`, OFF-by-default)

- Tanpa key, TIDAK melakukan apa-apa: service tidak ikut ter-deploy, gateway menjawab
  `GET /api/copilot/capabilities` → `{"enabled":false}`, UI menyembunyikan panel
  (ADR D24). Mode demo default (`--profile sim --profile chaos`) tidak berubah.
- **Mengaktifkan (aksi PEMILIK — jangan pernah menaruh key di repo/git):**
  1. `echo 'LASTMILE_OPENAI_API_KEY=sk-...' >> /home/rico/portfolio/lastmile-lab/deploy/.env`
     — **HARUS `deploy/.env`** (project directory compose; root `.env` TIDAK
     terbaca — diverifikasi empiris via `docker compose config`, 2026-10-02).
     Opsional: `LASTMILE_OPENAI_BASE_URL` untuk provider OpenAI-compatible lain,
     `LASTMILE_OPENAI_MODEL`, default `gpt-4o-mini`.
  2. `docker compose -p lastmile --profile sim --profile chaos --profile copilot pull && \
     docker compose -p lastmile --profile sim --profile chaos --profile copilot up -d`
  3. Verifikasi: `curl -s localhost:3010/api/copilot/capabilities` → `{"enabled":true}`;
     panel ADVISOR (Strategy Lab) & COPILOT (System Health) muncul di UI
     (tab tampil setelah panel dibuka — capabilities dicek saat panel mount).
     **Status: AKTIF sejak 2026-10-02** — evaluasi 15 kasus selesai
     (laporan fase 7 §6: metrik 5/5 benar, diag konteks-kosong ditolak validator).
- Menonaktifkan: hapus key dari `.env` lalu `up -d` tanpa profile `copilot`
  (atau `docker compose -p lastmile --profile copilot rm -sf copilot`).
- Rate limit bawaan 6/menit per endpoint (env `RATE_PER_MIN` pada service `copilot`);
  plan TIDAK pernah dieksekusi otomatis — tombol manusia dengan konfirmasi 2 langkah.
- Evaluasi kualitas jawaban (15 kasus ground truth): prosedur di
  `reports/phase-07-copilot.md` §6 — status: menunggu API key pemilik.


- Override `deploy/compose.pipeline.yaml` hanya mengubah `rider-sim` →
  `ORDER_SOURCE=pipeline`. Tanpa override itu, demo tetap internal.
- Setelah load test: kembali ke mode demo
  (`docker compose -p lastmile -f deploy/compose.yaml --profile sim up -d`).
- Angka zero-loss & prosedur lengkap: `reports/phase-02-loadtest.md`.
- Metrik pipeline (JSON): `GET :3010/api/metrics` (agregasi ingestion/consumer/
  loadgen) atau per service `/metrics` di jaringan internal.

## 5. Caddy (AKTIF — terverifikasi 2026-10-01)

Caddy produksi = container `turnaround-prod-caddy-1` (ports 80/443), config milik stack
turnaround-prod. Sentuhan yang sah:

1. Salin blok dari `deploy/caddy/lastmile-lab.caddy` ke Caddyfile produksi.
2. Reload TANPA restart: `docker exec turnaround-prod-caddy-1 caddy reload --config <path>`
   (verifikasi path config dengan `docker inspect` dulu).
3. `docker exec turnaround-prod-caddy-1 caddy validate --config <path>` SEBELUM reload.

**Status Fase 6:** blok `api.` + `ws.` terpasang & reload (log Fase 1); DNS pemilik
aktif; sertifikat terbit otomatis (ACME) — terverifikasi dari internet:

```
curl https://api.lastmile-lab.ricothen.com/healthz  → 200 (api-gateway)
curl https://ws.lastmile-lab.ricothen.com/healthz   → 200 (ws-gateway)
```

## 5.5 Monitoring eksternal — UptimeRobot (setup pemilik, ±5 menit)

1. Daftar gratis di uptimerobot.com (paket Free: 50 monitor, interval 5 menit).
2. Add New Monitor → **HTTP(s)**, interval 5 menit:
   - `api-healthz` → `https://api.lastmile-lab.ricothen.com/healthz`
     (harap 200 + JSON `{"ok":true,...}` — liveness api-gateway; bukan cukup
     TCP, karena /healthz juga mem-probe rider-sim).
   - Opsional: `ws-healthz` → `https://ws.lastmile-lab.ricothen.com/healthz`;
     `frontend` → `https://lastmile-lab.ricothen.com` (aktif setelah deploy Vercel).
3. Alert kontak: email pemilik (default). Keyword monitor (opsional): `\"ok\":true`
   agar 200 dengan body aneh pun dianggap down.
4. Timeout 30 s; jangan aktifkan "port monitoring" — cukup HTTP(s).

Health internal 24 jam: semua service expose `/healthz` (§7); UptimeRobot memantau
sisi internet (Caddy → api-gateway) — titik fail paling awal yang dilihat pengunjung.

## 6. DNS (pemilik domain) — ✅ SEMUA AKTIF (terverifikasi 2026-10-01/02)

| Record | Nilai | Status |
|---|---|---|
| `lastmile-lab` | CNAME `cname.vercel-dns.com` | ✅ hidup — project Vercel `lastmile-lab` (2026-10-02) |
| `api.lastmile-lab` | A → IP VPS | ✅ hidup — Caddy + ACME |
| `ws.lastmile-lab` | A → IP VPS | ✅ hidup — Caddy + ACME |

## 6.1 Frontend Vercel (project `lastmile-lab`)

- **Deploy otomatis**: repo GitHub terhubung — push ke `main` (perubahan `apps/web`)
  → build di infra Vercel → production `lastmile-lab.ricothen.com` otomatis.
  Jalur manual setara: `vercel deploy --prod` dari `apps/web` (CLI v62, project linked).
  **Penting**: project settings → **Root Directory = `apps/web`** (tanpa ini build
  git-integration gagal ERROR — build dari root repo tanpa package.json; sudah
  diset via API dan terbukti READY 2026-10-02).
- **Env produksi** (project settings → Environment Variables, scope Production):
  `NEXT_PUBLIC_WS_URL=wss://ws.lastmile-lab.ricothen.com/ws`,
  `NEXT_PUBLIC_API_URL=https://api.lastmile-lab.ricothen.com` — dipakai saat build
  (Next.js membake `NEXT_PUBLIC_*` di build time; ubah env → deploy ulang).
- **Deployment Protection: OFF** (situs portofolio publik — Vercel Authentication
  dimatikan via API, Attack Challenge Mode OFF via dashboard; nyala = semua
  pengunjung kena "Security Checkpoint", termasuk korporat/VPN). Protected
  sourcemaps boleh ON.
- Rollback: Vercel dashboard → Deployments → pilih versi → Promote to Production.

## 7. Kesehatan & Ketahanan 24 Jam

- Semua service expose `/healthz` (liveness) — dipantau UptimeRobot eksternal (Fase 6).
- Frontend punya **replay fallback**: backend mati ≠ situs mati.
- Log rotation json-file 10 MB ×3 (sudah di compose) — disk hanya 30 GB bebas.
- Sebelum deploy: cek `cat /proc/loadavg` dan `free -m` — bila load > 6 atau available
  RAM < 500 MB, tunda deploy dan selidiki dulu.

## 8. Sekret

- `deploy/.env` (gitignored): `LASTMILE_PG_PASSWORD`, dst.
- Template: `deploy/.env.example`. Tidak ada secret yang pernah masuk git.

## 9. Pemeriksaan keamanan (Fase 6)

- **Secret scan**: guard CI (ci.yaml — pola token/privkey) + scan lokal bersih;
  `deploy/.env` gitignored & tidak pernah di-track.
- **CORS**: api-gateway mengizinkan `GET, POST, OPTIONS` dari `*` — cukup untuk UI
  publik; tidak ada cookie/credential lintas origin.
- **Endpoint mutasi publik** (`/api/control/*`, `/api/lab/run`, `/api/chaos/kill`,
  `/api/demo/play|stop`): dibiarkan publik TANPA auth — keputusan risiko + mitigasi
  + jalur naik terdokumentasi di **ADR D23** (BLUEPRINT.md). Blast radius dibatasi
  mekanis: allowlist chaos, satu duel bersamaan, satu demo aktif, self-heal.
- **Rate limit global**: tidak ada (disengaja, lihat D23); endpoint berat sudah
  self-limit (lab 1 duel @ `cpus: 1.0`).
- **Surface internal**: order-ingestion/consumer/redpanda/postgres/redis TIDAK
  dipublish ke host; hanya api/ws/sim-control di-publish (loopback atau IP gateway
  Caddy), dua yang terakhir hanya di-proxy Caddy subdomain masing-masing.
