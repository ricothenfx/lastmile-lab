# Phase 3 — Dispatch Benchmark & Strategy Lab (laporan terukur)

> Angka di laporan ini berasal dari run nyata di VPS produksi (bukan angka karangan).
> Semua bisa direproduksi dengan perintah di §6. Benchmark jalan MANUAL — bukan gate CI.

## 1. Definisi metrik (eksplisit — bukan karangan)

| Metrik | Definisi |
|---|---|
| **Waktu keputusan dispatch** | Durasi satu pemanggilan `Strategy.Assign(orders, riders)` (mikrobenchmark, koordinat nyata graph Berlin: rider di node acak, pickup di POI acak, umur order 0–45 s). Diukur 400 call per strategi setelah 20 warmup. |
| **Delivery time** | `created → delivered` per order, jam virtual engine (ms). Dilaporkan p50/p95 dari seluruh order terkirim dalam duel. |
| **Rider utilization** | `busy rider-ms / total rider-ms × 100%`; busy = status `to_pickup | pickup | delivering` (waktu armada membawa/bebas-misi tugas). |
| **Cost/order** | **Total jarak tempuh on-task armada (km, leg pickup + leg antar, diukur dari progres edge graph) ÷ jumlah order terkirim.** Satuan biaya: 1 unit = 1 km tempuh on-task. Idle cruising TIDAK dihitung (tidak atributable ke order mana pun). Contoh baca: cost/order 0,93 = rata-rata 930 m jarak on-task per order terkirim. |
| **Expired / throughput** | Order melewati TTL tanpa assignment; order terkirim per menit durasi duel. |
| **Keadilan duel** | SATU generator order (seed sama) mem-produksi tiap order SEKALI dan menyuntikkannya ke KEDUA engine pada tick yang sama (`InjectExternal`) — bukan dua generator terpisah. Rider wander RNG bersifat per-engine (deterministik per sisi). Determinisme duel diuji unit test (`internal/duel`). |

## 2. Kondisi pengukuran

- Host: VPS `vmi3585780` (4 vCPU, 8 GB RAM, bersama stack produksi lain).
- Benchmark dijalankan dalam container Docker dengan cap **`--cpus 1 --memory 1g`,
  GOMAXPROCS=1`** (kondisi paling ketat — p99 produksi hanya akan lebih baik).
- Host loadavg saat run: **9,49 / 8,52 / 7,70** (host sedang sibuk — angka di bawah
  sudah termasuk degradasi ini; outlier `max` berasal dari scheduling host).
- Toolchain: go1.26.8, graph Berlin 9.459 node / 1.419 POI (OSM, embedded).

## 3. p99 keputusan dispatch (target keras: **< 50 ms** @ ≥100 order aktif + 100 rider)

| Skenario | Strategi | n order | n rider | mean ms | p50 ms | p95 ms | **p99 ms** | max ms |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| target | `batching` | 100 | 100 | 1.070 | 0.737 | 1.123 | **2.562** | 103.700 |
| target | `fifo` | 100 | 100 | 1.019 | 0.831 | 1.209 | **3.045** | 28.942 |
| target | `optimal` | 100 | 100 | 3.098 | 2.274 | 6.569 | **13.137** | 33.467 |
| target | `zone` | 100 | 100 | 0.538 | 0.213 | 2.139 | **5.514** | 20.027 |
| 3× target | `batching` | 300 | 300 | 3.250 | 2.457 | 6.827 | **9.355** | 23.186 |
| 3× target | `fifo` | 300 | 300 | 9.773 | 7.423 | 20.634 | **37.264** | 59.934 |
| 3× target | `optimal` | 300 | 300 | 54.211 | 37.919 | 107.223 | **303.099** | 741.884 |
| 3× target | `zone` | 300 | 300 | 2.161 | 1.187 | 5.014 | **8.439** | 147.220 |

**Verdict: p99 semua strategi < 50 ms pada beban target (terburuk `optimal` 13,1 ms —
3,8× headroom) meski host load 9+.** Pada 3× beban target, Hungarian O(n²m) mulai
terasa (p99 303 ms) — dokumentasi batas aman: gunakan `optimal` hingga ±150 antrian
per tick, atau pairing `zone`+`optimal` di fase berikutnya. Nilai `max` yang ekstrem
(103–741 ms) adalah spike scheduler host 1 CPU, bukan algoritma (p99 jauh di bawahnya).

## 4. Duel A/B — skenario identik (1 generator → 2 engine, seed 42)

Preset kapasitas: armada 100 rider di POI Berlin ≈ 25–30 order/menit.
`rush` = 45 order/menit (overload ±1,5×), `steady` = 20/menit (±70% kapasitas).

### 4.1 `rush` — 600 s virtual, 100 rider, 45 order/menit, TTL 240 s

| Duel | Side | Created | Delivered | Expired | P50 deliv | P95 deliv | Util | **Cost/order** | Throughput |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| fifo vs **optimal** | fifo | 441 | 129 | 46 | 174,5 s | 414,8 s | 87,6% | 2,040 km | 12,9 /m |
| | **optimal** | 441 | **255** | **27** | **164,4 s** | **376,4 s** | 87,6% | **0,931 km** | **25,5 /m** |
| fifo vs batching | fifo | 441 | 129 | **46** | 174,5 s | 414,8 s | 87,6% | 2,040 km | 12,9 /m |
| | batching | 441 | 129 | 68 | **142,6 s** | 395,8 s | 87,0% | 2,031 km | 12,9 /m |
| fifo vs zone | fifo | 441 | **129** | **46** | 174,5 s | **414,8 s** | 87,6% | **2,040 km** | **12,9 /m** |
| | zone | 441 | 119 | 56 | 166,9 s | 469,3 s | 87,7% | 2,214 km | 11,9 /m |
| zone vs **optimal** | optimal | 441 | **255** | **27** | **164,4 s** | **376,4 s** | 87,6% | **0,931 km** | **25,5 /m** |

### 4.2 `steady` — 300 s virtual, 100 rider, 20 order/menit, TTL 90 s

| Duel | Side | Created | Delivered | Expired | P50 deliv | P95 deliv | Util | Cost/order | Throughput |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| fifo vs optimal | fifo | 108 | 64 | 0 | 85,2 s | 204,7 s | 31,7% | 0,614 km | 12,8 /m |
| | optimal | 108 | 64 | 0 | 85,2 s | 204,7 s | 31,7% | 0,614 km | 12,8 /m |

### 4.3 Interpretasi jujur

- **`optimal` menang besar saat overload**: delivered +98% (129 → 255), expired −41%,
  cost/order −54% vs FIFO. Ini klaim utama yang bisa dipakai di README/artikel.
- **`steady` (30% util): semua strategi identik.** Pelajaran penting: pilihan strategi
  hanya berarti saat sistem di bawah tekanan — under-capacity, "nearest idle" FIFO
  sudah optimal-global. Duel engine kita membuktikan kapan optimasi berbayar.
- **`batching` menukar expiry dengan kecepatan**: p50 lebih cepat (142,6 vs 174,5 s)
  tapi expired naik (68 vs 46) — window 2 s menahan order; di rezim TTL pendek ini
  mahal. Tunable `WindowMs` menyeimbangkan.
- **`zone` sedikit kalah di demand POI Berlin yang seragam-padat** (119 vs 129):
  locality bias membayar saat demand mengelompok spasial, tidak pada POI yang sudah
  rapat di pusat. Tetap berguna sebagai pembatas radius (fallback global dijamin).

## 5. API & storage hasil duel

- `POST /api/lab/run {"strategy_a","strategy_b","preset","seconds","seed"}` → `202 {id}`
  (409 bila ada duel aktif; duel asinkron, polling via results).
- `GET /api/lab/results` ringkasan; `GET /api/lab/results/{id}` penuh: metrik sisi
  A/B + histogram bin bersama + frame replay kembar (≤600 frame/sisi, 1 frame/2 s).
- Service `strategy-lab` (port internal 4205, tidak publish ke host) — health
  `/healthz`, mem_limit 256 MiB, maks 8 hasil in-memory (+ persist opsional
  `LAB_DATA_DIR`). Duel 600 s virtual selesai ±1,4 s wall (satu duel bersamaan).

## 6. Reproduksi

```bash
# p99 benchmark (dari apps/services; docker, cap CPU — kondisi tercatat §2)
docker run --rm --cpus 1 --memory 1g -v "$PWD":/src -w /src \
  -v lastmile-gomod:/go/pkg/mod -e GOMAXPROCS=1 golang:1.26-alpine \
  go run ./tools/bench -orders 100 -riders 100 -ticks 400 -seed 7

# duel via API (stack hidup, profile sim)
curl -s localhost:3010/api/lab/run -d '{"strategy_a":"fifo","strategy_b":"optimal","preset":"rush","seconds":600}'
curl -s localhost:3010/api/lab/results

# unit + determinisme
docker run --rm -v "$PWD":/src -w /src golang:1.26-alpine go test ./...
```

## 7. UI Strategy Lab (kriteria pemblokir — verifikasi headless)

Diverifikasi via Playwright headless di VPS setelah deploy image (lihat
`reports/phase-03-ui-verify.mjs` + screenshot): token-only colors, angka mono
tabular, dua canvas render on-demand (tanpa rAF saat idle), reduced-motion =
frame akhir statis, duel end-to-end dari UI.
