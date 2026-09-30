# PHASE 03 — Dispatch Engine 4 Strategi + Strategy Lab

> Spesifikasi fase 3 (ditulis akhir fase 2). Scope & DoD di bawah adalah batas keras
> fase ini. Kriteria UI tetap kriteria pemblokir.

## Tujuan

Dispatch berhenti jadi satu strategi FIFO: empat strategi di belakang interface
`Strategy` (sudah ada di `pkg/dispatch` sejak Fase 2), dibandingkan secara adil di
Strategy Lab — dua dunia simulasi identik A vs B, delta angka first-class, export
benchmark yang kelak jadi sumber angka README. Target keras: p99 keputusan < 50 ms
pada beban target.

## Scope (kerjakan ini saja)

### Backend
1. **4 strategi** di `pkg/dispatch` (interface + test harness sudah ada):
   - `fifo` (ada, jadi baseline),
   - `batching` — kumpulkan order N ms / N order, grupkan per pickup terdekat,
   - `zone` — partisi peta ke grid/zona, order di-assign ke rider dalam zona,
     lintas zona hanya bila tak ada kandidat,
   - `optimal` — assignment bipartite min-cost (Hungarian atau OR-Tools; pilih
     yang paling ringan di RAM — Hungarian murni Go lebih disukai).
   Semua strategi punya `Reason` explainability + deterministik (seed).
2. **p99 dispatch benchmark**: harness yang mengukur waktu `Assign()` per tick pada
   beban target (≥ 100 order aktif + 100 rider); hasil ke `reports/phase-03-*.md`.
3. **Duel engine**: mode menjalankan dua engine identik (seed sama, input order
   sama, strategi beda) — reuse virtual clock; input order di-replay identik ke
   kedua engine (dari generator seed sama atau dari fixture).
4. **Metrik per engine**: delivery time p50/p95 (created→delivered), utilization
   rider (%, waktu bawa tugas), cost/order (jarak tempuh × tarif unit), expired,
   throughput. Tersedia via REST (`/api/lab/*`) + snapshot field optional bila perlu.
5. API: `POST /api/lab/run {strategy_a, strategy_b, preset, seconds}` → job async →
   hasil tersimpan (in-memory/file) + `GET /api/lab/results`.

### Frontend
6. **Strategy Lab panel**: pilih strategi A & B + preset skenario, tombol RUN →
   peta replay kembar (dua canvas mini, data hasil duel, bukan live), tabel delta
   (delivery time, utilization, cost/order — mono tabular), histogram overlay
   delivery time A vs B. Reduced-motion penuh; 60fps (dua canvas kecil, render
   on-demand — bukan loop per frame saat idle).
7. Snapshot wire: field optional baru hanya bila perlu (kontrak tidak breaking).

### Infra & docs
8. CI: job backend tetap mencakup semuanya; benchmark jalan manual (bukan CI gate).
9. Load/bench test terdokumentasi: `reports/phase-03-bench.md` (p99 per strategi,
   tabel delta duel, kondisi hardware).
10. Update docs: PROGRESS, ROADMAP, PHASES/phase-04.md, ADR bila ada keputusan baru
    (mis. pilihan Hungarian vs OR-Tools).

## Di luar scope (dilarang di fase ini)

- KPI Command Deck penuh + System Health + chaos injector (Fase 4), replay engine
  penuh + Golden Demo presets (Fase 5), autoscaling/K8s.

## Definition of Done

- [ ] 4 strategi jalan & deterministik (unit test: urutan, sifat unik assignment,
      determinism same-seed per strategi)
- [ ] p99 keputusan dispatch terukur **< 50 ms** pada beban target — angka di `reports/`
- [ ] Strategy Lab: A vs B skenario identik → peta kembar + tabel delta + histogram
      overlay, angka first-class (delivery time, utilization, cost/order)
- [ ] Export benchmark report (file di `reports/`, format dipakai README nanti)
- [ ] Kualitas UI: semua warna via token, mono tabular, kontras AA, reduced-motion,
      60fps saat idle & saat render hasil
- [ ] Semua service/endpoint baru punya health + limit RAM + CI hijau
- [ ] DoD fase + dokumen diperbarui, commit & push

## Catatan Teknis

- Interface `Strategy.Assign(orders, riders)` dipertahankan apa adanya — strategi
  baru TIDAK boleh mengubah engine (Fase 2 sudah membuktikan pola ini).
- Duel engine harus byte-identical input: gunakan satu generator order (seed sama)
  yang mem-pipe order yang sama ke dua engine per tick — bukan dua generator terpisah.
- `cost/order`: definisikan eksplisit di report (mis. jarak tempuh meter per order
  terkirim × 1 unit biaya/km) — jangan angka karangan tanpa definisi.
- Histogram overlay: bin bersama untuk A & B (sama sumbu), render canvas statis
  sekali saat data siap.
- OR-Tools = binding C++ (berat, CGO) → hindari; Hungarian O(n³) murni Go cukup
  untuk n ≤ 500 dan bebas dependensi.
