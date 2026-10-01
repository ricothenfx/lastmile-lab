# PHASE 05 — Replay Engine + Golden Demo Presets + Polish Motion

> Spesifikasi fase 5 (ditulis akhir fase 4). Scope & DoD di bawah adalah batas
> keras fase ini. Kriteria UI tetap kriteria pemblokir.

## Tujuan

Reviewer harus bisa memutar ulang simulasi seperti video: scrub ke detik mana
pun, klik rider → detail order yang dibawa + alasan keputusan dispatch, dan
menjalankan 2–3 Golden Demo presets (▶ Play) dengan narasi 90 detik yang selalu
konsisten. Sisi motion & visual diaudit akhir: konsistensi token, 60 fps,
responsive 1440/1024/768.

## Scope (kerjakan ini saja)

### Backend
1. **Rekaman session** — recorder snapshot ringan: rider-sim (atau ws-gateway)
   menyimpan ring frame terkompresi (mis. 15 menit @ 5 Hz, memori/size bounded)
   + metadata (seed, strategi, preset, surga/weather timeline). Endpoint
   `GET /api/replay/sessions` (daftar) + `GET /api/replay/sessions/{id}`
   (frame + decisions window). Kontrak `model.Snapshot` tidak berubah.
2. **Golden Demo presets** — skenario narasi terkurasi (contoh "Dinner Rush di
   Berlin": tenang → flash sale ×8 → hujan → kill rider-sim → pulih) dijalankan
   oleh service yang sudah ada (sim-control + chaos) dengan langkah ber-waktu;
   endpoint `POST /api/demo/play` + `GET /api/demo/state`; sekali jalan,
   bisa dihentikan; tanpa LLM.
3. Snapshot wire: field optional baru hanya bila perlu (kontrak tidak breaking).

### Frontend
4. **Replay & Inspect panel** — timeline ala scrubber (input range + marker
   incident), PLAY/PAUSE (rAF hanya saat PLAY), kecepatan ×1/×4/×16; klik rider
   → kartu detail (status, order dibawa, pickup/dropoff, alasan dispatch dari
   ring decisions); klik order → status + umur.
5. **Golden Demo launcher** — daftar preset dengan tombol ▶ Play; saat jalan:
   banner narasi berjalan (langkah ke langkah), kontrol demo (stop); aksi kill
   dalam preset memakai chaos yang sama (konfirmasi tidak perlu untuk preset).
6. **Fallback tetap harga mati** — tanpa backend: replay fixture rekaman lama
   tetap diputar; Golden Demo hanya aktif saat backend hidup.
7. Motion audit terhadap `docs/DESIGN.md`: semua warna via token; angka mono
   tabular; tanpa rAF idle baru (scrub/inspect render on-demand).

### Infra & docs
8. Eksperimen/verifikasi terdokumentasi: `reports/phase-05-replay.md`
   (ukuran buffer replay, akurasi scrub, durasi preset, bukti UI headless).
9. Update docs: PROGRESS, ROADMAP, PHASES/phase-06.md, ADR bila ada keputusan.

## Di luar scope (dilarang di fase ini)

- Hardening produksi (rate limit, log rotation audit, UptimeRobot) + README
  final + artikel (Fase 6); AI Ops Copilot (Fase 7); multi-kota; mobile app.

## Definition of Done

- [ ] Replay engine: scrub timeline ke detik mana pun + inspect rider/order
      dengan alasan keputusan dispatch terlihat
- [ ] 2–3 Golden Demo presets (▶ Play) dengan narasi ±90 detik, selalu
      konsisten, termasuk langkah chaos + pulih
- [ ] Frontend tetap hidup 100% saat backend dimatikan (replay fallback fase 1
      tetap bekerja; tidak ada regresi)
- [ ] Audit motion & visual: token-only, kontras AA, reduced-motion penuh,
      60fps (tanpa rAF idle baru), responsive 1440/1024/768
- [ ] Semua service/endpoint baru punya health + limit RAM + CI hijau
      (ci + images); angka keputusan di `reports/phase-05-replay.md`
- [ ] Commit & push

## Catatan Teknis

- Buffer replay di memori service (bounded, FIFO) — jangan tulis disk besar;
  ukuran frame ~30–60 KB @5 Hz → 15 menit ≈ 15–25 MB: hitung budget RAM stack
  sebelum final (tetap ≤ 2 GB total).
- Scrub = binary search frame + render on-demand (satu canvas reuse pola
  MiniReplay fase 3); jangan buka rAF kecuali PLAY.
- Golden Demo = orchestrasi kontrol yang SUDAH ada (surge/weather/chaos), bukan
  jalur kode baru di engine; langkah narasi hard-coded di satu file.
- Kill dalam preset menandai incident `chaos-kill` biasa → Incident Timeline
  tetap satu sumber kebenaran.
