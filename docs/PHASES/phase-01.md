# PHASE 01 — Simulasi Inti + Live Ops Map

> Spesifikasi awal fase 1 (ditulis akhir fase 0). Sesuaikan detail saat mulai eksekusi,
> tapi **scope & DoD di bawah adalah batas keras fase ini**.

## Tujuan

Momen "wow" pertama: kota Berlin gelap di peta, rider bergerak nyata di jalan sungguhan,
order berpulsa masuk, assignment tergambar — semua streaming 60fps dari backend Go
sungguhan. Landing page diganti aplikasi sungguhan dan live di Vercel.

## Scope (kerjakan ini saja)

### Backend (Go)
1. **rider-simulator** (`apps/services/rider-sim`):
   - Road network Berlin dari OSM (extract kecil, mis. berlin-latest dari Geofabrik,
     diproses jadi graph — pustaka apa pun yang praktis: osm graph di Go).
   - N rider (default 60, config) bergerak di graph dengan status: `idle` / `to_pickup` /
     `pickup` / `delivering`; kecepatan dasar + faktor cuaca (stub dulu, Fase 2 dipakai).
   - Tick simulasi fixed-rate (mis. 10 Hz internal), state expose via in-memory API.
2. **Order generator** (sederhana, dalam rider-sim atau service kecil):
   - Order muncul Poisson (configurable rate), lokasi pickup/dropoff dari titik-titik POI
     acak di graph; TTL antrean sederhana.
3. **Dispatch minimal FIFO** (interface `Strategy` sudah dibentuk — Fase 3 menambah
   strategi lain tanpa refactor): order tertua di-assign ke rider idle terdekat (haversine
   / jarak graph). Simpan alasan keputusan (explainability stub).
4. **ws-gateway** (`apps/services/ws-gateway`): broadcast state snapshot (posisi rider,
   order, assignment) via WebSocket, format JSON ringkas, throttled ~10–15 Hz.
5. `api-gateway` minimal: `/healthz` + snapshot REST awal (untuk fallback & debugging).
6. Dockerfile multi-stage untuk tiap service; compose service baru dengan mem_limit +
   healthcheck (ikuti pola `deploy/compose.yaml`).

### Frontend (`apps/web`)
7. **Migrasi ke Next.js + TypeScript + Tailwind + Framer Motion** (design tokens dari
   `docs/DESIGN.md` → `tokens.ts` + CSS variables; larangan hardcode warna).
8. **Live Ops Map**: MapLibre GL style gelap (style JSON sendiri memakai token warna:
   `map.road` `#223047`, `map.water` `#0F1B2D`), rider = titik bergerak berwarna status +
   trailing glow halus, order = pulsa ripple saat masuk, garis dashed animasi untuk
   assignment. Koneksi WebSocket ke `NEXT_PUBLIC_WS_URL`.
9. **Replay fallback minimal**: bila WS tidak tersambung dalam 3 detik → mainkan dataset
   rekaman statis (fixture) + banner kecil "replay mode". (Mesin replay penuh tetap Fase 5.)
10. Header app "Pulse", status koneksi, ticker jumlah order/rider aktif — sederhana,
    rapi, sesuai token.
11. `vercel.json` + env vars (`NEXT_PUBLIC_WS_URL`, `NEXT_PUBLIC_API_URL`) — siap deploy;
    lakukan deploy saat WS backend hidup di VPS (lihat deploy/README §DNS — butuh auth
    akun Vercel pemilik, one-time).

### Infra & docs
12. CI diperluas: build Go services + lint + `go vet`; build Next.js (typecheck+build) —
    build berat tetap di CI, bukan VPS.
13. Image GHCR untuk service backend (workflow build+push), supaya VPS nanti hanya pull.
14. Update `deploy/compose.yaml` dengan service baru (masih boleh tak dijalankan penuh di
    VPS sampai akhir fase); update `docs/PROGRESS.md`, ROADMAP, phase-02.md.

## Di luar scope (dilarang di fase ini)

- Redpanda/pipeline Kafka, idempotency Redis, Postgres persistence penuh (Fase 2)
- Surge Console & chaos (Fase 2/4), Strategy Lab (Fase 3), Replay engine penuh (Fase 5)
- Menjalankan stack permanen di VPS selain yang dibutuhkan demo map

## Definition of Done

- [ ] Rider bergerak mulus di jalan nyata Berlin (bukan titik acak), status warna sesuai
      token (`lime/amber/cyan/violet`)
- [ ] Order baru = pulsa radar + garis assignment dashed; tick angka order/rider live
- [ ] **60fps di laptop menengah** dengan 60 rider + 30 order/menit — kriteria pemblokir
- [ ] `prefers-reduced-motion` dihormati; kontras AA; semua warna via token
- [ ] WS putus → replay fallback jalan + banner; tidak ada layar putih/mati
- [ ] Semua service punya `/healthz`; CI hijau (build Go + web)
- [ ] App live di https://lastmile-lab.ricothen.com (frontend Vercel, WS via VPS
      `ws.lastmile-lab.ricothen.com`) — atau, bila DNS/auth belum selesai: demo via
      Vercel preview + catatan langkah sisa di PROGRESS.md
- [ ] Commit terkonvensional + push; ROADMAP fase 1 ✅; PROGRESS.md terupdate;
      `PHASES/phase-02.md` ditulis
