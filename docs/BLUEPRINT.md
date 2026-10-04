# BLUEPRINT — lastmile-lab

> Sumber kebenaran visi & arsitektur. Setiap keputusan arsitektur baru wajib dicatat di
> sini sebagai ADR (Architecture Decision Record) ringkas dengan tanggal.

---

## 1. Ringkasan Eksekutif

lastmile-lab adalah platform simulasi operasi last-mile delivery yang:

1. **Menyerang masalah inti bisnis perusahaan delivery** (biaya rider, janji waktu tempuh,
   ketahanan saat order surge) dengan simulasi kota nyata (OpenStreetMap).
2. **Terukur**: setiap klaim disertai angka dari benchmark yang bisa direproduksi
   (p99 latency, throughput, delivery time, utilization, cost per order).
3. **Tampil kelas produksi**: control-room dashboard gelap dengan peta hidup 60fps —
   bukan demo tutorial.
4. **Tahan banting**: replay mode membuat situs tetap hidup bahkan saat backend mati.

Dikembangkan bertahap (fase 0–7), setiap fase berakhir dengan demo yang jalan + indah,
commit dan push. Source of truth = repo ini, bukan percakapan chat.

## 2. Dasar Riset (mengapa dibangun seperti ini)

Riset terhadap Delivery Hero (careers portal, JD aktual, GitHub org) — Sep 2026:

| Temuan | Bukti | Implikasi ke project |
|---|---|---|
| Tech stack backend: Java/Kotlin/Go, microservices HA | JD "Sr Software Engineer Backend" | Services ditulis dalam **Go**; pola microservices + idempotency |
| Redis, MySQL/Mongo, AWS, observability eksplisit | JD yang sama | Stack memakai Redis + Postgres + metrics pipeline (Prometheus-compatible) |
| K8s/Helm/Argo/Atlantis/Grafana-Loki-Cortex | GitHub org `deliveryhero` (helm-charts 566★, dsb.) | Deploy skeleton docker compose dulu, manifest K8s sebagai artefak bonus portofolio |
| Infra AI agents aktif (`asya` — actor mesh utk AI agents, Python) | Repo aktif Sep 2026 | Fase 7 AI Ops Copilot — sebagai plugin, bukan core |
| Leadership Principles: Dive Deep ("Challenge the Data"), Deliver Value Fast, Own It | careers portal | Setiap fitur harus punya angka; demo interaktif; no fake metrics |
| Domain bisnis: order → vendor → rider → customer, quick commerce 15-min | 749 lowongan, 50 Logistics, 29 Data | Simulasi dispatch + surge + replay = jantung project |

**Kredibilitas angka** hanya dari 5 sumber: (1) load-test sebelum/sesudah, (2) simulasi
dengan data publik (OSM), (3) eksperimen chaos/SRE terukur, (4) evaluasi model vs baseline,
(5) kontribusi OSS. Tidak ada angka karangan.

## 3. Prinsip Sistem

1. **Core deterministik, tanpa LLM.** Keputusan operasional realtime (assign rider→order)
   adalah optimisasi kombinatorial: butuh < 50 ms, deterministik, explainable. LLM di
   posisi itu = salah alat.
2. **AI sebagai lapisan planning/penjelasan** (Fase 7, plugin): LLM *mengusulkan* Plan A/B/C
   → **simulator menjadi hakim** (dry-run tiap plan, keluar angka prediksi) → manusia
   memutuskan → eksekusi live. Human-in-the-loop. Tanpa API key, fitur tersembunyi,
   aplikasi tetap utuh (pola adapter).
3. **Semua interaktif, bukan video pasif.** Reviewer bisa menarik slider surge, menyalakan
   hujan, membunuh node — sistem bereaksi realtime.
4. **Angka selalu bisa ditelusuri** ke metrik internal yang terekspos via API.

## 4. Arsitektur Target

```mermaid
flowchart LR
    subgraph INJ["Event & Load Layer"]
        LG[load-generator]
        SC[sim-control API]
        CH[chaos injector]
    end
    subgraph CORE["Core Services (Go)"]
        GW[api-gateway :3010]
        OI[order-ingestion]
        K[Redpanda - Kafka API :19092 internal]
        DE[dispatch-engine]
        RS[rider-simulator]
        RG[ws-gateway :3012]
    end
    subgraph ST["State"]
        RD[(Redis :6380)]
        PG[(Postgres :5434)]
    end
    UI[Frontend Next.js - Vercel]
    LG-->OI
    SC-->RS
    OI-->K-->DE
    DE<-->RD
    DE-->RS
    RS-->RG
    OI-->PG
    RG-- wss -->UI
    GW-- https -->UI
    CH-.->CORE
```

Komponen & tanggung jawab:

| Komponen | Bahasa | Tanggung jawab |
|---|---|---|
| `order-ingestion` | Go | API penerima order, idempotency (Redis), backpressure, publish ke Kafka |
| `dispatch-engine` | Go | 4 strategi assign: FIFO, Batching, Zone-based, Optimal (Hungarian/OR-Tools) |
| `rider-simulator` | Go | Rider bergerak di road network OSM (OSRM/Valhalla graph), event cuaca/macet |
| `ws-gateway` | Go | Streaming posisi rider/order/KPI ke frontend via WebSocket |
| `api-gateway` | Go | REST untuk KPI, hasil benchmark, replay metadata, health |
| `sim-control` | Go | API kontrol simulasi: preset Golden Demo, slider surge, chaos (Fase 4+) |
| `load-generator` | Go | Trafik sintetis (Poisson rush-hour), skenario spike ×10 |
| `apps/web` | TS/Next.js | Control room: Live Map, Surge Console, KPI Deck, Strategy Lab, System Health, Replay |

Dispatch tetap di-balik interface `Strategy` — strategi baru bisa ditambah tanpa menyentuh
pipeline. Hasil assignment disertai alasan (explainability) untuk panel Replay.

## 5. Keputusan Arsitektur (ADR ringkas)

| # | Tanggal | Keputusan | Alasan |
|---|---|---|---|
| D1 | 2026-09-30 | Backend **Go** (bukan Kotlin/Java) | Match JD target; ringan di RAM (VPS 8 GB penuh); single binary di image kecil |
| D2 | 2026-09-30 | **Redpanda** bukan Kafka (API kompatibel) | RAM ~½–¼ Kafka JVM; VPS hanya sisa ~1–2 GB. Cerita "Kafka" tetap valid |
| D3 | 2026-09-30 | Peta **MapLibre GL + deck.gl** | Open source, tanpa API key/billing — demo tidak bisa mati karena kuota |
| D4 | 2026-09-30 | Frontend **Next.js + TypeScript + Tailwind + Framer Motion** | Ekosistem matang; Vercel native; motion primitives untuk 60fps |
| D5 | 2026-09-30 | **Postgres & Redis instance sendiri** (5434/6380) | Jangan meminjam 5433/6379 milik aviation — coupling = risiko 24h uptime |
| D6 | 2026-09-30 | Frontend **Vercel**; backend **VPS + Caddy** (`api.` & `ws.` subdomain) | Vercel = tidak tidur + preview per fase; backend heavy stack tidak muat di free tier pihak ketiga; Caddy milik turnaround-prod cuma ditambah site block + reload |
| D7 | 2026-09-30 | Build di **GitHub Actions → GHCR → VPS pull** | Load VPS sudah 2x oversubscribed; build di VPS bisa mengganggu produksi lain |
| D8 | 2026-09-30 | LLM **bukan bagian core**; Fase 7 plugin | Lihat Prinsip #1–2; demo Golden Demo harus selalu jalan offline |
| D9 | 2026-09-30 | **Replay mode** wajib sejak Fase 5 | Anti-halaman-mati saat reviewer datang di waktu apa pun |
| D10 | 2026-09-30 | Penamaan netral `lastmile-lab` | Portabel ke semua perusahaan sejenis (DH, Wolt, Deliveroo, Bolt, Uber Eats, Getir) |
| D11 | 2026-09-30 | Isolasi VPS: blok port sendiri, limit CPU/RAM per container, log rotation | Lihat `deploy/README.md`; registry di `/home/rico/PORTS.md` |
| D12 | 2026-09-30 | Peta **tanpa tile provider eksternal** — style MapLibre self-hosted dari ekstrak OSM (roads/water GeoJSON di repo, generator `tools/graphgen`) | Nol API key/kuota/billing (memperkuat D3); visual 100% konsisten token; ekstrak inner-city Berlin 9.459 node / 18.571 edge cukup untuk demo & sim |
| D13 | 2026-09-30 | Engine simulasi **virtual-clock deterministik** (`Tick(dt)`, seed RNG) di `internal/sim`; ws-gateway & api-gateway konsumsi snapshot via HTTP internal 10 Hz (polling), bukan langsung import | Sim, fixturegen, dan test = kode yang sama; kontrak `model.Snapshot` stabil sehingga Fase 2 bisa ganti sumber order ke Redpanda tanpa sentuh frontend; polling 10 Hz cukup (client di-interpolasi 60 fps) |
| D14 | 2026-09-30 | Replay fallback sejak Fase 1 = **fixture rekaman nyata** dari simulasi (`tools/fixturegen`, seed tetap, 5 Hz) yang di-commit; frontend memutar loop + banner saat WS mati | Anti-halaman-mati sejak hari pertama (D9 dipercepat dari Fase 5); data replay bukan karangan — direkam dari engine yang sama |
| D15 | 2026-09-30 | **Topologi pipeline Fase 2**: ingestion (Redis idem + Postgres + ack Kafka sync) → topic `orders` 3 partisi → dispatch-consumer menjalankan `Strategy` bersama (`pkg/dispatch`, dipindah dari internal/sim tanpa refactor frontend) → **injeksi assignment ke rider-sim via HTTP internal** (`POST /internal/orders`), bukan consumer Kafka kedua di rider-sim; engine memvalidasi ulang & fallback FIFO internal; dedupe tiga lapis (consumer LRU, engine seen-set, unique index DB) | rider-sim tetap ringan & bisa jalan tanpa broker (`ORDER_SOURCE=internal` default → demo tidak pernah mati); at-least-once aman; Fase 3 cukup menambah Strategy di satu tempat |
| D16 | 2026-09-30 | Observability Fase 2 = **metrik JSON** (`/metrics` per service + agregat `/api/metrics` di api-gateway) + laporan load test di `reports/`; Grafana ditunda ke Fase 4. **DITINJAU ULANG di Fase 4 (2026-09-30): tetap metrik JSON, Prometheus/Grafana TIDAK dipasang** | Opsi spec fase 2 saat RAM sempit (host load 8+, Grafana +Prometheus ≈ 300–400 MB di atas stack); angka zero-loss tetap terukur & terdokumentasi. Review Fase 4: limit stack sudah 1.856 MiB (chaos +64 MiB); menambah Prometheus (:9091, realistis ≥128 MiB) + Grafana (:3030, ≥192 MiB) mendorong total ≥ 2.176 MiB → **melanggar anggaran keras ≤ 2 GB**. KPI Command Deck + System Health di UI memenuhi kebutuhan observability fase ini (angka dari metrik nyata), jadi Grafana/Prometheus tidak memberi nilai tambah yang sepadan dengan risiko RAM VPS 8 GB yang dipakai stack lain |
| D17 | 2026-09-30 | Strategi `optimal` = **Hungarian/Jonker-Volgenant murni Go** O(n²m) di `pkg/dispatch` — BUKAN OR-Tools | OR-Tools = binding C++/CGO: image membengkak, build berat, dependency risiko; JV cukup untuk n ≤ 500 (p99 terukur 13,1 ms @ 100×100, `reports/phase-03-bench.md`); deterministik & bebas dependensi (prinsip "core deterministik tanpa LLM" tetap murni Go) |
| D18 | 2026-09-30 | **Strategy Lab** = service `strategy-lab` :4205 (internal saja) menjalankan duel: SATU generator order (seed sama) di-pipe ke DUA engine identik per tick — bukan dua generator; metrik level engine (delivery dur, km on-task, rider-ms busy) ditambah di `internal/sim` tanpa mengubah kontrak `model.Snapshot`; hasil via REST `/api/lab/*` (proxy api-gateway), frame replay pakai bentuk `model.Snapshot` yang sama | Keadilan A/B = input byte-identical; kontrak snapshot tidak breaking (frontend fase 1–2 aman); reuse wire format = renderer peta kembar murah; satu duel bersamaan + RAM 256 MiB = ramah VPS 4-core |
| D19 | 2026-09-30 (rev. 3) | **Chaos injector** = service `chaos` :4206 (internal saja, profile compose `chaos`) berbicara ke Docker Engine API via unix socket: kill = **SIGTERM ke PID 1 di dalam container via Docker exec API** (perintah exec FIXED `kill -TERM 1`; runtime Go keluar exit 2 → node mati; bukan exec arbitrer) dari **allowlist eksplisit 7 service stateless `lastmile-*`** (rider-sim, ws-gateway, api-gateway, sim-control, order-ingestion, dispatch-consumer, strategy-lab) — postgres/redis/redpanda/chaos sendiri dan semua container di luar project DITOLAK (403); pemulihan TIDAK pernah manual — yang diukur justru restart policy `unless-stopped` Docker (self-heal); monitor healthz 1 Hz + flap suppression 2 kegagalan beruntun membuka/menutup incident {t_start, t_detect, t_recover}; incident persist ke volume `chaos_data`. **Rev. 3 (2026-10-01), dua fakta Docker 29.8.1 terverifikasi di VPS**: (1) endpoint `/containers/{id}/kill` TIDAK memicu restart policy (container dianggap dihentikan manual — exit 137 tanpa restart); (2) PID 1 kebal SIGKILL dari dalam PID namespace-nya sendiri (kernel membuang sinyal tanpa handler), tapi SIGTERM mengenai handler runtime Go → proses keluar → restart policy `unless-stopped` memulihkannya (terverifikasi: StartedAt baru + Restarts naik) | Spec fase 4 mensyaratkan allowlist eksplisit tanpa wildcard + DILARANG menyentuh stack lain; docker.sock = hak istimewa besar → dibatasi: satu-satunya service lastmile dengan mount itu, berjalan root (akses socket), tanpa publish port, allowlist tervalidasi kode + unit test (deny postgres/stack lain/wildcard), perintah exec fixed |
| D20 | 2026-09-30 | **KPI live** = instrumentasi ring fixed-cap (512 sampel) di `internal/sim`: durasi delivery created→delivered + wall-clock di sekitar `Strategy.Assign` (p50/p95 delivery, p99 dispatch), diekspos `Engine.KPI()` → rider-sim `/internal/metrics` → api-gateway `/api/kpi` (agregasi + SLO eksplisit + grid healthz cache 2 s + error budget dari chaos). Kontrak `model.Snapshot` & `Engine.Metrics()` tidak berubah | Spec fase 4: KPI dari metrik nyata tanpa fitur baru di core — ring adalah instrumentasi (tidak masuk RNG/logika; determinisme same-seed tetap diuji), bukan fitur; ring fixed-cap = memori O(1) dan semantik "performa terkini" (bukan rata-rata sepanjang umur); polling 2 Hz di-cache 400 ms agar upstream tidak dihajar; angka yang tidak terukur = null → UI "—" |
| D21 | 2026-10-01 | **Replay engine** = perekam sesi di rider-sim (`internal/replay`): goroutine terpisah menarik `FullSnapshot()` 5 Hz ke ring berbudget (4 500 frame = 15 menit + pagar 24 MiB gzip + cap 8 000 keputusan dispatch), tiap snapshot di-marshal sekali & dikompresi gzip per frame; dump dilayani sebagai **SATU anggota gzip tunggal** (dekompresi-rekompresi streaming saat dilayani) via proxy api-gateway `/api/replay/*`. Engine TIDAK disentuh; kontrak `model.Snapshot` tidak berubah; rider-sim 128→160 MiB (stack 1 888 MiB ≤ 2 GB) | Replay = fallback & alat demo tanpa menyentuh core deterministik (prinsip "core tanpa LLM" tetap); gzip per frame = RAM ring 9–12 MiB (bukan 60 MB); **satu anggota gzip wajib karena Chromium menghentikan stream multi-member setelah anggota pertama** (curl/Go menerima multi-member — ditemukan saat verifikasi, fix `65daada` + unit test assert 1 anggota); keputusan dispatch ikut direkam supaya inspect rider menampilkan ALASAN keputusan pada titik waktu scrub |
| D22 | 2026-10-01 | **Golden Demo** = orchestrator kecil di api-gateway (`demo.go`, narasi hard-coded satu file): 3 preset ±90 s berlangkah ber-waktu yang HANYA memakai aktuator existing — surge/weather via sim-control, kill via chaos injector (incident tetap satu sumber kebenaran, ADR D19); endpoint `/api/demo/presets|play|stop|state`; UI launcher dock + banner narasi + STOP, mati saat backend offline; sim-control ikut profile `sim` (RAM sudah terhitung) | Demo harus selalu konsisten & bisa direproduksi (narasi ±90 s, langkah tepat di detik yang sama — terverifikasi log: t+0.3/12/36/54/64/74/86, selesai t+90.0s); tanpa jalur kode baru di engine & tanpa LLM (prinsip inti); service baru = nol port/image/limit tambahan; langkah kill yang gagal (timeout/409) ditandai `skipped` tanpa menggagalkan narasi |
| D23 | 2026-10-01 | **Endpoint mutasi dibiarkan publik TANPA auth** (keputusan risiko fase 6): `POST /api/control/{surge,weather}`, `POST /api/lab/run`, `POST /api/chaos/kill`, `POST /api/demo/{play,stop}` dapat dijangkau internet via `api.lastmile-lab.ricothen.com`. **Risiko residu**: pihak ketiga bisa memicu kill service (pulih sendiri, MTTR terukur 2,0–3,0 s), mengubah surge/weather, atau men-start duel CPU. **Mitigasi yang sudah melekat**: chaos = allowlist eksplisit 7 service stateless `lastmile-*` (infra ber-state ditolak 403, ADR D19) + self-heal restart policy; strategy-lab = satu duel bersamaan (409) dengan `cpus: 1.0` — dampak CPU terbatas; demo = satu preset aktif; TIDAK ada data pengguna/PPI (semua data sintetis seed tetap); CORS terbatas verb `GET, POST, OPTIONS` (main.go `cors()`); monitoring UptimeRobot + Incident Timeline membuat flap cepat terlihat. **Jalur naik bila abuse nyata**: bearer token sederhana di api-gateway khusus POST mutasi (middleware ±30 baris, env `MUTATION_TOKEN`, UI mengikuti) — sengaja TIDAK diimplementasikan sekarang agar reviewer bisa mencoba Golden Demo/Strategy Lab langsung tanpa friksi; auth penuh (OAuth dsb.) di luar skala proyek | Ini showcase portofolio, bukan produksi multi-tenant: nilai produknya justru "boleh dicoba" oleh reviewer/rekruter kapan pun; mekanisme auth besar tanpa kebutuhan nyata = kompleksitas tanpa risiko yang dilindungi; blast radius tiap endpoint sudah dibatasi mekanis (allowlist/mutex/cpus), dan kerusakan terburuk = demo berflap sementara & pulih otomatis — bukan kebocoran data |
| D24 | 2026-10-01 | **AI Ops Copilot = plugin adapter di luar core (Fase 7)**: service `copilot` :4207 (profile compose `copilot` OFF-by-default, internal saja, 128 MiB) dengan interface `LLM { Complete }` + provider **noop (ErrNoLLM)** tanpa `OPENAI_API_KEY` → `/capabilities {"enabled":false}` via proxy api-gateway → **UI tidak merender panel apa pun (0 node copilot di DOM)** dan seluruh 6 layar + Golden Demo + replay tetap utuh (terverifikasi headless, console 0). Plan Advisor: metrik live (`/api/kpi`+`/api/metrics`+`/api/chaos/incidents` — tanpa akses DB) → 1 call LLM schema JSON ketat ≤3 plan → parser membuang plan invalid (bukan memperbaiki diam-diam) → **setiap plan di-dry-run simulator deterministik via jalur duel `internal/duel` (seed sama baseline vs plan, 120 s virtual ≈ 0,2 s wall di graph Berlin penuh)**; aksi `kill` tidak bisa disimulasikan → plan dilaporkan tanpa angka palsu + Note. Ask Ops: jawaban WAJIB sitasi data internal (field `sources`); tanpa sitasi / id tak dikenal = **ditolak 422**. Eksekusi plan = tombol manusia konfirmasi 2 langkah memakai endpoint kontrol existing (sim-control/chaos); LLM tidak pernah punya kredensial & tidak pernah mengeksekusi. Secret hanya env VPS (`.env`, aksi pemilik); provider OpenAI-compatible apa pun via `OPENAI_BASE_URL` (HTTP murni, tanpa SDK); rate limit token bucket sederhana per endpoint | ADR D8 (core 100% deterministik tanpa LLM) diperkuat: LLM = pengusul teks, **simulator = judge**, manusia = eksekutor; "tanpa key = fitur tersembunyi" diuji sebagai kriteria pemblokir (regresi 6 layar + demo + replay); dry-run adil dijamin konstruksi duel (satu generator → dua engine, ADR D18); evaluasi kualitas jawaban (ground truth 15 kasus) terdokumentasi di `reports/phase-07-copilot.md` — eksekusi live evaluasi menunggu API key pemilik (kode + unit test penilaian sudah lengkap) |
| D25 | 2026-10-02 | **Zona panas peta = densitas order aktif yang mengikuti surge, digambar di loop rAF peta existing** (fase 8): grid ±500 m (pickup utk waiting/assigned, dropoff utk in-transit), sel ≥2 order digambar sprite radial `status.amber→status.coral` (token zone.hot dari DESIGN.md), intensitas & napas sinus 4 s proporsional `st.su` nyata; delivery burst (in-transit hilang = violet/cyan 700 ms) & expiry fade (waiting hilang = coral 450 ms) dengan guard lompatan waktu (scrub/mundur >8 s = reset tanpa spawn); reduced-motion = statis & tanpa spawn; nama jalan = properti visual `n` di roads.geojson (replikasi simplifikasi graphgen atas Overpass — graph routing byte-identical) + glyph PBF Inter self-hosted ±355 KB (ADR D12 tetap: nol tile/font provider) | Motion bermakna data (DESIGN §5): panas = order nyata, burst = delivery nyata, gagal = expiry nyata; janji `zone.hot` DESIGN.md akhirnya terimplementasi; nol rAF baru (terukur 28/2 s idle); hit-test hover 0,1–0,2 ms memakai pickData per-frame yang sudah ada; inspector live memakai derivasi klien (haversine) tanpa mengubah kontrak `model.Snapshot` |
| D26 | 2026-10-04 | **Bahasa simbol peta + tema dark/light** (permintaan pemilik pasca-fase 9; frontend-only, kontrak snapshot tidak berubah): (1) satu jenis entitas = satu bentuk — rider = sprite motor digambar Path2D (per status warna token) **dirotasi menghadap arah gerak** (heading prev→posisi terinterpolasi di ruang layar; rider belum bergerak menghadap target assignment), order waiting/assigned = ikon garpu-pisau di titik pickup (cyan/amber), order in-transit = ikon rumah di titik dropoff (violet); rider idle redup tanpa glow (hierarki: yang aktif menonjol); (2) **decluttering garis assignment** — garis dashed hanya untuk entitas di-hover/dipilih, semua garis baru muncul saat zoom ≥ 14.5 (di atas clamp zoom-min ~13.4 yang disebabkan `maxBounds` pada viewport lebar — temuan verifikasi); (3) **nama jalan & kawasan di zoom default** — label major fade-in mulai z12.2 (sebelumnya z13 = tak pernah terlihat karena clamp), jalan dinaikkan kontrasnya (`map.road` #223047→#2B3B58), layer baru: label kawasan `places.geojson` (4 distrik + 8 landmark, statis di repo) + POI kuliner `pois.geojson` (1 419 titik diekspor dari graph via `scripts/export-pois.mjs`, circle samar z13+); (4) **tema light/dark = dua palet token penuh** di `tokens.ts` (objek live dimutasi `lib/theme.ts`), CSS vars kedua tema di-inject layout + atribut `data-theme` pada `<html>` diset skrip inline pre-paint (localStorage `pulse.theme` → `prefers-color-scheme` → dark, tanpa flash), MapLibre di-switch via `setPaintProperty` (tanpa reload style/geojson), canvas memakai komposit berbeda per tema (`lighter` di dark, `multiply`+alpha lebih rendah untuk heatmap di light), tombol sun/moon di TopBar (ikon via CSS, nol hydration mismatch) | Simbol sesuai objek = permintaan eksplisit pemilik (motor/resto/rumah, bukan dot seragam); bentuk+arah = redundansi kode-warna (ramah buta warna) & informasi baru (heading gratis dari interpolasi existing); ikon = sprite Path2D pre-render 2× di loop rAF yang SAMA — nol loop baru (terukur 18–40/2 s idle), nol dependensi & nol aset gambar; tema = satu gerbang token (DISIPLIN DESIGN.md §7 terjaga: nol hex di komponen) sehingga seluruh UI + peta + canvas ikut tanpa hardcode; AA dipertahankan kedua tema (aksen light digelapkan: cyan-700/lime-700/amber-700/rose-700/violet-700); ADR D12 tetap utuh — places/pois = data statis OSM di repo, tanpa tile/font provider baru |

## 6. Frontend — 6 Layar Control Room ("Pulse")

1. **Live Ops Map** — peta kota gelap (Berlin dulu), rider = titik bergerak berwarna status
   (idle hijau / ke-resto kuning / pickup biru / antar ungu) + trailing glow; order = pulsa
   radar + garis dashed ke rider; zona = polygon; surge = zona membara (heatmap bernapas).
2. **Surge & Chaos Console** — slider surge ×1→×10, toggle hujan/flash-sale/fleet −30%,
   tombol kill-node; semua aksi tercatat di Incident Timeline (flight recorder).
3. **KPI Command Deck** — kartu metrik angka live (tabular mono): delivery time,
   utilization, cost/order, orders/min, p99, antrian; streaming area chart; SLO gauge;
   kartu bergetar halus saat SLO dilanggar.
4. **Strategy Lab** — jalankan strategi A vs B pada skenario identik → peta replay kembar
   + tabel delta + histogram overlay; export benchmark report (sumber angka README).
5. **System Health** — pipeline order→Kafka→processor divisualisasikan partikel; grid
   status pod; autoscaling events; p99 & RPS live (bukti load-test hidup).
6. **Replay & Inspect** — timeline ala video editor; scrub ke detik mana pun; klik rider →
   detail order yang dibawa + alasan keputusan dispatch.

Design system lengkap: `docs/DESIGN.md` (sumber token — komponen dilarang hardcode warna).

## 7. Skenario Demo

| Mode | Isi | Kapan |
|---|---|---|
| 🟢 Golden Demo Presets | 2–3 narasi kurasi (contoh "Dinner Rush di Berlin": tenang → flash sale ×8 → hujan → kill node → recovery), tombol ▶ Play | Kesan pertama reviewer — selalu konsisten |
| 🟡 Free Playground | Semua kontrol bebas dipakai reviewer, reaksi realtime | Kedalaman teknis |
| 🔵 Replay | Pemutar rekaman simulasi | Fallback saat backend tidak hidup |

## 8. Metrik Benchmark yang Dilaporkan (target format)

- Throughput order: baseline → sesudah optimasi (req/menit, zero loss)
- p99 latency API keputusan dispatch (< 50 ms target)
- Delta antar strategi dispatch: avg/p95 delivery time, rider utilization, cost per order
- Chaos: MTTD/MTTR, error budget, waktu self-heal
- Semua dapat direproduksi: `make benchmark` → laporan di `reports/`

## 9. Hosting & Isolasi (ringkas — detail runbook di `deploy/README.md`)

- Frontend: Vercel (Hobby) — `lastmile-lab.ricothen.com`
- Backend: VPS vmi3585780 — `api.lastmile-lab.ricothen.com` (:3010) &
  `ws.lastmile-lab.ricothen.com` (:3012) via Caddy (site block + reload, tanpa restart)
- Blok port khusus terdaftar di `/home/rico/PORTS.md` (3010/3012/3013/3030, 4201–4204,
  5434, 6380, 9091, 19092) — total budget memori stack ≤ 2 GB
- Monitor eksternal (UptimeRobot) pada `/healthz` — Fase 6
- Anti-halaman-mati: replay fallback + aset statis di Vercel

## 10. Batasan & Non-Goals

- Bukan sistem produksi sungguhan; tidak memproses data pribadi/pembayaran nyata.
- Tidak meniru brand apa pun (netral, open source style, tanpa aset Delivery Hero).
- Multi-kota (selain Berlin) = non-goal sampai Fase 6 selesai.
- Mobile app = non-goal (responsive web cukup).
