# PHASE 04 — KPI Command Deck + System Health + Chaos

> Spesifikasi fase 4 (ditulis akhir fase 3). Scope & DoD di bawah adalah batas keras
> fase ini. Kriteria UI tetap kriteria pemblokir.

## Tujuan

Dashboard berhenti jadi peta + dua panel: KPI Command Deck (kartu live, streaming
chart, SLO gauge) membuat kesehatan sistem terbaca sekali lirak; System Health
memvisualisasikan pipeline order→Kafka→consumer dan grid node; chaos injector
membunuh node saat trafik — self-heal terlihat di UI + Incident Timeline. Angka
MTTD/MTTR/error budget dari eksperimen nyata, tercatat di `reports/`.

## Scope (kerjakan ini saja)

### Backend
1. **Metrik KPI agregat**: endpoint ringkasan KPI (delivery time live p50/p95,
   utilization, cost/order, orders/min, p99 dispatch, panjang antrean, lag
   konsumen) — dibangun dari metrik yang SUDAH ada (Engine.Metrics, counters
   pipeline `/api/metrics`, metrik strategy-lab). Tanpa fitur baru di core.
2. **SLO & error budget**: definisikan SLO eksplisit (mis. p95 delivery < 6 menit
   pada preset steady; zero message loss; /healthz semua service) + kalkulasi
   budget terpakai dari event.
3. **Chaos injector**: skenario kill-node — matikan/restart service non-kritis
   (atau tambahkan fault injection di ws-gateway/consumer: drop frame, lag konsumen,
   duplikasi pesan) → self-heal restart `unless-stopped` + eksponen di UI.
   DILARANG menyentuh stack lain di host (aturan keras).
4. **Incident Timeline API**: event incident (mulai, deteksi, pulih) persist ke
   file/Postgres — sumber Incident Timeline UI + hitungan MTTD/MTTR.
5. Snapshot wire: field optional baru hanya bila perlu (kontrak tidak breaking).

### Frontend
6. **KPI Command Deck**: kartu live (angka mono tabular), streaming area chart
   (canvas, render on-demand saat idle — data 2 Hz cukup), SLO gauge; kartu bergetar
   halus saat SLO langgar (mati saat reduced-motion).
7. **System Health**: visualisasi pipeline (partikel order→Kafka→consumer, canvas),
   grid status node/service (healthz polling), event autoscaling/health.
8. **Chaos Console + Incident Timeline**: tombol skenario chaos (dengan konfirmasi),
   timeline incident (mulai/deteksi/pulih, MTTD/MTTR per incident).
9. Panel dapat hidup tanpa backend: fallback replay/fixture untuk KPI (angka
   placeholder —`—` — bukan angka karangan).

### Infra & docs
10. Prometheus (:9091, reserved di PORTS.md) + Grafana (:3030) — keputusan D16
    ditinjau ulang di sini: pasang jika RAM budget masih ≤ 2 GB total, kalau tidak
    pertahankan metrik JSON + dokumentasikan alasannya (update ADR).
11. Eksperimen chaos terdokumentasi: `reports/phase-04-chaos.md` (MTTD/MTTR/
    error budget, kondisi hardware, repro).
12. Update docs: PROGRESS, ROADMAP, PHASES/phase-05.md, ADR bila ada keputusan baru.

## Di luar scope (dilarang di fase ini)

- Replay engine penuh (scrub timeline, inspect rider) + Golden Demo presets (Fase 5),
  hardening produksi/README final (Fase 6), AI copilot (Fase 7).

## Definition of Done

- [ ] KPI Command Deck: kartu live + streaming chart + SLO gauge — angka dari metrik
      nyata (bukan dekorasi), mono tabular
- [ ] System Health: pipeline visual + grid node + health events
- [ ] Chaos injector: kill node saat trafik → self-heal terlihat di UI +
      Incident Timeline; eksperimen TIDAK menyentuh stack lain
- [ ] MTTD/MTTR/error budget tercatat dari eksperimen nyata di `reports/`
- [ ] Kualitas UI: token-only colors, kontras AA, reduced-motion penuh, 60fps
      (canvas on-demand), responsive 1440/1024/768
- [ ] Semua service/endpoint baru punya health + limit RAM + CI hijau
- [ ] DoD fase + dokumen diperbarui, commit & push

## Catatan Teknis

- Grafana 3030 / Prometheus 9091 sudah tercatat di PORTS.md — aktifkan dengan
  profile compose sendiri (`observability`) agar demo mode tidak berat.
- Chaos injector = service/compose task terpisah dengan allowlist eksplisit
  container `lastmile-*` — jangan pernah wildcard.
- Streaming chart: window tetap (mis. 5 menit @2 Hz), render hanya saat data baru
  ATAU interaksi — jangan rAF permanen (DoD 60fps idle).
- Incident timeline event schema: {id, t_start, t_detect, t_recover, kind, detail}
  — MTTD = t_detect−t_start, MTTR = t_recover−t_detect (definisi di laporan).
