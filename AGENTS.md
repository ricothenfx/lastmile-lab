# AGENTS.md — Aturan Main Project lastmile-lab

> **WAJIB dibaca oleh setiap sesi AI / developer sebelum menulis satu baris kode pun.**
> Dokumen ini adalah kontrak kerja. Pelanggaran aturan di sini = pekerjaan dianggap gagal
> meskipun kodenya jalan.

---

## 1. Identitas Project

- **Nama:** lastmile-lab
- **Satu kalimat:** Platform simulasi & benchmark operasi last-mile delivery — dispatch engine
  cerdas, order service anti-surge, control-room dashboard yang hidup.
- **Tujuan bisnis:** Portfolio engineering kelas produksi untuk melamar posisi di perusahaan
  delivery/quick-commerce (Delivery Hero, Just Eat Takeaway, Wolt, Deliveroo, Bolt Food, dkk).
- **Produk fiktif di dalamnya:** "Pulse" — control room untuk operasi last-mile.
- **URL produksi:** https://lastmile-lab.ricothen.com
- **Lokasi di VPS:** `/home/rico/portfolio/lastmile-lab`
- **Docker compose project name:** `lastmile` (SELALU `-p lastmile`, tanpa kecuali)

## 2. Urutan Membaca Dokumen (setiap sesi baru)

1. `AGENTS.md` (file ini)
2. `docs/PROGRESS.md` — status terkini, apa yang sudah/lagi/akan dikerjakan
3. `docs/ROADMAP.md` — peta fase + kriteria selesai
4. `docs/PHASES/phase-0N.md` — spesifikasi fase aktif (scope, task, DoD)
5. `docs/BLUEPRINT.md` + `docs/DESIGN.md` — rujukan arsitektur & visual saat implementasi
6. `deploy/README.md` — runbook VPS saat menyentuh deployment/infra

**Jangan pernah** mengandalkan memori chat sebagai source of truth. Semua keputusan baru
wajib dicatat di `docs/BLUEPRINT.md` (keputusan arsitektur) atau `docs/PROGRESS.md`
(status/log) pada commit yang sama dengan kodenya.

## 3. Ritual Per Fase

1. Baca urutan dokumen di atas. Pahami scope fase aktif. **Kerjakan scope itu saja.**
2. Implementasi + verifikasi **seluruh** DoD (Definition of Done) di spesifikasi fase.
   - Kriteria UI/animasi adalah **kriteria pemblokir**, bukan "nice to have".
3. Jalankan verifikasi yang tersedia (lint, build CI, test). Fase UI: cek visual sebelum push.
4. Commit dengan pesan konvensional: `feat|fix|docs|chore|refactor|test(scope): pesan`.
5. Push ke GitHub. **Fase dinyatakan selesai hanya setelah push sukses.**
6. Dalam commit yang sama: perbarui `docs/PROGRESS.md` (log) + tandai fase di
   `docs/ROADMAP.md` + tulis `docs/PHASES/phase-0(N+1).md` untuk fase berikutnya
   bila spesifikasinya sudah matang.

## 4. Aturan Keras (HARD RULES)

### Keamanan & repo
- **Tidak ada secret di git.** API key/credential hanya lewat env vars / `.env` yang
  di-gitignore. File `.env.example` boleh, isinya placeholder.
- Tidak ada force-push ke `main`. Tidak ada commit langsung yang melompati CI merah.

### VPS (host: vmi3585780, 4 core / 8 GB RAM — RAM tipis, jaga!)
- **Jangan jalankan build berat di VPS** (npm install penuh, compile besar). Build = GitHub
  Actions → image ke GHCR → VPS hanya `docker compose -p lastmile pull && up -d`.
  Dependency kecil untuk tooling dev boleh, asal sadar beban (cek `cat /proc/loadavg` dulu).
- **Dilarang menyentuh stack lain:** `turnaround-prod` (Caddy miliknya), `aviation`,
  `ro-botriv` (API produksi di 3001), project compose lain. Satu-satunya sentuhan yang sah
  ke Caddy: MENAMBAH site block lalu `caddy reload` (lihat `deploy/README.md`).
- **Port:** ikuti alokasi di `/home/rico/PORTS.md`. Blok lastmile: 3010, 3012, 3013, 3030,
  4201–4204, 5434, 6380, 9091, 19092. Mau port baru? Tambah ke PORTS.md dulu, baru pakai.
- Semua container: `-p lastmile`, `restart: unless-stopped`, `mem_limit` + `cpus` eksplisit,
  `healthcheck` + endpoint `/healthz`. Total budget memori stack: **≤ 2 GB**.
- Bind host ports ke `127.0.0.1` kecuali memang di-proxy Caddy.
- Service yang tidak butuh akses dari luar **tidak dipublish ke host sama sekali**
  (jaringan Docker internal saja).

### Arsitektur
- **Core 100% deterministik tanpa LLM.** LLM hanya boleh masuk di Fase 7 (AI Ops Copilot)
  sebagai plugin adapter: tanpa API key → fitur tersembunyi, aplikasi tetap utuh.
- Order/dispatch decisions = algoritma (bukan LLM). p99 keputusan dispatch target < 50 ms.
- Frontend tidak boleh depend pada backend untuk tampil hidup: **replay mode** dari data
  rekaman adalah fallback wajib.
- Semua endpoint frontend via env var (`NEXT_PUBLIC_API_URL` dsb.) — jangan pernah
  hardcode host.

### Kualitas frontend (harga mati)
- Dark theme konsisten dengan `docs/DESIGN.md` (design tokens — jangan hardcoded warna
  di komponen).
- Motion punya makna data, target 60 fps di laptop menengah. Tidak ada animasi dekoratif
  yang mengganggu.
- Setiap angka di dashboard harus punya sumber metrik nyata di sistem (bukan angka hiasan).

## 5. Peta Cepat

| Hal | Lokasi |
|---|---|
| Status kerja terkini | `docs/PROGRESS.md` |
| Peta fase + DoD | `docs/ROADMAP.md` |
| Keputusan arsitektur | `docs/BLUEPRINT.md` (ADR) |
| Design system | `docs/DESIGN.md` |
| Runbook VPS/deploy | `deploy/README.md` |
| Registry port host | `/home/rico/PORTS.md` (di luar repo — sumber kebenaran host) |
| Frontend | `apps/web` |
| Backend services | `apps/services/` (mulai Fase 1–2) |
| CI | `.github/workflows/ci.yaml` |
