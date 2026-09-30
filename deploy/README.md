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
        │  4201-4204 internal services (tidak publish ke host)   │
        │  redpanda 19092 (internal network saja)                │
        └────────────────────────────────────────────────────────┘
```

## 2. Blok Port (terdaftar di /home/rico/PORTS.md)

| Port | Pemilik | Bind | Catatan |
|---|---|---|---|
| 3010 | lastmile-api-gateway | 127.0.0.1 | via Caddy api.* |
| 3012 | lastmile-ws-gateway | 127.0.0.1 | via Caddy ws.* (WebSocket) |
| 3013 | lastmile-sim-control | 127.0.0.1 | kontrol simulasi/chaos |
| 3030 | lastmile-grafana (opsional) | 127.0.0.1 | dashboard internal |
| 4201–4204 | dispatch/rider-sim/loadgen/replay | tidak dipublish | jaringan docker internal |
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
