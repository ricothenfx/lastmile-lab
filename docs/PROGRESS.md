# PROGRESS — lastmile-lab

> Log status kerja. **Sesi AI/developer baru: baca file ini PERTAMA setelah AGENTS.md.**
> Update setiap selesai pekerjaan berarti, di commit yang sama dengan kodenya.

## Status Saat Ini

- **Fase aktif:** 0 — Bootstrap
- **Kondisi:** eksekusi fase 0 selesai (lihat log di bawah)
- **Langkah berikutnya:** mulai Fase 1 di chat baru (kick-off: baca AGENTS.md → PROGRESS →
  ROADMAP → PHASES/phase-01.md). Fase 1 harus menulis `PHASES/phase-01.md` bila belum ada.
- **Blokir/tergantung user:** none

## Log

### 2026-09-30 — Fase 0: Bootstrap
- Keputusan sudah dikunci sebelum repo (hasil brainstorming + riset Delivery Hero):
  lihat `BLUEPRINT.md` §5 (ADR D1–D11).
- Struktur monorepo dibuat; docs sumber kebenaran ditulis (AGENTS, BLUEPRINT, ROADMAP,
  DESIGN, PHASES/phase-00).
- `deploy/` skeleton: compose.yaml (infra dengan limit sumber daya), site block Caddy
  (api./ws. subdomain), runbook `deploy/README.md`.
- `apps/web/`: landing statis bertema design tokens (deployable ke Vercel tanpa build).
- CI: docs-guard + YAML valid (`.github/workflows/ci.yaml`).
- Blok port lastmile didaftarkan di `/home/rico/PORTS.md` (3010/3012/3013/3030,
  4201–4204, 5434, 6380, 9091, 19092).
- Repo GitHub `lastmile-lab` dibuat (account ricothenfx), commit pertama di-push.

### Keputusan menunggu / catatan
- Deploy Vercel butuh one-time auth akun Vercel pemilik (bisa via dashboard: import repo,
  atau CLI `vercel login`). Baru wajib di Fase 1.
- DNS: `lastmile-lab.ricothen.com` → Vercel; `api.` & `ws.` → IP VPS (record A).
  Detail di `deploy/README.md` §DNS.
