/**
 * Klien + tipe wire AI Ops Copilot (Fase 7) — mirror service copilot (Go).
 * Prinsip keras: panel hanya dirender bila capabilities {"enabled":true};
 * tanpa API key TIDAK ADA elemen copilot di DOM (ADR D24). LLM tidak pernah
 * mengeksekusi — Execute memakai endpoint kontrol existing (sim-control/chaos).
 */

import { useEffect, useState } from 'react';

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://127.0.0.1:3010';

export interface CopilotAction {
  kind: 'surge' | 'weather' | 'strategy' | 'kill';
  params: Record<string, string>;
}

export interface CopilotPlan {
  name: string;
  rationale: string;
  actions: CopilotAction[];
}

export interface DryrunMetrics {
  created: number;
  delivered: number;
  expired: number;
  delivery_p50_ms: number;
  delivery_p95_ms: number;
  cost_per_order_km: number;
  utilization_pct: number;
  throughput_per_min: number;
}

export interface CopilotPlanResult {
  plan: CopilotPlan;
  baseline: DryrunMetrics | null;
  predicted?: DryrunMetrics | null;
  note?: string;
}

export interface CopilotAdvise {
  seed: number;
  seconds: number;
  base_rate_per_min: number;
  baseline: DryrunMetrics;
  plans: CopilotPlanResult[];
  model?: string;
  took_ms?: number;
}

export interface CopilotSource {
  id: string;
  label: string;
  value: string;
}

export interface CopilotAnswer {
  text: string;
  sources: CopilotSource[];
  model?: string;
  took_ms?: number;
}

/** Deteksi sekali per mount — false/404/error = sembunyikan total. */
export async function fetchCopilotCapabilities(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/api/copilot/capabilities`, { cache: 'no-store' });
    if (!res.ok) return false;
    const data = (await res.json()) as { enabled?: boolean };
    return data.enabled === true;
  } catch {
    return false;
  }
}

/** Hook capability — default false (tersembunyi) sampai backend bilang enabled. */
export function useCopilotEnabled(): boolean {
  const [enabled, setEnabled] = useState(false);
  useEffect(() => {
    let alive = true;
    fetchCopilotCapabilities().then((v) => {
      if (alive) setEnabled(v);
    });
    return () => {
      alive = false;
    };
  }, []);
  return enabled;
}

export async function postCopilotAdvise(): Promise<{ data?: CopilotAdvise; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/copilot/advise`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
    const json = (await res.json().catch(() => ({}))) as { error?: string } & CopilotAdvise;
    if (!res.ok) return { error: json.error ?? `HTTP ${res.status}` };
    return { data: json };
  } catch {
    return { error: 'copilot unreachable' };
  }
}

export async function postCopilotAsk(
  question: string,
): Promise<{ data?: CopilotAnswer; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/copilot/ask`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ question }),
    });
    const json = (await res.json().catch(() => ({}))) as { error?: string } & CopilotAnswer;
    if (!res.ok) return { error: json.error ?? `HTTP ${res.status}` };
    return { data: json };
  } catch {
    return { error: 'copilot unreachable' };
  }
}

/** Aksi kontrol live via endpoint EXISTING — bukan LLM yang mengeksekusi. */
export async function executeAction(
  action: CopilotAction,
): Promise<{ ok: boolean; error?: string; skipped?: boolean }> {
  const post = async (url: string, body: unknown) => {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const data = (await res.json().catch(() => ({}))) as { error?: string };
      return { ok: false, error: data.error ?? `HTTP ${res.status}` };
    }
    return { ok: true };
  };
  try {
    switch (action.kind) {
      case 'surge':
        return await post(`${API_BASE}/api/control/surge`, { factor: Number(action.params.factor) });
      case 'weather':
        return await post(`${API_BASE}/api/control/weather`, { factor: Number(action.params.factor) });
      case 'kill':
        return await post(`${API_BASE}/api/chaos/kill`, { target: action.params.target });
      case 'strategy':
        // Tidak ada endpoint kontrol strategi live — butuh restart service
        // (DISPATCH_STRATEGY env). Jujur: ditandai skipped, bukan gagal diam.
        return { ok: false, skipped: true, error: 'strategy butuh restart (env DISPATCH_STRATEGY)' };
      default:
        return { ok: false, error: 'aksi tidak dikenal' };
    }
  } catch {
    return { ok: false, error: 'control unreachable' };
  }
}
