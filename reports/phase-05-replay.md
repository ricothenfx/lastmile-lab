# Fase 5 — Replay Engine + Golden Demo + Polish Motion — Laporan Verifikasi

> Tanggal: 2026-10-01 · Stack `lastmile` profile `sim` + `chaos` di VPS vmi3585780.
> Verifikasi headless: Playwright v1.63.0 (container `mcr.microsoft.com/playwright:v1.63.0-noble`,
> `--network host`, app `next start` :3000 dibangun dengan `NEXT_PUBLIC_WS_URL=ws://172.19.0.1:3012/ws`
> & `NEXT_PUBLIC_API_URL=http://172.19.0.1:3010` — pola `reports/phase-04-ui-verify.mjs`).

## 1. Replay Engine (scrub ke detik mana pun + inspect)

**Rekaman**: goroutine terpisah di rider-sim menarik `FullSnapshot()` 5 Hz (200 ms) ke ring
`internal/replay` — engine TIDAK disentuh (determinisme same-seed tetap diuji), kontrak
`model.Snapshot` tidak berubah (nol field baru). Blob JSON per frame dikompresi gzip sekali
saat direkam (storage), keputusan dispatch ikut direkam (dedupe by seq, cap 8 000).

| Ukuran buffer (terukur) | Nilai |
|---|---|
| Frame per 15 menit @ 5 Hz | 4 500 (cap) + pagar byte 24 MiB |
| Storage ring terukur | 9,0 MiB gzip @ 3 583 frame (~2,5 KB/frame @ 100 rider + ±40 order) |
| Estimasi ring penuh | ±11–12 MiB gzip (unit test: 4 500 frame raw 20,7 MB → 4,6 MB @ tanpa order) |
| Keputusan terekam | 244 @ 11 menit (cap 8 000 ≈ 2 MB struct) |
| RSS rider-sim steady | **46,4 MiB / 160 MiB** (naik dari 17,6 MiB saat kosong) |
| Dump via api-gateway | 8,63 MB gzip ⇔ 29,9 MB JSON (±4 380 frame), `Content-Encoding: gzip` passthrough |

**Endpoint**: `GET /api/replay/sessions` (daftar) + `GET /api/replay/sessions/{id}`
(dump `{"meta","wall0","t0","count","frames":[…],"decisions":[…]}`), proxy api-gateway
`/api/replay/*`. Scrub akurasi **≤ 0,2 s** (interval frame 200 ms), binary search di sisi klien
(pola MiniReplay fase 3).

**Inspect (bukti UI — `phase05-replay-inspect.png`)**: klik rider → kartu status/order dibawa/
pickup/dropoff + **alasan keputusan dispatch** dari ring decisions, contoh terekam di sesi:

> "fifo: antrean tertua (umur 0s) → rider r40 idle terdekat (1011 m, haversine)"

klik order → kartu status + umur (sejak order muncul di feed) + rider pengangkut +
pickup/dropoff (contoh: ORDER o000049 · ASSIGNED · UMUR 03:28 · RIDER r91).

## 2. Golden Demo presets

Tiga preset narasi ±90 s hard-coded di `apps/services/api-gateway/demo.go` (satu file);
orchestrator HANYA memakai kontrol yang sudah ada — surge/weather via sim-control, kill via
chaos injector (ADR D19) — tanpa jalur kode baru di engine, tanpa LLM:

| Preset | Durasi | Langkah | Chaos |
|---|---|---|---|
| `dinner-rush` — Dinner Rush di Berlin | 90 s | 7 | kill `rider-sim` @t+54 |
| `blackout-drill` — Blackout Drill | 90 s | 5 | kill `ws-gateway` → `strategy-lab` → `rider-sim` |
| `rain-commute` — Rain Commute | 88 s | 6 | — (narasi demand/hujan) |

**Eksekusi end-to-end di UI headless (dinner-rush)** — log api-gateway:

```
03:14:18 langkah 1/7 t+0.3s  — Jaringan tenang — armada 100 rider siap [ok]
03:14:31 langkah 2/7 t+12.0s — FLASH SALE ×8 — order membanjir [ok]
03:14:54 langkah 3/7 t+36.0s — Hujan deras — rider melambat 40% [ok]
03:15:15 langkah 4/7 t+54.0s — CHAOS: node simulasi dimatikan — self-heal
03:15:22 langkah 5/7 t+64.0s — Hujan reda saat node pulih [ok]
03:15:32 langkah 6/7 t+74.0s — Lonjakan mereda — antrean mencair [ok]
03:15:44 langkah 7/7 t+86.0s — Kembali tenang — SLO pulih [ok]
03:15:48 selesai t+90.0s (durasi preset 90s)
```

- Durasi preset (pengukuran langsung API, play → inactive): **dinner-rush 91,2 s**,
  **rain-commute 89,3 s** — langkah selalu di detik yang sama (t+12/36/54/…).
- Kill dalam preset = `POST /api/chaos/kill` biasa → **incident chaos-kill terekam di
  Incident Timeline (satu sumber kebenaran)**: `inc-muoy6vze-rider-sim` MTTD 642 ms;
  run lain MTTD 0,43–2,17 s · MTTR 0–5,1 s (MTTR 0 ms = pulih di antara dua probe 1 Hz).
  Pada satu run, POST kill timeout 4 s karena load host 11+ → langkah ditandai `skipped`
  tanpa menggagalkan narasi, dan chaos tetap mengeksekusi kill-nya (incident tetap ada) —
  bukti sanitas fallback.
- UI: banner narasi langkah-demi-langkah + tombol STOP; `demo_steps_seen 1/7 … 7/7`;
  setelah selesai app kembali LIVE LINK otomatis (self-heal), `demo_state.last_id=dinner-rush`.
- Catatan: angka "durasi" di loop verifikasi (116–122 s) = slack polling browser di bawah
  beban (innerText memaksa layout tiap 2 s + koneksi browser jenuh sesaat setelah kill);
  kebenaran durasi adalah log server di atas.

## 3. Frontend hidup 100% tanpa backend (regresi fixture fase 1 — lolos)

`OFFLINE_MODE=1` dijalankan setelah `docker stop rider-sim + ws-gateway + api-gateway`:

- `fixture_fallback: REPLAY MODE tampil (tanpa backend)` — banner + loop fixture fase 1 utuh.
- Replay panel beralih otomatis ke fixture: `REPLAY & INSPECT · FIXTURE (OFFLINE)`,
  scrub tetap jalan (`T+00:22 / 00:45`).
- Golden Demo: tombol `▶ DEMO · OFFLINE` (mati) — demo hanya aktif saat backend hidup.
- `console_errors: 0` (error koneksi yang diharapkan disaring; tanpa crash/halaman putih).

## 4. Motion & visual audit

- **Tanpa rAF idle baru**: panel replay tidak punya rAF sendiri — scrub/pause menggambar
  hanya saat frame berubah (guard `key` + flag dirty di LiveMap, pola on-demand MiniReplay);
  PLAY memajukan playhead dari wall-clock yang dibaca loop rAF peta yang SUDAH ada.
  Terukur: rAF pause+scrub 35–91/2 s ≈ baseline halaman (nilai headless dipengaruhi load
  host 8–11; tidak ada loop kedua).
- **Reduced-motion penuh**: PLAY disembunyikan (`reduced_play_hidden: ya`), scrub statis
  tetap berfungsi (`reduced_scrub: ok (T+00:43 / 01:02)`), animasi CSS dimatikan global.
- **Token-only**: semua warna komponen baru via utilitas token existing (teks-ink/*,
  border-line-subtle, surface-raised, status-*); canvas memakai `tokens`/`riderStatusColor`.
- **Responsive 1440/1024/768**: bukti `phase05-1440/1024/768.png` — dock bawah (▶ DEMO +
  ⟲ REPLAY) muat di ketiganya; panel replay max-w `calc(100vw-2rem)`.
- **Console error 0** sepanjang mode utama.
- Artefak dikenal (non-fungsional): potongan putih ±25 px di pojok kanan-bawah GL canvas
  pada renderer software swiftshader headless (di belakang panel System Health). Tidak
  muncul di pipeline GPU normal; verifikasi laptop fisik tetap langkah sisa go-live.

## 5. Bug yang ditemukan & diperbaiki selama verifikasi

1. **Chromium menghentikan stream gzip multi-member setelah anggota pertama** — dump awal
   digabung dari anggota gzip per frame (valid untuk curl/Go, GAGAL di browser:
   `Unexpected end of JSON input`). Fix `65daada`: dump kini SATU anggota gzip tunggal
   (dekompresi-rekompresi streaming saat dilayani); unit test mengassert tepat 1 anggota.
2. **Peta hitam total**: CSS `.maplibregl-map { position: relative }` (maplibre-gl.css)
   menimpa utility Tailwind `absolute` saat urutan chunk CSS bergeser (penambahan modul
   baru fase 5) → kontainer peta tinggi 0 → overlay canvas 1440×0, peta/inspect mati.
   Fix `668abf7`: `position: absolute` inline pada kontainer (menang di cascade manapun).
3. **ARIA `role="menu"`/`menuitem`** pada dropdown demo mengubah role semantik tombol
   (Playwright `getByRole('button')` tidak menemukannya; screen reader juga kehilangan
   semantics button). Fix `668abf7`: pola popover biasa + `aria-label`.

## 6. Deploy & budget

- CI hijau di ketiga commit (ci + images: rider-sim, api-gateway, dsb. — 9 image GHCR).
- Stack di-deploy ulang: `docker compose -p lastmile --profile sim --profile chaos pull && up -d`
  — 7/7 kontainer lastmile healthy, kini termasuk **sim-control di profile `sim`**
  (aktuator Golden Demo mode demo; RAM sudah terhitung sebelumnya).
- RAM stack: **1 888 MiB limit ≤ 2 GB** (rider-sim 128→160 MiB untuk ring replay; total
  tetap di bawah anggaran keras). Terukur: rider-sim RSS 46,4 MiB, api-gateway 1,9 MiB,
  sim-control 4,8 MiB.
- Tanpa port host baru (semua via api-gateway :3010 yang sudah terdaftar).

## 7. Artefak

| File | Isi |
|---|---|
| `reports/phase05-ui-verify.mjs` | skrip verifikasi (mode utama + `OFFLINE_MODE=1`) |
| `phase05-replay-inspect.png` | peta + scrub T+05:49 + kartu order (umur, rider, koordinat) |
| `phase05-demo-kill.png` | detik kill: REPLAY MODE banner, deck PARTIAL 5/8, RAIN aktif |
| `phase05-demo-done.png` | setelah preset selesai, app kembali LIVE |
| `phase05-fixture-offline.png` | backend mati: FIXTURE panel + demo tombol OFFLINE |
| `phase05-reduced.png` | reduced-motion (tanpa PLAY, scrub statis) |
| `phase05-1440/1024/768.png` | responsive |
