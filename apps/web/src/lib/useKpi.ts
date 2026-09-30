'use client';

/**
 * useKpi — satu poller /api/kpi untuk seluruh UI (2 Hz saat hidup, melambat
 * saat tab tersembunyi). Polling ketat 2 Hz sesuai spec fase 4: cukup untuk
 * streaming chart; canvas menggambar hanya saat data baru (tanpa rAF).
 * Payload null/stale → kartu "—" (frontend tetap hidup tanpa backend).
 */

import { useEffect, useRef, useState } from 'react';
import {
  CHART_MAX_POINTS,
  CHART_WINDOW_MS,
  fetchKpi,
  type ChartPoint,
  type KpiPayload,
} from '@/lib/kpi';

export interface KpiState {
  /** Payload terakhir; null = belum pernah / backend tak terjangkau. */
  kpi: KpiPayload | null;
  /** true bila payload terakhir lebih tua dari 3 s (angka tidak fresh). */
  stale: boolean;
  /** Riwayat chart (orders/min + p95) — ring 5 menit @2 Hz. */
  chart: ChartPoint[];
}

const POLL_LIVE_MS = 500; // 2 Hz (spec streaming chart)
const POLL_HIDDEN_MS = 2000;
const STALE_MS = 3000;

export function useKpi(): KpiState {
  const [kpi, setKpi] = useState<KpiPayload | null>(null);
  const [stale, setStale] = useState(true);
  const [chart, setChart] = useState<ChartPoint[]>([]);
  const lastTs = useRef(0);
  const lastAt = useRef(0);

  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const ctrl = new AbortController();

    const tick = async () => {
      if (disposed) return;
      const payload = await fetchKpi(ctrl.signal);
      if (disposed) return;
      if (payload && payload.ts_ms !== lastTs.current) {
        lastTs.current = payload.ts_ms;
        lastAt.current = performance.now();
        setKpi(payload);
        setStale(false);
        setChart((prev) => {
          const next: ChartPoint[] = [
            ...prev,
            {
              ts: payload.ts_ms,
              ordersPerMin: payload.orders_per_min,
              deliveryP95: payload.sim.reachable && payload.sim.delivery_samples > 0
                ? payload.sim.delivery_p95_ms
                : null,
            },
          ];
          // jaga window 5 menit + kapasitas maksimum
          const cut = payload.ts_ms - CHART_WINDOW_MS;
          let from = 0;
          while (from < next.length && next[from].ts < cut) from++;
          let sliced = next.slice(from);
          if (sliced.length > CHART_MAX_POINTS) {
            sliced = sliced.slice(sliced.length - CHART_MAX_POINTS);
          }
          return sliced;
        });
      } else if (!payload || performance.now() - lastAt.current > STALE_MS) {
        setStale(true);
      }
      timer = setTimeout(tick, document.hidden ? POLL_HIDDEN_MS : POLL_LIVE_MS);
    };

    void tick();
    const onVis = () => {
      // tab kembali terlihat → segarkan segera
      if (!document.hidden && timer) {
        clearTimeout(timer);
        timer = setTimeout(tick, 0);
      }
    };
    document.addEventListener('visibilitychange', onVis);
    return () => {
      disposed = true;
      ctrl.abort();
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVis);
    };
  }, []);

  return { kpi, stale, chart };
}
