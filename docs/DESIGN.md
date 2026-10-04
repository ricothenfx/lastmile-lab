# DESIGN — Design System "Pulse"

> Sumber kebenaran visual & motion. **Komponen dilarang hardcode warna/ukuran** — semua
> lewat token di file ini (nanti: `apps/web/tokens.ts` + CSS variables). Kriteria UI pada
> DoD tiap fase adalah kriteria pemblokir.

## 1. Konsep

**Mission Control / Air Traffic Control untuk delivery.** Dark theme (default) dengan
**tema light opsional** (tombol sun/moon di TopBar, ADR D26), data mengalir
terus, gerakan selalu bermakna. Reviewer harus merasa melihat pusat komando operasi
nyata — bukan dashboard tutorial.

## 2. Palet Warna (token)

### Surface & teks
| Token | Nilai | Pemakaian |
|---|---|---|
| `bg.base` | `#0B1120` | Latar utama aplikasi |
| `bg.raised` | `#111827` | Kartu, panel, sidebar |
| `bg.overlay` | `#1F2937` | Modal, popover |
| `border.subtle` | `#1E293B` | Divider, outline kartu |
| `text.primary` | `#F8FAFC` | Teks utama |
| `text.secondary` | `#94A3B8` | Teks pendukung, label |
| `text.muted` | `#64748B` | Timestamp, hint |

### Aksen & status (satu makna, dipakai konsisten di map + chart + komponen)
| Token | Nilai | Makna |
|---|---|---|
| `accent.cyan` | `#22D3EE` | Aksen utama, data "hidup", link, fokus |
| `accent.lime` | `#A3E635` | Sistem sehat, metrik baik, rider idle |
| `status.amber` | `#FBBF24` | Warning, rider menuju resto |
| `status.coral` | `#FB7185` | Kritis/SLO violation, kill-node, surge ekstrem |
| `status.violet` | `#8B5CF6` | Rider delivering, elemen "in-flight" |
| `map.water` | `#0F1B2D` | Area air peta |
| `map.road` | `#2B3B58` | Jalan peta (naik dari `#223047` agar terbaca di zoom default — D26) |
| `map.roadGlow` | `#35496E` | Glow jalan major saat zoom (fase 8) |
| `map.label` | `#94A3B8` | Label jalan minor (fase 8; = text.secondary, AA 7,4×) |
| `map.labelMajor` | `#CBD5E1` | Label jalan major (fase 8, AA 12,7×) |
| `zone.hot` | gradien `#FBBF24 → #FB7185` | Heatmap zona panas (bernapas) — terimplementasi fase 8 |

### Palet light (tema kedua, ADR D26)
Hue identik dengan dark; aksen/status digelapkan satu step agar teks berwarna tetap
WCAG AA di surface terang. Aktif via `[data-theme="light"]` pada `<html>` (diset
pre-paint; persist di localStorage `pulse.theme`). Semua warna tetap lewat token —
komponen tidak boleh hardcode tema.

| Token | Light | Catatan |
|---|---|---|
| `bg.base` / `bg.raised` / `bg.overlay` | `#F8FAFC` / `#FFFFFF` / `#F1F5F9` | surface terang |
| `border.subtle` | `#E2E8F0` | divider |
| `text.primary` / `secondary` / `muted` | `#0F172A` / `#475569` / `#64748B` | AA terpenuhi |
| `accent.cyan` | `#0E7490` | cyan-700 (AA 4,8×) |
| `accent.lime` | `#4D7C0F` | lime-700 |
| `status.amber` | `#B45309` | amber-700 |
| `status.coral` | `#BE123C` | rose-700 |
| `status.violet` | `#6D28D9` | violet-700 |
| `map.water` | `#C9E2F5` | air peta terang |
| `map.road` / `roadGlow` | `#C4CFDB` / `#D8E0EA` | jalan di latar terang |
| `map.label` / `labelMajor` | `#5A6B80` / `#26334A` | AA 5,5× / 13× |

## 2.5 Bahasa simbol peta (fase 10, ADR D26)

Satu jenis entitas = satu **bentuk**, warna hanya lapis kedua (ramah buta warna).
Semua digambar sprite Path2D pre-render 2× di loop rAF peta yang sama — tanpa
aset gambar, tanpa dependensi, nol rAF baru.

| Simbol | Entitas | Makna warna |
|---|---|---|
| **Motor** (menghadap arah gerak) | rider | lime idle · amber to pickup · cyan pickup · violet delivering |
| **Garpu-pisau** (titik pickup) | order waiting/assigned | cyan waiting · amber assigned |
| **Rumah** (titik dropoff) | order in-transit | violet |
| Titik amber samar | POI kuliner OSM (1 419, `pois.geojson`) | konteks kepadatan resto, z13+ |

Aturan pendamping: rider idle digambar redup tanpa glow (yang aktif menonjol);
garis assignment dashed hanya untuk entitas di-hover/dipilih, semua garis muncul
saat zoom ≥ 14.5; komposit heatmap/glow mengikuti tema (`lighter` di dark,
`multiply` + alpha lebih rendah di light); label kawasan (`places.geojson`) tampil
sejak z11 lalu mundur saat label jalan mengambil alih.

**Kontras:** semua kombinasi teks/surface wajib lolos WCAG AA **di kedua tema**.
Tema gelap/terang bukan alasan kontras rendah.

## 3. Tipografi

| Token | Nilai | Pemakaian |
|---|---|---|
| `font.ui` | Inter | Semua UI text |
| `font.mono` | JetBrains Mono | **Semua angka** (tabular-nums wajib), kode, log |

Skala: `12 / 14 / 16 / 20 / 28 / 40`. Angka dashboard besar pakai 28–40 mono;
label 12 uppercase +0.08em tracking.

## 4. Spasi & Bentuk

- Grid **8px**: 4 (micro) / 8 / 12 / 16 / 24 / 32 / 48.
- Radius: `4` (input) / `8` (kartu) / `12` (panel) / `999` (pill status).
- Hierarki maksimal 3 level per layar. Whitespace murah — jangan ditakuti.

## 5. Motion (Framer Motion)

| Token | Nilai | Pemakaian |
|---|---|---|
| `motion.fast` | 120 ms, easeOut | Hover, toggle, tooltip |
| `motion.base` | 200 ms, easeOut | Transisi antar layar (fade+slide 8px), kartu muncul |
| `motion.spring` | spring stiffness 260 / damping 22 | Angka counter yang men-tick, panel draw |
| `motion.map` | interpolasi 60 fps (requestAnimationFrame) | Posisi rider, pulsa order (easing eksponensial) |

**Aturan:**
1. Motion bermakna data: rider bergerak = posisi baru; pulsa = order masuk; glow merah =
   tekanan sistem. Tidak ada animasi dekoratif tanpa makna.
2. Target **60 fps di laptop menengah** (uji: 2020 dual-core i5/integrated GPU, 100 rider
   aktif + 30 order/menit). Kriteria pemblokir Fase 1 & 5.
3. `prefers-reduced-motion` dihormati: semua non-esensial dimatikan.
4. Feedback kausal < 1 detik: aksi di console → efek terlihat di map/KPI.

## 6. Komponen Dasar (dibangun bertahap sesuai fase)

`StatCard` (angka tick live, getar halus saat SLO langgar) · `LiveChart` (streaming area)
· `StatusPill` · `GaugeSLO` · `TimelineIncident` · `SliderSurge` · `ToggleScenario` ·
`KpiTicker` · `NodeGrid` · `ReplayScrubber` · `MapLegend`.

## 7. Checklist Kualitas UI (lampirkan di DoD fase UI)

- [ ] Semua warna via token; tidak ada hex liar di komponen
- [ ] Semua angka pakai font mono + tabular-nums
- [ ] **Semua copy user-facing bahasa Inggris** (label, aria-label, error text,
      placeholder, narasi demo, reason dispatch — aturan ditambahkan 2026-10-02
      setelah sweep bahasa sesi 13; komentar kode tetap bebas)
- [ ] Kontrol punya `title`/tooltip + panduan in-app (`HelpOverlay`, auto-buka
      sekali untuk pengunjung baru; tombol `? GUIDE` di TopBar)
- [ ] Kontras AA pada seluruh teks
- [ ] Motion 60fps (uji laptop menengah), reduced-motion berfungsi
- [ ] State kosong/loading/error dirancang, bukan kosong mentah
- [ ] Konsistensi radius/spacing 8px grid
- [ ] Responsive: 1440 (utama) / 1024 / 768 minimal tak rusak
