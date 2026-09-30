'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import type { Snapshot } from './protocol';

export type OpsMode = 'connecting' | 'live' | 'replay';

export interface RenderPair {
  prev: Snapshot | null;
  next: Snapshot | null;
  /** 0..1 — fraksi interpolasi prev→next (selalu 1 saat reduced motion). */
  alpha: number;
}

export interface OpsStream {
  mode: OpsMode;
  stats: Snapshot['st'] | null;
  getPair: (now: number) => RenderPair;
}

const WS_URL = process.env.NEXT_PUBLIC_WS_URL ?? 'ws://127.0.0.1:3012/ws';
const CONNECT_TIMEOUT_MS = 3000; // DoD: WS tak tersambung 3 detik → replay
const FRESH_MS = 3000;
const RETRY_LIVE_MS = 6000;
const MAX_FRAMES = 3;
const STATS_ECHO_MS = 250; // throttle angka live (Surge Console echo < 1s, TopBar tetap murah)

/**
 * Satu sumber data untuk seluruh UI:
 *  - live: WebSocket snapshot 10 Hz, disisipkan pasangan frame untuk
 *    interpolasi 60fps di canvas (tidak pernah memicu React render per frame)
 *  - replay: fixture rekaman diputar loop bila WS tak hidup dalam 3 detik,
 *    sambil terus mencoba tersambung kembali
 */
export function useOpsStream(): OpsStream {
  const [mode, setMode] = useState<OpsMode>('connecting');
  const [stats, setStats] = useState<Snapshot['st'] | null>(null);

  const frames = useRef<{ snap: Snapshot; at: number }[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const modeRef = useRef<OpsMode>('connecting');
  const replayRef = useRef<{ frames: Snapshot[]; t0: number; start: number } | null>(null);
  const lastFrameAt = useRef(0);
  const lastStatsAt = useRef(0);
  const statsRef = useRef<Snapshot['st'] | null>(null);

  const applyMode = useCallback((m: OpsMode) => {
    modeRef.current = m;
    setMode(m);
  }, []);

  const getPair = useCallback((now: number): RenderPair => {
    const rp = replayRef.current;
    if (modeRef.current === 'replay' && rp && rp.frames.length >= 2) {
      const span = rp.frames[rp.frames.length - 1].t - rp.t0;
      if (span > 0) {
        const simT = rp.t0 + ((now - rp.start) % span);
        let lo = 0;
        let hi = rp.frames.length - 1;
        while (lo < hi - 1) {
          const mid = (lo + hi) >> 1;
          if (rp.frames[mid].t <= simT) lo = mid;
          else hi = mid;
        }
        const prev = rp.frames[lo];
        const next = rp.frames[hi];
        const alpha = next.t > prev.t ? (simT - prev.t) / (next.t - prev.t) : 0;
        return { prev, next, alpha };
      }
    }
    const f = frames.current;
    if (f.length === 0) return { prev: null, next: null, alpha: 0 };
    if (f.length === 1) return { prev: f[0].snap, next: f[0].snap, alpha: 0 };
    const p = f[f.length - 2];
    const n = f[f.length - 1];
    const interval = Math.max(1, n.at - p.at);
    const alpha = Math.min(1, Math.max(0, (now - n.at) / interval));
    return { prev: p.snap, next: n.snap, alpha };
  }, []);

  const startReplay = useCallback(async () => {
    if (replayRef.current) {
      applyMode('replay');
      return;
    }
    try {
      const res = await fetch('/fixtures/replay-sample.json');
      if (!res.ok) throw new Error(`fixture ${res.status}`);
      const data = await res.json();
      const fs: Snapshot[] = Array.isArray(data.frames) ? data.frames : [];
      if (fs.length === 0) throw new Error('fixture kosong');
      replayRef.current = { frames: fs, t0: fs[0].t, start: performance.now() };
      statsRef.current = fs[fs.length - 1].st ?? null;
      setStats(statsRef.current);
      applyMode('replay');
    } catch {
      // fixture gagal dimuat — tetap connecting, banner menunggu feed live
    }
  }, [applyMode]);

  useEffect(() => {
    const startedAt = performance.now();
    let disposed = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    const retryTimers: ReturnType<typeof setTimeout>[] = [];
    const setRetry = (fn: () => void, ms: number) => {
      retryTimer = setTimeout(fn, ms);
      retryTimers.push(retryTimer);
    };

    const clearWs = () => {
      const ws = wsRef.current;
      wsRef.current = null;
      if (ws) {
        ws.onopen = ws.onmessage = ws.onclose = ws.onerror = null;
        try {
          ws.close();
        } catch {
          /* sudah tutup */
        }
      }
    };

    const connect = () => {
      if (disposed || wsRef.current) return;
      let opened = false;
      let ws: WebSocket;
      try {
        ws = new WebSocket(WS_URL);
      } catch {
        setRetry(connect, RETRY_LIVE_MS);
        return;
      }
      wsRef.current = ws;
      const openTimeout = setTimeout(() => {
        if (!opened) {
          try {
            ws.close();
          } catch {
            /* noop */
          }
        }
      }, CONNECT_TIMEOUT_MS);

      ws.onopen = () => {
        opened = true;
        clearTimeout(openTimeout);
        replayRef.current = null;
        applyMode('live');
      };
      ws.onmessage = (ev) => {
        let snap: Snapshot;
        try {
          snap = JSON.parse(ev.data as string) as Snapshot;
        } catch {
          return;
        }
        if (typeof snap?.t !== 'number' || !Array.isArray(snap?.r)) return;
        lastFrameAt.current = performance.now();
        const f = frames.current;
        f.push({ snap, at: lastFrameAt.current });
        while (f.length > MAX_FRAMES) f.shift();
        if (modeRef.current !== 'live') applyMode('live');
        if (lastFrameAt.current - lastStatsAt.current > STATS_ECHO_MS) {
          lastStatsAt.current = lastFrameAt.current;
          statsRef.current = snap.st ?? null;
          setStats(statsRef.current);
        }
      };
      ws.onclose = () => {
        clearTimeout(openTimeout);
        if (wsRef.current === ws) wsRef.current = null;
        if (!disposed) {
          setRetry(connect, RETRY_LIVE_MS);
        }
      };
      ws.onerror = () => {
        try {
          ws.close();
        } catch {
          /* noop */
        }
      };
    };

    connect();

    // Watchdog: WS tak hidup dalam 3 detik ATAU frame berhenti → replay mode
    // (DoD fallback), sambil terus mencoba live kembali.
    const watchdog = setInterval(() => {
      if (disposed) return;
      const now = performance.now();
      const fresh = now - lastFrameAt.current < FRESH_MS;
      if (modeRef.current === 'live' && !fresh) applyMode('connecting');
      if (
        !fresh &&
        modeRef.current !== 'replay' &&
        now - startedAt >= CONNECT_TIMEOUT_MS
      ) {
        void startReplay();
      }
    }, 1000);

    return () => {
      disposed = true;
      clearInterval(watchdog);
      retryTimers.forEach(clearTimeout);
      clearWs();
    };
  }, [applyMode, startReplay]);

  return { mode, stats, getPair };
}
