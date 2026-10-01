/**
 * Klien + tipe wire Replay Engine & Golden Demo (Fase 5) — mirror
 * internal/replay (rider-sim) dan demo.go (api-gateway). Frame replay
 * memakai bentuk Snapshot yang sama dengan live stream (protocol.ts).
 */
import type { Snapshot } from './protocol';
import type { RenderPair } from './useOpsStream';
import { API_BASE } from './lab';

/** Pasangan render + key frame (untuk guard render on-demand LiveMap). */
export type FramePair = RenderPair & { key?: number };

/** Entitas yang diklik di peta (inspect). */
export interface MapPick {
  kind: 'rider' | 'order';
  id: number | string;
}

/** Override replay yang dipasang panel ke aliran render peta. */
export interface ReplayOverride {
  /** Null = override tidak aktif → peta memakai pasangan live/fixture. */
  getPair: (now: number) => FramePair | null;
}

export interface ReplayMeta {
  id: string;
  seed: number;
  strategy: string;
  riders: number;
  tick_hz: number;
  started_wall_ms: number;
  hz: number;
  max_seconds: number;
}

export interface ReplaySessionInfo {
  id: string;
  frames: number;
  frame_bytes: number;
  t_first: number;
  t_last: number;
  wall_first_ms: number;
  wall_last_ms: number;
  seconds_retained: number;
  decisions: number;
  meta: ReplayMeta;
}

/** Keputusan dispatch — mirror model.Decision (explainability). */
export interface Decision {
  seq: number;
  t: number;
  strategy: string;
  order: string;
  rider: number;
  dist_m: number;
  reason: string;
}

export interface ReplayDump {
  meta: ReplayMeta;
  wall0: number;
  t0: number;
  count: number;
  frames: Snapshot[];
  decisions: Decision[];
}

export async function fetchReplaySessions(): Promise<ReplaySessionInfo[]> {
  const res = await fetch(`${API_BASE}/api/replay/sessions`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = (await res.json()) as { sessions?: ReplaySessionInfo[] };
  return data.sessions ?? [];
}

export async function fetchReplayDump(id = 'live'): Promise<ReplayDump> {
  const res = await fetch(`${API_BASE}/api/replay/sessions/${encodeURIComponent(id)}`, {
    cache: 'no-store',
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as ReplayDump;
}

/** Incident chaos untuk marker timeline — mirror chaos.Incident (Fase 4). */
export interface ChaosIncident {
  id: string;
  target: string;
  kind: string; // "chaos-kill" | "health"
  t_start: number;
  t_detect: number;
  t_recover: number;
  detail: string;
}

export async function fetchIncidents(): Promise<ChaosIncident[]> {
  const res = await fetch(`${API_BASE}/api/chaos/incidents`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = (await res.json()) as { incidents?: ChaosIncident[] };
  return data.incidents ?? [];
}

// ---- Golden Demo ----

export interface DemoStep {
  at_s: number;
  label: string;
  kind: 'reset' | 'surge' | 'weather' | 'kill';
  value?: number;
  target?: string;
}

export interface DemoPreset {
  id: string;
  name: string;
  desc: string;
  duration_s: number;
  steps: DemoStep[];
}

export interface DemoState {
  active: boolean;
  id?: string;
  name?: string;
  step: number;
  total_steps: number;
  label?: string;
  elapsed_s: number;
  duration_s: number;
  step_status?: string[];
  last_id?: string;
}

export async function fetchDemoPresets(): Promise<DemoPreset[]> {
  const res = await fetch(`${API_BASE}/api/demo/presets`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = (await res.json()) as { presets?: DemoPreset[] };
  return data.presets ?? [];
}

export async function fetchDemoState(): Promise<DemoState> {
  const res = await fetch(`${API_BASE}/api/demo/state`, { cache: 'no-store' });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as DemoState;
}

export async function postDemoPlay(id: string): Promise<{ ok: boolean; error?: string }> {
  const res = await fetch(`${API_BASE}/api/demo/play`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    return { ok: false, error: data.error ?? `HTTP ${res.status}` };
  }
  return { ok: true };
}

export async function postDemoStop(): Promise<void> {
  await fetch(`${API_BASE}/api/demo/stop`, { method: 'POST' }).catch(() => undefined);
}

/** Fixture replay fase 1 — fallback penuh saat backend mati. */
export async function fetchFixtureFrames(): Promise<{ frames: Snapshot[]; t0: number }> {
  const res = await fetch('/fixtures/replay-sample.json');
  if (!res.ok) throw new Error(`fixture ${res.status}`);
  const data = (await res.json()) as { frames?: Snapshot[] };
  const frames = Array.isArray(data.frames) ? data.frames : [];
  if (frames.length === 0) throw new Error('fixture kosong');
  return { frames, t0: frames[0].t };
}

/** Binary search frame terakhir dengan t <= simT (pola MiniReplay fase 3). */
export function frameIndexAt(frames: Snapshot[], simT: number): number {
  let lo = 0;
  let hi = frames.length - 1;
  if (frames.length < 2) return lo;
  while (lo < hi - 1) {
    const mid = (lo + hi) >> 1;
    if (frames[mid].t <= simT) lo = mid;
    else hi = mid;
  }
  return lo;
}
