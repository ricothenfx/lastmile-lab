# PHASE 08 — Map Craft & Map Interactivity

> Spesifikasi fase 8 (diminta pemilik 2026-10-02): peta Live Ops Map naik kelas —
> **nama jalan nyata dari data OSM**, animasi yang **semuanya bermakna data**
> (bukan dekoratif), dan **dot rider/order bisa di-hover & diklik di mode live**
> (sebelumnya inspect hanya aktif di replay). Pemicu: review pemilik — "peta tidak
> ada nama jalan", "dot driver tidak bisa diklik/hover".

## Prinsip yang tidak bisa ditawar

1. **Nol API key / tile provider eksternal** (ADR D3 + D12): nama jalan dari
   GeoJSON OSM yang di-commit, glyph font di-host sendiri di `public/fonts/`.
2. **Graph routing tidak disentuh**: `berlin_graph.json` + kontrak
   `model.Snapshot` tetap byte-identical — nama jalan hanya *props visual* di
   `roads.geojson` (merge by koordinat node, bukan regenerate graph).
3. **Tidak ada rAF baru**: semua animasi digambar di loop rAF peta yang sudah
   ada (audit rAF idle tidak boleh naik). Render on-demand replay-pause tetap.
4. **Motion bermakna data** (DESIGN §5): heatmap = densitas order nyata yang
   mengikuti surge factor; burst = delivery sungguhan; glow jalan = konsekuensi
   zoom. Reduced-motion mematikan semua non-esensial.
5. Token-only, UI 100% Inggris, kontras AA (DESIGN §7).

## Scope (kerjakan ini saja)

### A. Nama jalan (data + glyph + style)

1. `scripts/add-road-names.mjs` (one-shot, terdokumentasi): query Overpass
   `way["highway"]["name"]` untuk bbox peta → map pasangan-koordinat-node →
   nama → merge properti `n` ke `roads.geojson` existing. Laporan match-rate.
   Graph & fixture TIDAK berubah.
2. Glyph PBF self-hosted: fontstack **Inter** (static weight 400 di-instance
   dari variable font via fonttools, lalu fontnik → `public/fonts/inter/
   {range}.pbf`, range 0–1023). MapLibre `glyphs: '/fonts/{fontstack}/
   {range}.pbf'` — tanpa font server pihak ketiga.
3. Style: 2 layer `symbol` — jalan major (minzoom 13, `map-label-major`) dan
   minor (minzoom 14, `map-label-muted`), `symbol-placement: line`, halo
   `bg.base`, fade-in halus. Token baru: `mapLabel`, `mapLabelMajor`,
   `mapRoadGlow`.

### B. Glow jalan saat zoom (DESIGN `map.road` "glow halus")

Layer `roads-glow` di bawah `roads-major`: width & opacity naik dengan zoom
(z13.5 mulai terlihat, z15 penuh). Murni style, nol biaya runtime tambahan.

### C. Heatmap zona bernapas (data-driven, ADR baru D25)

- Densitas order aktif per grid ±500 m (pickup utk waiting/assigned, dropoff
  utk in-transit), sel dengan ≥2 order digambar sprite radial `status.amber →
  status.coral` (blend mengikuti `st.su` 1→10), alpha dasar naik dengan surge,
  **bernapas** sinus 4 s (amplitudo ikut surge).
- Surge 1 & traffic sepi → tidak tampak (jujur: tidak ada order = tidak ada
  panas). Demo Dinner Rush → menyala.
- Reduced-motion: statis tanpa napas. Replay-pause: ikut guard key (statis).

### D. Delivery burst & expiry fade

- Order in-transit menghilang di frame berikut → **burst sukses** di titik
  dropoff (2 cincin cyan/violet + flash, ~700 ms).
- Order waiting/assigned menghilang → **fade coral singkat** (~400 ms,
  kegagalan TTL — bukan perayaan).
- Guard: lompatan waktu mundur/scrub > 8 s → reset tanpa spawn (mencegah
  badai burst saat scrub replay). Reduced-motion: tidak spawn.

### E. Hover + klik inspector mode live

- **Hover** rider/order (hit-test radius 16 px pada `pickData` yang memang
  sudah dihitung tiap frame): cursor pointer + cincin hover di canvas + kartu
  tooltip mengikuti pointer (rider #, status berwarna, order dibawa, rute
  pickup→dropoff, jarak haversine — dihitung di klien). Tooltip tidak memicu
  re-render per gerakan mouse (posisi via ref DOM; state hanya saat ganti id).
- **Klik** saat replay override TIDAK aktif → kartu inspect live (kiri-bawah
  peta): status, order + umur (first-seen per id), jarak rider→target,
  jarak pickup→dropoff. Update 1 Hz saat terbuka (subtree kecil saja).
  Klik area kosong → deselect. Mode replay: perilaku existing tidak berubah.
- Hook verifikasi: `__lmPick` kini selalu terekspos + `__lmHoverMs`,
  `__lmHeat`, `__lmBursts`, `__lmMap`.

## Di luar scope

Backend Go apa pun (kontrak & graph utuh), tile provider, three-deck.gl,
search/panah rute, perubahan replay engine.

## DoD (kriteria pemblokir)

- [x] Label nama jalan tampil saat zoom ≥ 13 (probe `queryRenderedFeatures`),
      hanya jalan bernama, glyph di-load dari domain sendiri (tanpa request
      font pihak ketiga), tanpa clutter (halo + fade-in + minzoom).
- [x] Glow jalan major terlihat saat zoom; murni layer style.
- [x] Heatmap: sel terukur via probe; intensitas naik saat surge dinaikkan
      (dan dipulihkan ×1); statis di reduced-motion; nol rAF tambahan
      (audit rAF idle dalam toleransi baseline).
- [x] Burst delivery terjadi (counter naik dalam 30 s) tanpa console error.
- [x] Hover dot → tooltip tampil dengan id yang cocok + `__lmHoverMs < 16`;
      cursor pointer.
- [x] Klik dot mode live → kartu inspect live konsisten (id + status);
      klik replay → kartu reason existing TIDAK regresi.
- [x] `tsc --noEmit` + `next build` sukses; backend tidak berubah.
- [x] Verifikasi headless `reports/phase-08-ui-verify.mjs`: semua di atas +
      console error 0 + screenshot 1440/1024/768 + reduced-motion →
      `reports/phase-08-map.md`.
- [x] Docs: PROGRESS log + ROADMAP status + spec phase-09 — satu commit.

> **SELESAI 2026-10-02.** Bukti lengkap di `reports/phase-08-map.md`
> (ALL PHASE-08 CHECKS PASSED — 23 check, console 0, rAF idle 28/2 s,
> hit-test hover 0,1–0,2 ms, alphaSum heatmap 2,2× pada surge ×4).
