# PHASE 06 — Produksi: Go-Live, CI/CD Penuh, README & Artikel

> Spesifikasi fase 6 (ditulis akhir fase 5). Scope & DoD di bawah adalah batas
> keras fase ini. Sebagian besar langkah go-live adalah **aksi pemilik akun/
> domain** — fase ini menyiapkan semuanya sehingga aksi pemilik tinggal
> mengeksekusi, lalu diverifikasi.

## Tujuan

lastmile-lab hidup di URL produksi (`lastmile-lab.ricothen.com` + `api.` + `ws.`),
CI/CD dari push sampai deploy berjalan penuh, halaman depan menjual proyek
(README + GIF + tabel metrik + cara reproduksi), dan artikel teknis terbit di
`docs/blog/`. Ini fase "produk jadi di etalase".

## Scope (kerjakan ini saja)

### Go-live & infra
1. **DNS (pemilik)** — `ws.lastmile-lab` & `api.lastmile-lab` → A record IP VPS;
   `lastmile-lab` → CNAME Vercel. Terverifikasi saat `curl https://api.lastmile-lab
   .ricothen.com/healthz` hijau.
2. **Caddy** — site block `deploy/caddy/lastmile-lab.caddy` sudah terpasang;
   verifikasi HTTPS + websocket upgrade di `ws.` setelah DNS aktif. Tanpa
   menyentuh stack lain.
3. **Frontend Vercel (pemilik)** — import repo (root `apps/web`), env
   `NEXT_PUBLIC_WS_URL` / `NEXT_PUBLIC_API_URL` produksi, domain custom.
   Frontend tanpa backend → replay mode (tidak pernah putih).
4. **CI/CD deploy otomatis** — job `deploy` di images.yaml (workflow_dispatch /
   after publish): SSH ke VPS → `docker compose -p lastmile pull && up -d`
   (secret `DEPLOY_SSH_KEY`); atau satu perintah `scripts/deploy.sh` yang aman
   dijalankan manual. Log rotation & `restart: unless-stopped` sudah terpasang.
5. **Monitoring** — UptimeRobot (pemilik) pada `/healthz`; tambahkan ringkasan
   healthz + budget RAM ke runbook (`deploy/README.md`).

### Kualitas & keamanan
6. **Pemeriksaan keamanan dasar**: secret scan (sudah ada di CI) + pastikan
   tidak ada endpoint mutasi yang terbuka tanpa perlu (chaos/lab via `api.` —
   tinjau apakah perlu token sederhana untuk mutasi di publik; minimal:
   dokumenkan risikonya di ADR).
7. **Uji beban produksi ringan** — satu eksperimen loadgen 5 menit di VPS
   (profile `loadtest`) setelah go-live; hasil ke `reports/phase-06-prod.md`.
8. **Audit akhir**: budget RAM ≤ 2 GB terverifikasi, semua `/healthz` hijau
   dari luar (via domain), replay fallback bekerja di domain produksi
   (matikan backend sementara → banner REPLAY).

### Konten etalase
9. **README akhir** — arsitektur (diagram), GIF/screenshot dashboard + demo,
   tabel metrik benchmark (fase 2–5), cara reproduksi (compose up + loadtest +
   bench), penjelasan keputusan utama (ringkas, link ADR).
10. **Artikel teknis** di `docs/blog/` — judul kandidat: "Making a dispatch
    engine explainable: replay, inspect, and a golden demo that never lies" —
    isi: masalah, pendekatan deterministik, replay engine, golden demo, angka.

## Di luar scope (dilarang di fase ini)

- AI Ops Copilot & Plan Advisor (Fase 7); multi-kota; mobile app; fitur engine
  baru apa pun (core beku sejak fase 5).

## Definition of Done

- [ ] HTTPS hidup: `lastmile-lab.ricothen.com` (Vercel) + `api.` + `ws.`
      (Caddy/VPS) — healthz hijau dari internet
- [ ] CI/CD: push → build → image → deploy otomatis (atau satu perintah
      terdokumentasi) — terbukti satu siklus nyata di commit fase ini
- [ ] UptimeRobot /healthz aktif (bukti setup di runbook); log rotation aktif
- [ ] Budget RAM ≤ 2 GB terverifikasi ulang pasca go-live (docker stats di report)
- [ ] Uji beban ringan pasca go-live + laporan `reports/phase-06-prod.md`
- [ ] README final (arsitektur, GIF/screenshot, tabel metrik, reproduksi)
- [ ] Artikel teknis di `docs/blog/` (draft final, siap dibagikan)
- [ ] Pemeriksaan keamanan dasar + keputusan exposure endpoint mutasi (ADR)
- [ ] Commit & push (CI hijau)
