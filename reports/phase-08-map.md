# PHASE 08 REPORT — Map Craft & Map Interactivity

> Tanggal: 2026-10-02 · Verifikasi: `reports/phase-08-ui-verify.mjs` —
> **ALL PHASE-08 CHECKS PASSED** (23 check) · console error 0 · target uji:
> build produksi lokal (identik bundle Vercel, env `wss://ws.`+`https://api.`
> dibaked) + backend produksi nyata (stack lastmile di VPS ini).

## 1. Hasil verifikasi (headless Playwright 1.63, chromium swiftshader)

| Check | Hasil |
|---|---|
| `live_link` | ok — LIVE LINK via WS produksi |
| `glyphs_selfhosted` | ok — `glyphs: '/fonts/{fontstack}/{range}.pbf'` |
| `layer_roads-glow` / `roads-label-major` / `roads-label-minor` | ok — 3 layer baru ada di style |
| `street_labels_rendered` | ok — 31 label ter-render di z15, contoh "Waisenstraße" |
| `font_requests_local` | ok — 1 request font, 0 request font pihak ketiga |
| `hover_tooltip` | ok — tooltip `RIDER r1 · IDLE` mengikuti pointer |
| `hover_hit_test_ms` | ok — 0,1–0,2 ms (< 16 ms DoD) |
| `live_inspect_card` | ok — kartu `RIDER R1` + status + ring coral; Escape/klik kosong menutup |
| `heat_probe` | ok — cells=11, max=4, total=54 order, su=1 |
| `heat_surge_echo` | ok — echo `su=4` terbaca di probe setelah POST ×4 |
| `heat_intensifies` | ok — **alphaSum 0,64 → 1,393 (2,2×)** pada su 1→4; dipulihkan ×1 |
| `delivery_burst` | ok — counter naik (delivered=4, expired=3) tanpa console error |
| `raf_idle_2s` | ok — 28 callback/2 s (baseline loop peta; nol rAF baru) |
| `replay_inspect_reason` | ok — kartu inspect replay (reason dispatch) tidak regresi |
| `responsive_screens` | ok — 1024 & 768 |
| `console_errors` | **0** |
| `reduced_motion_heat_flag` / `reduced_motion_tooltip` | ok — napas & burst mati, tooltip tetap bekerja |

Bukti visual: `reports/phase08-labels-z15.png` (nama jalan + glow) ·
`phase08-hover-tooltip.png` (tooltip + heatmap dasar) · `phase08-heat.png`
(surge ×4 — blob menghangat) · `phase08-live-inspect.png` (kartu live) ·
`phase08-replay-regression.png` · `phase08-reduced-motion.png` ·
`phase08-responsive-{1024,768}.png`.

## 2. Yang dibangun

1. **Nama jalan OSM** — `scripts/add-road-names.mjs`: replikasi logika
   simplifikasi graphgen (walk chain derajat-2) atas data Overpass mentah
   (4 kuadran, retry/backoff), mayoritas nama segmen per chain → properti `n`
   di `roads.geojson` (match 52,3%; sisanya = service road yang memang tak
   bernama di OSM — 98,7% jalan class-1 bernama). Graph routing tidak disentuh.
2. **Glyph PBF self-hosted** — `public/fonts/Inter/{0,256,512,768}-1023.pbf`
   (±355 KB total) via fontnik (docker node:22-bookworm) dari variable font
   Inter; default instance = Regular 400; cakupan äöüß terverifikasi
   (font-inspect). Nol font/tile eksternal (ADR D12 utuh).
3. **Style peta** — layer `roads-glow` (glow jalan major mengikuti zoom —
   janji token `map.road` di DESIGN.md), 2 layer label `symbol` (major z13,
   minor z14, halo bgBase, fade-in). Token baru: `mapRoadGlow`, `mapLabel`,
   `mapLabelMajor` (kontras AA terhadap bgBase: 7,4× dan 12,7×).
4. **Hover + klik inspector live** — hit-test pada `pickData` yang memang
   sudah dihitung tiap frame (0,1–0,2 ms); tooltip mengikuti pointer via DOM
   langsung (tanpa re-render per gerakan; state hanya saat ganti identitas);
   klik → kartu inspect live (status, order, umur first-seen, jarak haversine
   klien, koordinat rute); Escape/klik kosong menutup; replay panel tetap
   memegang klik saat aktif (kartu reason fase 5 tidak regresi).
5. **Zona panas bernapas (ADR D25)** — densitas order aktif per grid ±500 m
   (pickup utk waiting/assigned, dropoff utk in-transit), sel ≥2 order,
   sprite radial `status.amber→status.coral`, intensitas + napas sinus 4 s
   mengikuti `st.su` nyata; sepi+surge 1 = tidak tampak; reduced-motion statis.
6. **Delivery burst / expiry fade** — order in-transit hilang = burst violet/
   cyan 700 ms; waiting/assigned hilang = fade coral 450 ms; guard lompatan
   >8 s / mundur (scrub replay) reset tanpa spawn; reduced-motion tidak spawn.
7. **Hook verifikasi** — `__lmPick` (selalu), `__lmHoverMs`, `__lmHeat`
   (cells/max/total/su/reduced/alphaSum), `__lmBursts`, `__lmMap`.

## 3. Bug yang ditemukan & diperbaiki selama verifikasi

1. **Pair-match nama jalan 0–15%** — `roads.geojson` = edge HASIL
   simplifikasi (ujung = node persimpangan), bukan node OSM berurutan →
   skrip dirombak mereplikasi walk chain graphgen (solusi §2.1).
2. **Filter `['has', ['get','n']]`** — `has` butuh key string, bukan
   ekspresi `get` → label tak pernah ter-place; fix `['has', 'n']`.
3. **Casing direktori glyph** — fontstack `Inter` → `/fonts/Inter/…`;
   direktori awal `inter/` (lowercase) = 404 → label tanpa glyph.
4. **`next start` cache daftar public saat boot** — rename direktori font
   butuh restart server (persekitaran verifikasi, bukan bug app).
5. **Skrip verifikasi** — `innerText` ter-transform uppercase CSS (cek teks
   kini regex case-insensitive); titik-klik "clear" menghitung jarak ke
   dirinya sendiri (0 px → tak pernah clear); `queryRenderedFeatures` pakai
   bentuk satu-argumen; timeout dump replay 12,9 MB dinaikkan ke 90 s
   (host load 8+).

## 4. Catatan proses

- Overpass: `overpass-api.de` 406 (WAF) & kumi 429 dari VPS → mirror
  `maps.mail.ru` dipakai dengan kuadran + backoff (terdokumentasi di skrip).
- Verifikasi dijalankan terhadap build lokal ber-env produksi (pola fase 6)
  karena produksi baru berubah setelah push; backend = stack produksi nyata.
  Mutasi surge dipulihkan ×1 (terverifikasi `surge_restored: ok`).
- DoD fase 8 di `docs/PHASES/phase-08.md` — seluruh checkbox terpenuhi.
