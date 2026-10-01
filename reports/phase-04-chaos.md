# Fase 4 — Laporan Eksperimen Chaos & KPI (phase-04-chaos.md)

> Eksperimen chaos nyata pada stack produksi `lastmile` di VPS (bukan simulasi):
> node di-kill saat trafik berjalan, self-heal diukur dari restart policy Docker,
> MTTD/MTTR/error budget dihitung dari incident yang direkam service `chaos`.

## 1. Definisi (eksplisit, dipakai konsisten di seluruh laporan)

- **t_start** — `chaos-kill`: saat permintaan kill diterbitkan ( respons 202 );
  `health`: timestamp healthz terakhir yang SUKSES sebelum kegagalan teramati.
- **t_detect** — timestamp probe healthz pertama yang GAGAL setelah t_start
  (monitor 1 Hz per target, timeout probe 1,5 s, dijalankan service `chaos`).
- **t_recover** — timestamp probe healthz pertama yang SUKSES setelah t_detect.
- **MTTD = t_detect − t_start** (seberapa cepat kegagalan *dideteksi*).
- **MTTR = t_recover − t_detect** (seberapa cepat *dipulihkan* — di sini murni
  self-heal Docker: chaos tidak pernah me-restart manual).
- **Kill = SIGTERM ke PID 1 di dalam container** via Docker exec API (perintah
  fixed `kill -TERM 1`). Dua fakta Docker 29.8.1 yang menentukan desain ini
  (lihat ADR D19 rev. 3): endpoint `/containers/{id}/kill` TIDAK memicu restart
  policy (dianggap stop manual), dan PID 1 kebal SIGKILL dari dalam PID
  namespace-nya sendiri. SIGTERM mengenai handler runtime Go → proses exit →
  restart policy `unless-stopped` yang benar-benar memulihkan.
- **Error budget** — SLO availability ≥ 99,9% → budget downtime 0,1% dari jendela
  pengukuran. Availability = 1 − Σdowntime / (window × jumlah target yang pernah
  hidup di jendela itu; target standby tidak menghukum angka).
- **SLO fase 4** (dievaluasi live per polling di `GET /api/kpi`, sumber angka
  metrik nyata, bukan dekorasi):
  1. `delivery_p95` — p95 durasi created→delivered < 360 s (target spec fase 4;
     dari ring 512 delivery terakhir engine).
  2. `zero_loss` — lag topik 0 dan error publish/db 0 (hanya terukur saat
     pipeline hidup; `null` → UI "—").
  3. `grid_health` — semua service core (rider-sim, ws-gateway, api-gateway) up.
  4. `availability` — ≥ 99,9% pada jendela monitor chaos (budget 0,1%).

## 2. Kondisi Eksperimen

| Hal | Nilai |
|---|---|
| Host | VPS vmi3585780 — 4 core / 8 GB RAM (dipakai stack lain juga) |
| Load host saat eksperimen | loadavg 1-min 6,4–7,7 (aturan: jangan > 8 — dipatuhi) |
| Docker | 29.8.1, Compose v5.5.1, project `-p lastmile` |
| Stack demo (E1–E3) | rider-sim, ws-gateway, api-gateway, strategy-lab, chaos — semua `restart: unless-stopped` + healthz |
| Stack E4 | demo + postgres, redis, redpanda (topic `orders` 3 partisi), order-ingestion, dispatch-consumer, sim-control, loadgen (Poisson 300/menit, spike ×10 60 s tiap 90 s, seed 7) |
| Trafik E4 | sent 5 460 = acked 5 460, error 0, 429 = 0, p50 POST 52 ms, p99 1 309 ms, durasi 300 s |
| Metode pengukuran | service `chaos` :4206 (monitor 1 Hz) → `GET /api/chaos/incidents`; kill via `POST /api/chaos/kill` (allowlist 7 container stateless `lastmile-*`, sisanya 403) |

## 3. Hasil — kill node saat trafik (4 eksperimen)

| # | Target | Mode | MTTD | MTTR | Kill→pulih |
|---|---|---|---|---|---|
| E1 | rider-sim | demo (trafik internal Poisson) | **811 ms** | **2 035 ms** | 2,85 s |
| E2 | ws-gateway | demo (browser live) | **1 295 ms** | **1 986 ms** | 3,28 s |
| E3 | api-gateway | demo (UI polling /api/kpi 2 Hz) | **1 139 ms** | **3 017 ms** | 4,16 s |
| E4 | dispatch-consumer | pipeline @300/menit + spike ×10 | **1 073 ms** | **1 982 ms** | 3,06 s |

Rata-rata: **MTTD 1,08 s · MTTR 2,26 s** (murni self-heal restart policy Docker —
tidak ada intervensi manual; RestartCount container naik + StartedAt baru
diverifikasi via `docker inspect`).

Interpretasi:
- **MTTD ~0,8–1,3 s** = batas bawah 1 periode monitor (1 Hz) + stagger probe —
  deteksi bekerja secepat yang bisa dicapai monitor ini.
- **MTTR ~2–3 s** = backoff restart Docker (~100 ms) + boot service + graph
  Berlin termuat (rider-sim/strategy-lab paling lambat) + healthz hijau.
- E3 (api-gateway mati): KPI Command Deck kehilangan sumber → kartu "—"
  (bukan angka karangan), Incident tetap terekam service chaos, setelah pulih
  polling 2 Hz lanjut sendiri. Verifikasi UI headless: `reports/phase-04-ui-verify.mjs`
  (`console_errors: 0`, layout 1440/1024/768 ok).
- E2 (ws-gateway mati): frontend kehilangan WS → fallback replay fase 1 bekerja
  (banner REPLAY), saat pulih otomatis kembali live.

## 4. E4 — Zero message loss saat consumer di-kill

Kronologi (semua angka dari counter service + Postgres, bukan perkiraan):

1. Trafik stabil: `published == consumed == 2 440`, lag 0 (74 s sejak loadgen start).
2. `POST /api/chaos/kill {"target":"dispatch-consumer"}` pada t0.
3. Selama downtime (~2 s): published terus naik (trafik 5/menit × 60... pesan
   tertahan di topik `orders`), tidak ada pesan hilang — at-least-once +
   idempotency tiga lapis (LRU consumer, seen-set engine, unique index DB).
4. Self-heal t0+1,07 s terdeteksi, t0+3,05 s sehat (MTTR 1 982 ms) → consumer
   resume dari offset ter-commit, backlog 2 s langsung terkuras.
5. Setelah loadgen selesai (300 s, sent 5 460 = acked 5 460, 0 error, 0 kalau
   429) dan topik tenang:
   - DB: **5 460 baris `orders` dibuat pada jendela run == 5 460 ack** —
     `SELECT count(*) FROM orders WHERE created_ms BETWEEN <start> AND <end>` = 5 460.
   - `order_events`: 10 550 ingested == 10 550 baris orders (semua order punya
     event; angka 10 550 mencakup total akumulasi tabel dari fase 2).
   - Counter consumer pasca-restart: consumed +3 015 tanpa `parse_errors`,
     `sim_errors`, `db_errors` (semua 0).
   - Topik habis: `consumed` diam (delta 0 dalam 12 s).
6. Catatan jujur: counter `consumed` per-proses — setelah restart angkanya
   mulai dari 0, sehingga "lag = published − consumed" HANYA valid dalam satu
   umur proses consumer. Laporan ini memakai DB sebagai sumber kebenaran lintas
   restart; keterbatasan metrik ini tercatat sebagai pekerjaan Fase 6
   (lag sejati = high-watermark − committed offset).

Kesimpulan E4: **zero message loss** terjaga melalui kill consumer saat trafik —
SLO `zero_loss` terpenuhi.

## 5. Error budget (jendela eksperimen)

Capture pada jendela monitor 799 s (session eksperimen E1–E4 + teardown):

| Metrik | Nilai |
|---|---|
| Incident chaos-kill | 4 |
| Total downtime terukur | 17,1 s |
| Availability | **99,69%** |
| Budget (0,1%) | **terlampaui** selama jendela eksperimen |

Budget sengaja "gagal" di sini: 4 kill dalam 13 menit adalah kondisi yang jauh
lebih hostile dari SLO 99,9% bulanan (setara ~43 menit downtime/bulan).
Angka yang jujur justru menunjukkan sistem pengukurannya bekerja: incident
terbuka/tertutup otomatis, budget terhitung dari data, dan UI menandai
`BUDGET EXCEEDED` (merah) tanpa disembunyikan. Pada operasi normal tanpa chaos,
downtime 0 → availability 100%.

Catatan teardown: menghentikan pipeline setelah E4 membuka 3 incident `health`
(order-ingestion, dispatch-consumer, sim-control — mati terencana, status OPEN
sampai pipeline dinyalakan lagi; standby tanpa baseline tidak pernah membuka
incident). Ini perilaku benar: monitor merekam realita, bukan menyembunyikannya.

## 6. Verifikasi UI (kriteria pemblokir fase 4)

`reports/phase-04-ui-verify.mjs` (headless chromium, container
`mcr.microsoft.com/playwright:v1.63.0-noble --network host`, app lokal memakai
WS/API 172.19.0.1). Hasil run final:

- KPI Command Deck LIVE — angka kartu == `GET /api/kpi` (p99 dispatch 0,21 ms,
  4 SLO, gauge terpasang), streaming chart canvas tergambar (on-data, tanpa rAF).
- System Health: canvas pipeline tergambar + partikel bergerak (bukti piksel),
  grid node tampil, health events mengalir.
- Chaos E2E dari UI: tombol KILL dua langkah (arm → CONFIRM) → kill
  strategy-lab → incident `chaos-kill` **MTTD 1 008 ms · MTTR 1 994 ms** tampil
  di timeline (RECOVERED), service kembali `healthy` di grid.
- rAF audit (deterministik): canvas pipeline bergerak saat tab terbuka (bukti
  piksel), UNMOUNT saat tab diganti & konten panel unmount saat ditutup
  (cleanup React membatalkan rAF — tanpa rAF permanen baru). Hitungan
  informatif: 88/2 s terbuka vs 42–72/2 s baseline map (berfluktuasi ikut
  beban host).
- reduced-motion: pipeline statis tergambar, tanpa getaran.
- Layout 1440/1024/768 tanpa tumpang tindih panel; console error 0.
- Bukti gambar: `reports/phase04-deck-live.png`, `phase04-chaos-recovered.png`,
  `phase04-final.png`, `phase04-reduced.png`, `phase04-1024.png`, `phase04-768.png`.

## 7. Repro

```bash
# 0. stack + chaos (di VPS, /home/rico/portfolio/lastmile-lab/deploy)
docker compose -p lastmile --profile sim --profile chaos up -d
cat /proc/loadavg   # eksperimen hanya saat load 1-min < 8

# 1. kill node (target: rider-sim|ws-gateway|api-gateway|sim-control|
#    order-ingestion|dispatch-consumer|strategy-lab — lainnya 403)
curl -s localhost:3010/api/chaos/kill -X POST \
  -H 'Content-Type: application/json' -d '{"target":"rider-sim"}'

# 2. baca MTTD/MTTR + budget
curl -s localhost:3010/api/chaos/incidents | python3 -m json.tool

# 3. KPI live (SLO + grid + budget)
curl -s localhost:3010/api/kpi | python3 -m json.tool

# 4. E4 (pipeline + beban)
docker compose -p lastmile -f compose.yaml -f compose.pipeline.yaml \
  --profile infra --profile sim --profile pipeline --profile chaos up -d
docker compose -p lastmile -f compose.yaml -f compose.pipeline.yaml \
  --profile infra --profile sim --profile pipeline --profile loadtest \
  --profile chaos up -d loadgen
# kill dispatch-consumer saat load (lihat langkah 1), lalu:
#   curl localhost:3010/api/metrics          # counter pipeline
#   docker exec lastmile-postgres psql -U lastmile -d lastmile -c \
#     "SELECT count(*) FROM orders WHERE created_ms BETWEEN <t0> AND <t1>;"
#   # == acked_201 loadgen (LOADGEN_FINAL di docker logs lastmile-loadgen)

# 5. kembali mode demo
docker compose -p lastmile --profile sim --profile chaos up -d
```

## 8. Tuning demo agar SLO steady terpenuhi

Konfigurasi demo lama (60 rider vs demand 20/menit) terukur DI ATAS kapasitas
armada — p95 delivery merambat > 6 menit sehingga SLO steady dari spec tidak
pernah mungkin terpenuhi (expired 41% dari order). RIDERS dinaikkan 60 → 100
(kapasitas terukur fase 3: ±25–30 order/menit untuk 100 rider). Hasil setelah
ekuilibrium ±6 menit di stack live: p50 99 s · **p95 197 s < 360 s (SLO OK)** ·
expired 0 · util 40% · idle 41 · p99 dispatch 0,19 ms · cost 0,69 km/order.
Pelajaran KPI Deck: angka yang jujur membuat mis-konfigurasi terlihat — kartu
p95 "merah" adalah apa yang menunjukkan masalah ini.

## 9. Deny-list (uji negatif — di unit test & runtime)

`POST /api/chaos/kill` untuk `postgres`, `redis`, `redpanda`, `chaos`, `loadgen`,
`db-migrate`, `redpanda-init`, nama container stack lain (`turnaround-prod`,
`aviation`, `ro-botriv`), wildcard `*`, atau nama dengan spasi/huruf besar →
**HTTP 403 `target_outside_allowlist`**. Unit test: `apps/services/chaos/chaos_test.go`
(`TestAllowlistDeny`, `TestKillHandlerValidation`).
