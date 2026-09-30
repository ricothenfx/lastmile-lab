# PHASE 02 — Order Ingestion + Load Generator + Surge Console

> Spesifikasi fase 2 (ditulis akhir fase 1). Scope & DoD di bawah adalah batas keras
> fase ini. Kriteria UI tetap kriteria pemblokir.

## Tujuan

Pipeline order sungguhan: API ingestion idempotent → Redpanda (Kafka API) → consumer →
dispatch, tahan spike ×10 tanpa kehilangan pesan. Postgres mulai menyimpan order/events.
Surge Console pertama: slider surge + toggle hujan yang bereaksi < 1 detik di UI.
Semua angka (throughput, loss) terdokumentasi di `reports/`.

## Scope (kerjakan ini saja)

### Backend
1. **order-ingestion** (`apps/services/order-ingestion`, :4202 internal):
   - `POST /orders` — validasi, idempotency key (Redis `SET NX` + TTL), backpressure
     (429 saat antrean penuh), publish ke Redpanda topic `orders`.
   - `/healthz`; Redis instance lastmile (6380) — BUKAN 6379 aviation.
2. **Redpanda** aktif (compose sudah ada, profile infra) — topic `orders` (partisi 3,
   RF 1 — single node).
3. **dispatch-consumer**: konsumsi `orders` → Strategy FIFO via interface yang sama
   (dipindah dari rider-sim ke path bersama tanpa refactor frontend) → assign rider.
   Commit offset manual, at-least-once, dedupe by order id.
4. **rider-sim**: sumber order beralih dari generator internal ke konsumsi hasil
   dispatch (flag `ORDER_SOURCE=internal|pipeline`, default tetap internal agar demo
   tidak pernah mati).
5. **Postgres** (:5434): tabel `orders`, `order_events`; consumer + ingestion menulis.
   Migration tool ringan (sql file + job init).
6. **load-generator** (`apps/services/loadgen`): trafik Poisson, rate + surge factor
   via env/flag; skenario `spike` ×10 terjadwal; metrik lokal (sent/acked/429).
7. **sim-control** mini (:3013, loopback): `POST /control/surge {factor}`,
   `POST /control/weather {factor}` — memengaruhi rider-sim + loadgen realtime.

### Frontend
8. **Surge Console** panel: slider surge ×1→×10, toggle hujan/flash-sale; aksi →
   sim-control → efek di map/KPI < 1 detik (feedback kausal, DESIGN.md §5.4).
9. Snapshot wire +1 field cuaca/surge (backward-compatible, optional).

### Infra & docs
10. CI: job backend mencakup service baru; images GHCR +2 service.
11. compose: order-ingestion + loadgen + sim-control (limit memori eksplisit,
    healthcheck, internal kecuali 3013).
12. **Load test terdokumentasi**: spike ×10 → zero message loss (angka di
    `reports/phase-02-loadtest.md`): sent vs acked vs consumed vs stored.
13. Grafana dasar (:3030, loopback) — panel pipeline sederhana (opsional bila RAM
    sempit: metrik JSON endpoint + screenshot).
14. Update docs: PROGRESS, ROADMAP, PHASES/phase-03.md, ADR bila ada keputusan baru.

## Di luar scope (dilarang di fase ini)

- Strategy Lab & strategi tambahan (Fase 3), KPI Deck penuh (Fase 4), chaos injector
  (Fase 4), replay engine penuh (Fase 5).
- Autoscaling/K8s manifests (bonus akhir, bukan syarat fase).

## Definition of Done

- [ ] `POST /orders` idempotent: dua POST dengan key sama → 1 order (terbukti di test)
- [ ] Spike ×10: zero message loss end-to-end (sent = consumed = stored), angka di
      `reports/`
- [ ] Rider bergerak berdasarkan order dari pipeline (flag `pipeline`), fallback
      internal tetap jalan
- [ ] Surge Console: slider ×8 + hujan → efek di map < 1 detik (terverifikasi manual)
- [ ] Postgres berisi order + events yang konsisten dengan snapshot (spot-check SQL)
- [ ] Semua service baru punya `/healthz` + limit RAM + masuk CI hijau
- [ ] Budget RAM total stack ≤ 2 GB saat profile sim+infra aktif (tercatat angkanya)
- [ ] DoD fase + dokumen diperbarui, commit & push

## Catatan Teknis

- RAM tipis: Redpanda 400M + Postgres 384M sudah di compose; loadgen hanya jalan saat
  pengujian (`--profile loadtest`), jangan permanen.
- Kontrak `model.Snapshot` JANGAN diubah secara breaking — frontend fase 1 hidup dari
  situ; tambahan field harus optional.
- Idempotency: key = `Idempotency-Key` header atau hash(konten); Redis TTL 24h.
- Offset Kafka: commit manual per batch setelah DB write sukses (at-least-once).
