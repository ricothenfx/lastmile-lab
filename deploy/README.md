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
{"target":"rider-sim"}` = SIGKILL PID 1 di dalam container (exec API — endpoint
`/kill` Docker TIDAK memicu restart policy) dari **allowlist eksplisit 7 service
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


- Override `deploy/compose.pipeline.yaml` hanya mengubah `rider-sim` →
  `ORDER_SOURCE=pipeline`. Tanpa override itu, demo tetap internal.
- Setelah load test: kembali ke mode demo
  (`docker compose -p lastmile -f deploy/compose.yaml --profile sim up -d`).
- Angka zero-loss & prosedur lengkap: `reports/phase-02-loadtest.md`.
- Metrik pipeline (JSON): `GET :3010/api/metrics` (agregasi ingestion/consumer/
  loadgen) atau per service `/metrics` di jaringan internal.

## 5. Caddy (DRAFT sampai Fase 6)

Caddy produksi = container `turnaround-prod-caddy-1` (ports 80/443), config milik stack
turnaround-prod. Sentuhan yang sah:

1. Salin blok dari `deploy/caddy/lastmile-lab.caddy` ke Caddyfile produksi.
2. Reload TANPA restart: `docker exec turnaround-prod-caddy-1 caddy reload --config <path>`
   (verifikasi path config dengan `docker inspect` dulu).
3. `docker exec turnaround-prod-caddy-1 caddy validate --config <path>` SEBELUM reload.

## 6. DNS (action items pemilik domain)

| Record | Nilai | Kapan |
|---|---|---|
| `lastmile-lab` | CNAME `cname.vercel-dns.com` | Fase 1 (saat deploy Vercel) |
| `api.lastmile-lab` | A → IP VPS | Fase 2 |
| `ws.lastmile-lab` | A → IP VPS | Fase 1 |

## 7. Kesehatan & Ketahanan 24 Jam

- Semua service expose `/healthz` (liveness) — dipantau UptimeRobot eksternal (Fase 6).
- Frontend punya **replay fallback**: backend mati ≠ situs mati.
- Log rotation json-file 10 MB ×3 (sudah di compose) — disk hanya 30 GB bebas.
- Sebelum deploy: cek `cat /proc/loadavg` dan `free -m` — bila load > 6 atau available
  RAM < 500 MB, tunda deploy dan selidiki dulu.

## 8. Sekret

- `deploy/.env` (gitignored): `LASTMILE_PG_PASSWORD`, dst.
- Template: `deploy/.env.example`. Tidak ada secret yang pernah masuk git.
