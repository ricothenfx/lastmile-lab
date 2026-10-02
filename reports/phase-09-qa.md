# PHASE 09 REPORT — Autonomous QA Suite + Interview Page

> Tanggal: 2026-10-02 · **verify-all mode local: 33 PASS / 0 FAIL**
> (`reports/verify-local/summary.json` + 9 screenshot) · build produksi
> identik-commit (env `wss://ws.`+`https://api.` dibaked) + backend produksi
> nyata · tsc clean · `/interview` static prerender (96,2 kB first load).

## 1. Halaman `/interview`

- **36 Q&A dalam 8 kategori** (Product & architecture 5 · Kafka & pipeline 5 ·
  Dispatch & algorithms 6 · Frontend craft 6 · Reliability & SRE 4 · Ops & cost
  4 · Security 2 · AI copilot & scale 4) — melebihi target ≥25.
- Setiap jawaban punya baris **evidence** yang menunjuk ADR (BLUEPRINT §5)
  atau angka terukur (`reports/`) — diverifikasi otomatis: 36/36 sitasi.
- Statis murni: server component + `<details>/<summary>` (nol JS klien untuk
  interaksi), nol call backend — filosofi anti-halaman-mati berlaku.
- Link **INTERVIEW** di TopBar (sembunyi di layar < sm) + rujukan di guide.
- Copy 100% Inggris, token-only, AA.

## 2. Suite QA terpadu — `scripts/verify-all.mjs`

Satu command, dua mode, filter `--only`:

```
node scripts/verify-all.mjs --target=local   # penuh: termasuk chaos/duel/demo
node scripts/verify-all.mjs --target=prod    # subset read-only + surge (restore)
```

| Suite | Isi | local | prod |
|---|---|---|---|
| smoke | mode koneksi + healthz | ✓ | ✓ |
| map | style layer + label jalan (probe queryRenderedFeatures) | ✓ | ✓ |
| map-interact | hover tooltip + hit-test < 16 ms + kartu inspect + Escape | ✓ | ✓ |
| heat | probe + surge ×4 (alphaSum naik) + restore ×1 | ✓ | ✓ |
| bursts | counter delivery burst ≤ 40 s | ✓ | ✓ |
| kpi | /api/kpi + angka nyata | ✓ | ✓ |
| replay | panel + inspect reason (regresi fase 5) | ✓ | ✓ |
| offline-fallback | blokir api/ws → REPLAY MODE → pulih | ✓ | ✓ |
| interview | ≥25 Q&A + sitasi + render | ✓ | ✓ |
| copilot | kapabilitas ↔ visibility panel konsisten | ✓ | ✓ |
| a11y-motion | reduced-motion: flag + tooltip tetap hidup | ✓ | ✓ |
| perf | rAF idle 2 s (< 200) | ✓ | ✓ |
| console | nol error aplikasi (+ deteksi 404) | ✓ | ✓ |
| chaos-local | kill strategy-lab → self-heal terukur (202; pulih 3,9–6,6 s) | ✓ | — |
| lab-local | duel fifo vs optimal → status done | ✓ | — |
| demo-local | Golden Demo play → step maju → stop | ✓ | — |

Hasil run terakhir (local): **33 pass / 0 fail** — bukti screenshot:
`reports/verify-local/` (map-labels, inspect, heat-surge4, replay,
interview, reduced-motion, offline-replay, chaos-healed, smoke).

## 3. Bug historis → check permanen

- Z-index overlay peta vs canvas MapLibre (sesi 13) → pixel-probe `__lmPick`
  + probe layer/style.
- Tinggi kontainer peta (fase 5) → wait frame pertama (`__lmPick.riders`).
- Casing `/fonts/Inter/` + filter `['has','n']` (fase 8) → probe glyphs URL,
  layer, dan label ter-render.
- Multi-member gzip replay (fase 5) → unit test backend (dirujuk; bukan
  cek browser).
- innerText ter-transform uppercase CSS → semua cek teks regex
  case-insensitive.
- `next start` cache daftar public saat boot → dicatat (prosedur verifikasi).

## 4. Catatan lingkungan (penting untuk verifikasi --target=prod)

- **Vercel Security Checkpoint** mulai 403 terhadap request dari IP VPS ini
  (datacenter, region sin1) — curl maupun browser headless; terverifikasi
  `"Failed to verify your browser · Code 21"`. Bukan masalah aplikasi:
  monitor eksternal (UptimeRobot) tetap UP dan browser manusia lolos.
  Konsekuensi: jalankan `--target=prod` dari IP lain (laptop pribadi dengan
  docker yang sama), atau pakai mode local (build produksi identik + backend
  produksi nyata — dilakukan untuk fase ini). Dianggap environmental, bukan
  blocker; dicatat di runbook §9.
- Overpass API dari VPS: overpass-api.de 406 (WAF) & kumi 429 → mirror
  maps.mail.ru + kuadran + backoff (lihat `scripts/add-road-names.mjs`).
- `next start` perlu restart setelah mengubah isi `public/` (cache daftar
  file saat boot) — prosedur verifikasi.

## 5. DoD fase 9

- [x] Halaman `/interview` ≥25 Q&A / 8 kategori bersitasi (36/36) — hidup
      di build produksi; statis (tanpa call backend); link TopBar.
- [x] `verify-all` local → 33 PASS / 0 FAIL satu command; mutasi dipulihkan
      (surge ×1, demo di-stop, strategy-lab pulih sendiri).
- [x] Mode prod terimplementasi (subset read-only; lihat §4 soal checkpoint).
- [x] Screenshot + summary.json di `reports/verify-local/`; laporan ini.
- [x] Docs: PROGRESS + ROADMAP fase 9 ✅ — satu commit dengan kodenya.
