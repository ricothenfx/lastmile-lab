/**
 * Klien + tipe wire KPI/System Health/Chaos (Fase 4) — mirror api-gateway
 * kpi.go & chaos service. Semua angka berasal dari metrik nyata backend;
 * nilai null = tidak terukur → UI menampilkan "—" (bukan angka karangan).
 */

export const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://127.0.0.1:3010';

export interface KpiSim {
  reachable: boolean;
  now_ms: number;
  riders: number;
  idle_riders: number;
  orders_active: number;
  orders_waiting: number;
  orders_created: number;
  orders_delivered: number;
  orders_expired: number;
  surge: number;
  weather: number;
  strategy: string;
  utilization_pct: number;
  cost_per_order_km: number;
  delivery_p50_ms: number;
  delivery_p95_ms: number;
  delivery_samples: number;
  p50_dispatch_ms: number;
  p99_dispatch_ms: number;
  dispatch_calls: number;
}

export interface GridNode {
  name: string;
  group: 'core' | 'lab' | 'pipeline' | 'chaos';
  status: 'up' | 'down' | 'standby';
  latency_ms: number;
}

export interface SloItem {
  id: 'delivery_p95' | 'zero_loss' | 'grid_health' | 'availability';
  label: string;
  target: string;
  ok: boolean | null;
  value: string | null;
}

export interface ErrorBudget {
  window_sec: number;
  downtime_sec: number;
  consumed_pct: number;
  budget_pct: number;
  availability_pct: number;
  ok: boolean;
}

export interface KpiPayload {
  ts_ms: number;
  source: 'live' | 'partial';
  sim: KpiSim;
  orders_per_min: number | null;
  delivered_per_min: number | null;
  queue: {
    waiting_orders: number;
    ingestion_inflight: number | null;
  };
  pipeline: {
    ingestion_ok: boolean;
    consumer_ok: boolean;
    published: number | null;
    consumed: number | null;
    lag_msgs: number | null;
    publish_errors: number | null;
    db_errors: number | null;
  };
  grid: GridNode[];
  slo: SloItem[];
  error_budget: ErrorBudget | null;
  incidents_open: number | null;
}

export interface ChaosTarget {
  name: string;
  container: string;
  health_url: string;
  status: 'healthy' | 'down' | 'standby';
  last_ok_ms?: number;
  last_probe_ago_ms: number;
}

export interface ChaosIncident {
  id: string;
  target: string;
  kind: 'chaos-kill' | 'health';
  t_start: number;
  t_detect: number;
  t_recover: number;
  detail: string;
}

export interface ChaosIncidents {
  incidents: ChaosIncident[];
  summary: {
    window_start_ms: number;
    window_sec: number;
    incidents_total: number;
    incidents_open: number;
    chaos_kills: number;
    mttd_avg_ms: number;
    mttr_avg_ms: number;
    downtime_total_sec: number;
    monitored_targets: number;
    availability_pct: number;
    budget_pct: number;
    budget_ok: boolean;
  };
}

export async function fetchKpi(signal?: AbortSignal): Promise<KpiPayload | null> {
  try {
    const res = await fetch(`${API_BASE}/api/kpi`, { cache: 'no-store', signal });
    if (!res.ok) return null;
    return (await res.json()) as KpiPayload;
  } catch {
    return null;
  }
}

export async function fetchChaosState(): Promise<ChaosTarget[] | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chaos/state`, { cache: 'no-store' });
    if (!res.ok) return null;
    const data = (await res.json()) as { targets?: ChaosTarget[] };
    return data.targets ?? null;
  } catch {
    return null;
  }
}

export async function fetchChaosIncidents(): Promise<ChaosIncidents | null> {
  try {
    const res = await fetch(`${API_BASE}/api/chaos/incidents`, { cache: 'no-store' });
    if (!res.ok) return null;
    return (await res.json()) as ChaosIncidents;
  } catch {
    return null;
  }
}

export async function postChaosKill(
  target: string,
): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/chaos/kill`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ target }),
    });
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    if (!res.ok) return { ok: false, error: data.error ?? `HTTP ${res.status}` };
    return { ok: true };
  } catch {
    return { ok: false, error: 'chaos unreachable' };
  }
}

/** Data satu titik chart streaming (ring 5 menit @2 Hz di sisi klien). */
export interface ChartPoint {
  ts: number;
  ordersPerMin: number | null;
  deliveryP95: number | null;
}

export const CHART_WINDOW_MS = 5 * 60 * 1000;
export const CHART_MAX_POINTS = 600; // 5 menit @ 2 Hz
