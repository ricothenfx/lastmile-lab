# PHASE 00 — Bootstrap Repo & Fondasi

> Spesifikasi fase aktif. Kerjakan scope ini SAJA. Fase selesai = semua DoD dicek +
> commit + push sukses.

## Tujuan

Repo hidup sebagai source of truth: struktur monorepo, dokumen tata kelola, skeleton
deployment, landing statis bertema design tokens, CI hijau, terdaftar di PORTS.md,
ter-push ke GitHub.

## Scope

1. Struktur folder:
   ```
   lastmile-lab/
   ├── AGENTS.md
   ├── README.md
   ├── .gitignore
   ├── docs/{BLUEPRINT,ROADMAP,DESIGN,PROGRESS}.md
   ├── docs/PHASES/phase-00.md
   ├── apps/web/index.html          (landing statis, token DESIGN.md)
   ├── deploy/{compose.yaml,README.md}
   ├── deploy/caddy/lastmile-lab.caddy
   └── .github/workflows/ci.yaml
   ```
2. Semua dokumen terisi substansi (bukan placeholder kosong).
3. CI: validasi keberadaan docs + parse YAML compose/workflow.
4. Update `/home/rico/PORTS.md`: blok port lastmile + entri keputusan bertanggal.
5. git init (branch main), commit pertama, buat repo GitHub `lastmile-lab` (public —
   tujuan portofolio), push.

## Di luar scope (dilarang dikerjakan di fase ini)

- Scaffold Next.js penuh / npm install berat di VPS (Fase 1, build via CI)
- Menjalankan container compose (Fase 2)
- Menyentuh Caddy produksi (Fase 6 — site block baru dipasang saat backend ada isinya)
- Deploy Vercel (Fase 1, butuh auth akun pemilik)

## Definition of Done

- [x] Semua file scope ada & berisi
- [x] CI hijau di GitHub setelah push (run 36684638419, docs-guard 4s)
- [x] PORTS.md terupdate (blok 3010/3012/3013/3030, 4201–4204, 5434, 6380, 9091, 19092)
- [x] Repo public `lastmile-lab` ada, commit pertama ter-push, default branch main
      (https://github.com/ricothenfx/lastmile-lab, root-commit 90c6e0d)
- [x] `docs/ROADMAP.md` fase 0 ✅, `docs/PROGRESS.md` terupdate, `PHASES/phase-01.md`
      ditulis (spesifikasi awal fase 1)
- [x] Landing statis lolos pemeriksaan visual: dark token sesuai DESIGN.md, teks kontras AA

**STATUS: SELESAI 2026-09-30.**
