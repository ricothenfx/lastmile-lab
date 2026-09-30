# Load Test Fase 2 — Zero Message Loss saat Spike ×10

> Tanggal: 2026-09-30 · Host: VPS vmi3585780 (4 core / 8 GB, load host ~7–9 selama tes)
> Stack: `docker compose -p lastmile` profile infra + sim + pipeline + loadtest,
> semua kontainer dengan `mem_limit`/`cpus` eksplisit. Build image lokal via docker
> (BuildKit cache, `GOMAXPROCS=2` di stage build) — tanpa build berat di luar docker.

## 1. Skenario

| Parameter | Nilai |
|---|---|
| Pola trafik | Poisson (jarak antar-datang distribusi eksponensial) |
| Rate dasar | 300 order/menit (5/s) |
| Skenario spike | ×10 (3.000 order/menit = 50/s), terjadwal: t+90s–150s dan t+240s–300s |
| Durasi run | 300 s (`RUN_DURATION_SEC`), `SEED=7` (reproducible) |
| Rider / TTL | 60 rider, TTL antrean simulasi 90 s |
| Idempotency | `Idempotency-Key` unik per pesan (`lg<run>-<seq>`) → mengukur sent/acked murni |
| Pipeline | loadgen → order-ingestion (Redis idem + Postgres + Kafka sync-ack) → topic `orders` (3 partisi, RF 1) → dispatch-consumer (FIFO via `pkg/dispatch`, commit offset manual per batch) → rider-sim `ORDER_SOURCE=pipeline` (injeksi + validasi ulang) |
| Pra-run | `TRUNCATE orders, order_events` + snapshot counter semua lapisan (delta yang dilaporkan) |

## 2. Hasil — Zero Message Loss

Jendela terukur (dalam delta counter; 1 pesan smoke sebelum truncate dikecualikan
secara konsisten di semua lapisan):

| Lapisan | Metric | Nilai |
|---|---|---|
| loadgen | sent (attempt) | 4.850 |
| loadgen | acked 201 | **4.849** |
| loadgen | rejected 429 | **0** |
| loadgen | replay 200 | 0 |
| loadgen | error klien | 1 ⚠ (server 503 — lihat catatan A) |
| loadgen | p50 / p99 latensi POST | 175 ms / 1.903 ms (puncak spike) |
| order-ingestion | published (delta) | 4.849 |
| order-ingestion | publish_errors | 0 |
| dispatch-consumer | consumed (delta) | **4.849** |
| dispatch-consumer | duplicates (lokal/engine) | 0 / 0 |
| dispatch-consumer | parse/sim/db errors | 0 / 0 / 0 |
| Kafka | TOTAL-LAG (`rpk group describe`) | **0** (3 partisi, offset rata) |
| Postgres | baris `orders` | **4.849** (59 `assigned` + 4.790 `received`) |
| Postgres | `order_events` | 4.849 `ingested` + 59 `assigned` (unique per order+type) |
| rider-sim | order ter-inject (`cr` delta) | **4.849** |

**Kesimpulan: sent(acked) = consumed = stored = injected = 4.849 — ZERO MESSAGE LOSS
end-to-end saat spike ×10. Nol 429, nol duplikat, nol lag konsumen.**

Catatan A — 1 dari 4.850 attempt mendapat 503 dari order-ingestion (ketat di bawah
beban puncak). Pesan itu TIDAK pernah masuk pipeline (published delta = acked
delta = 4.849; publish_errors = 0; consumed = 4.849): server menolak secara
eksplisit sebelum publish — kegagalan terlihat klien, bukan pesan hilang diam-diam.
At-least-once tidak dilanggar: setiap pesan yang di-ack 100% tersimpan & ter-consume.

Catatan B — 59 order mendapat rider (60 rider, 44 selesai diantar dalam jendela);
sisa mengantre dan tunduk TTL 90 s. Ini perilaku simulasi (kapasitas armada vs
beban ×10), **bukan** kehilangan pesan: semua order tersimpan di DB dan tercatat
di engine (`cr`).

## 3. Budget RAM (saat spike aktif, t+135s)

| Kontainer | Pemakaian | Limit |
|---|---|---|
| redpanda | 152,5 MiB | 512 MiB |
| postgres | 45,8 MiB | 384 MiB |
| dispatch-consumer | 16,7 MiB | 96 MiB |
| rider-sim | 21,6 MiB | 128 MiB |
| order-ingestion | 11,0 MiB | 96 MiB |
| loadgen | 8,2 MiB | 64 MiB |
| ws-gateway | 8,0 MiB | 64 MiB |
| api-gateway | 3,8 MiB | 64 MiB |
| redis | 9,4 MiB | 64 MiB |
| sim-control | 2,7 MiB | 64 MiB |
| **TOTAL aktual** | **≈ 280 MiB** | |
| **TOTAL limit (worst-case)** | | **1.536 MiB ≤ 2 GB** ✓ |

`free -m` host saat spike: available 2.066 MB (stack lain tidak terganggu).

## 4. Surge Console — reaksi < 1 detik (kriteria pemblokir)

Rantai: klik UI → POST api-gateway `/api/control/*` → sim-control → rider-sim
(`SetControls`) → snapshot berikutnya (10 Hz) → echo `su`/`we` di UI.

Pengukuran API (kontrol POST → nilai baru terlihat di `/api/snapshot`, 5 sampel):
surge ×8 = 242 ms · ×3 = 14 ms · ×1 = 208 ms · weather 0,6 = 119 ms · 1,0 = 420 ms.

Pengukuran UI headless (Chromium 1440×900, sumber `reports/phase-02-ui-verify.mjs`;
echo dibaca dari baris EFFECT yang hanya berisi nilai snapshot — bukan state tombol):

| Aksi | Run 1 | Run 2 |
|---|---|---|
| FLASH SALE →×8 (echo) | 226 ms | 405 ms |
| RAIN on (echo) | 874 ms | 386 ms |
| Slider →×3.5 (echo) | 542 ms | 347 ms |

Semua **< 1 detik** ✓. `console_errors: 0`; `prefers-reduced-motion: reduce` →
halaman tetap LIVE tanpa error. Screenshot: `reports/phase-02-surge-console.png`
(state FLASH ×8 + RAIN aktif, pill RAIN dari echo snapshot).

Catatan fps: angka absolut headless di VPS (13–52 fps, software GL, load host 8+)
terkontaminasi load host — sama seperti Fase 1 (PROGRESS log sesi 2). Angka final
60 fps tetap diverifikasi di laptop fisik (prosedur di PROGRESS.md §Langkah sisa);
budget draw canvas tidak berubah oleh Fase 2 (panel DOM kecil, tanpa rAF baru).

## 5. Idempotency POST /orders

- **Unit test** (`order-ingestion/ingest_test.go`, CI): dua POST key sama → 201
  lalu 200 + `Idempotent-Replay: true`, id sama, tepat 1 publish, 1 baris DB.
  Termasuk: key dari hash body bila header absen, 429 saat antrean penuh, 503
  saat broker down, validasi 400.
- **Live**: `POST /orders` key `smoke-*` → `{"duplicate":false,"id":"o-894f…"}`;
  POST kedua key sama → HTTP 200, header `Idempotent-Replay: true`, id sama.
  Redis `SET NX` TTL 24h.

## 6. Spot-check konsistensi Postgres ↔ snapshot

```
SELECT status, count(*) FROM orders GROUP BY status;  → assigned 59 | received 4790
SELECT event_type, count(*) FROM order_events ...;    → ingested 4849 | assigned 59
rider-sim /internal/state → st.cr = 4850 (4849 + 1 smoke pra-truncate) ✓
```

## 7. Bug yang ditemukan & diperbaiki selama verifikasi live

1. `redpanda v24.2.7`: flag `--check-enabled=false` sudah dihapus → command compose
   diperbaiki (entry Fase 0 baru pertama kali dinyalakan di fase ini).
2. Healthcheck `rpk cluster health` tidak match `Healthy: true` (format kolom
   lebar) → regex `Healthy:\s+true` + `-X brokers=127.0.0.1:19092`.
3. Sintaks `-X brokers` rpk wajib `key=value` (init job crash-loop).
4. franz-go: producer idempotent menuntut acks=all → opsi `LeaderAck` dihapus
   (default idempotent = guarantee lebih kuat untuk zero-loss).
5. loadgen: counter `sent` tidak pernah di-increment; proses tidak keluar setelah
   `RUN_DURATION_SEC` → keduanya diperbaiki (exit bersih via `srv.Shutdown`).
6. sim-control memblokir respons hingga fan-out loadgen selesai (hingga 1 s) →
   fan-out loadgen dibuat async; weather tidak dikirim ke loadgen (bukan target).
7. **Penting**: `created_ms` wall-clock dari pipeline (≈1,79×10¹²) tidak pernah
   melewati TTL jam virtual engine (mulai dari 0) → antrean simulasi menumpuk
   tanpa batas. Fix: clamp `created_ms` ke jam virtual saat injeksi + 2 unit test
   (`TestInjectExternalTTLExpiry` wall-clock, `TestInjectExternalStaleOrderExpiresFast`).

## 8. Cara mereproduksi

```bash
cd /home/rico/portfolio/lastmile-lab
docker compose -p lastmile -f deploy/compose.yaml -f deploy/compose.pipeline.yaml \
  --profile infra --profile sim --profile pipeline up -d
# tunggu semua healthy, lalu (opsional truncate + snapshot counter):
docker exec lastmile-postgres psql -U lastmile -d lastmile -c "TRUNCATE orders, order_events;"
docker compose -p lastmile -f deploy/compose.yaml \
  --profile infra --profile sim --profile pipeline --profile loadtest up -d loadgen
# setelah ~300 s: docker logs lastmile-loadgen | grep LOADGEN_FINAL
# bandingkan: loadgen acked ↔ order-ingestion /metrics ↔ dispatch-consumer /metrics
#             ↔ SELECT count(*) FROM orders ↔ rider-sim st.cr (semua delta harus sama)
```
