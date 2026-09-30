/**
 * Klien + tipe wire Strategy Lab (Fase 3) — mirror internal/duel (backend Go).
 * Frame replay memakai bentuk Snapshot yang sama dengan live stream
 * (protocol.ts) sehingga renderer peta bisa berbagi logika gambar.
 */
import type { Snapshot } from './protocol';

export const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://127.0.0.1:3010';

export const STRATEGIES = ['fifo', 'batching', 'zone', 'optimal'] as const;
export type StrategyName = (typeof STRATEGIES)[number];

export const PRESETS = ['steady', 'flash', 'rush'] as const;
export type PresetName = (typeof PRESETS)[number];

export interface LabSide {
  strategy: string;
  created: number;
  delivered: number;
  expired: number;
  delivery_p50_ms: number;
  delivery_p95_ms: number;
  utilization_pct: number;
  task_km: number;
  cost_per_order_km: number;
  throughput_per_min: number;
  delivery_durs_ms?: number[];
  frames?: Snapshot[];
}

export interface LabHistogram {
  edges_ms: number[];
  count_a: number[];
  count_b: number[];
}

export interface LabResult {
  id: string;
  status: 'running' | 'done' | 'error';
  progress_pct?: number;
  error?: string;
  strategy_a: string;
  strategy_b: string;
  preset: string;
  preset_note?: string;
  seed: number;
  seconds: number;
  riders: number;
  rate_per_min: number;
  ttl_sec: number;
  created_at: string;
  duration_wall_ms?: number;
  a?: LabSide;
  b?: LabSide;
  histogram?: LabHistogram;
}

export interface LabSummary {
  id: string;
  status: LabResult['status'];
  progress_pct: number;
  error?: string;
  strategy_a: string;
  strategy_b: string;
  preset: string;
  created_at: string;
  seconds: number;
  delivered_a?: number;
  delivered_b?: number;
  delivery_p50_a_ms?: number;
  delivery_p50_b_ms?: number;
  duration_wall_ms?: number;
}

export async function postLabRun(body: {
  strategy_a: string;
  strategy_b: string;
  preset: string;
  seconds: number;
}): Promise<{ id?: string; error?: string }> {
  const res = await fetch(`${API_BASE}/api/lab/run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const data = (await res.json().catch(() => ({}))) as { id?: string; error?: string };
  if (!res.ok) return { error: data.error ?? `HTTP ${res.status}` };
  return data;
}

export async function fetchLabResults(): Promise<LabSummary[]> {
  const res = await fetch(`${API_BASE}/api/lab/results`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = (await res.json()) as { results?: LabSummary[] };
  return data.results ?? [];
}

export async function fetchLabResult(id: string): Promise<LabResult> {
  const res = await fetch(`${API_BASE}/api/lab/results/${encodeURIComponent(id)}`, {
    cache: 'no-store',
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as LabResult;
}

/** Peta graph (nodes+edges) untuk latar mini-map — dimuat sekali, di-cache. */
export interface GraphJSON {
  meta: { bbox: number[] };
  nodes: [number, number][]; // [lat, lon]
  edges: [number, number, number][];
}

let graphPromise: Promise<GraphJSON> | null = null;

export function getGraph(): Promise<GraphJSON> {
  if (!graphPromise) {
    graphPromise = fetch(`${API_BASE}/api/graph`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json() as Promise<GraphJSON>;
      })
      .catch((err) => {
        graphPromise = null; // boleh dicoba ulang nanti
        throw err;
      });
  }
  return graphPromise;
}
