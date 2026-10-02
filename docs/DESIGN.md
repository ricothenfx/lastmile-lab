# DESIGN — Design System "Pulse"

> Sumber kebenaran visual & motion. **Komponen dilarang hardcode warna/ukuran** — semua
> lewat token di file ini (nanti: `apps/web/tokens.ts` + CSS variables). Kriteria UI pada
> DoD tiap fase adalah kriteria pemblokir.

## 1. Konsep

**Mission Control / Air Traffic Control untuk delivery.** Dark theme, data mengalir
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
| `map.road` | `#223047` | Jalan peta (glow halus saat zoom) |
| `zone.hot` | gradien `#FBBF24 → #FB7185` | Heatmap zona panas (bernapas) |

**Kontras:** semua kombinasi teks/surface wajib lolos WCAG AA. Dark theme bukan alasan
kontras rendah.

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
