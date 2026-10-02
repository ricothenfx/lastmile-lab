'use client';

/**
 * Golden Demo launcher (Fase 5) — daftar preset narasi ±90 detik dengan
 * tombol ▶ Play; saat jalan: banner narasi langkah-demi-langkah + STOP.
 *
 * Demo HANYA aktif saat backend hidup (poll /api/demo/state gagal →
 * tombol mati + label offline). Kill dalam preset memakai chaos injector
 * yang sama — incident tercatat di Incident Timeline (satu sumber
 * kebenaran). Tanpa animasi rAF; warna 100% token.
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import {
  fetchDemoPresets,
  fetchDemoState,
  postDemoPlay,
  postDemoStop,
  type DemoPreset,
  type DemoState,
} from '@/lib/replay';

function fmtClock(sec: number): string {
  const s = Math.max(0, Math.round(sec));
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

function DemoLauncherImpl() {
  const [open, setOpen] = useState(false);
  const [presets, setPresets] = useState<DemoPreset[]>([]);
  const [state, setState] = useState<DemoState | null>(null);
  const [offline, setOffline] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState('');
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = async () => {
      try {
        const st = await fetchDemoState();
        if (!aliveRef.current) return;
        setState(st);
        setOffline(false);
        timer = setTimeout(tick, st.active ? 1000 : 5000);
      } catch {
        if (!aliveRef.current) return;
        setState(null);
        setOffline(true);
        timer = setTimeout(tick, 5000);
      }
    };
    void tick();
    return () => {
      aliveRef.current = false;
      if (timer) clearTimeout(timer);
    };
  }, []);

  const loadPresets = useCallback(() => {
    fetchDemoPresets()
      .then((p) => {
        setPresets(p);
        setErr('');
      })
      .catch(() => setErr('Preset list unreachable'));
  }, []);

  const play = useCallback(async (id: string) => {
    setBusy(true);
    setErr('');
    const res = await postDemoPlay(id);
    if (!res.ok) setErr(res.error ?? 'failed to start demo');
    setBusy(false);
    setOpen(false);
  }, []);

  const active = state?.active ?? false;

  return (
    <>
      {/* tombol dock + dropdown pilihan preset */}
      <div className="pointer-events-auto relative">
        <button
          type="button"
          aria-expanded={open}
          aria-haspopup="menu"
          onClick={() => {
            const next = !open;
            setOpen(next);
            if (next && presets.length === 0) loadPresets();
          }}
          title="Guided 90 s story: surge → rain → chaos kill → self-heal"
          className={`rounded-pill border px-3 py-1.5 font-mono text-[11px] tracking-[0.08em] backdrop-blur transition-colors duration-fast ${
            active
              ? 'border-status-amber/60 bg-status-amber/10 text-status-amber'
              : open
                ? 'border-accent-cyan/60 bg-accent-cyan/10 text-accent-cyan'
                : 'border-line-subtle bg-surface-raised/90 text-ink-secondary hover:text-ink-primary'
          }`}
        >
          ▶ DEMO{offline ? ' · OFFLINE' : active ? ' · LIVE' : ''}
        </button>

        {open && (
          <div
            aria-label="Golden Demo presets"
            className="absolute bottom-[44px] left-1/2 z-30 w-[320px] max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-panel border border-line-subtle bg-surface-raised/95 p-3 backdrop-blur"
          >
            <div className="mb-2 flex items-center justify-between">
              <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
                Golden Demo — 90s guided story
              </span>
              <button
                type="button"
                onClick={() => setOpen(false)}
                aria-label="Close demo list"
                className="font-mono text-[11px] text-ink-muted transition-colors duration-fast hover:text-ink-primary"
              >
                ✕
              </button>
            </div>

            {offline && (
              <p className="mb-2 font-mono text-[10px] leading-relaxed text-status-coral" role="alert">
                BACKEND OFFLINE — GOLDEN DEMO HANYA JALAN SAAT BACKEND HIDUP.
              </p>
            )}
            {err && (
              <p className="mb-2 font-mono text-[10px] text-status-coral" role="alert">
                {err.toUpperCase()}
              </p>
            )}

            <ul className="flex flex-col gap-1.5">
              {presets.map((p) => (
                <li
                  key={p.id}
                  className="flex items-center justify-between gap-2 rounded-card border border-line-subtle bg-surface-overlay/50 px-2.5 py-2"
                >
                  <span className="min-w-0">
                    <span className="block truncate font-mono text-[11px] text-ink-primary">{p.name}</span>
                    <span className="block font-mono text-[9px] leading-relaxed text-ink-muted">
                      {p.desc} · {Math.round(p.duration_s)}s · {p.steps.length} steps
                    </span>
                  </span>
                  <button
                    type="button"
                    disabled={offline || active || busy}
                    onClick={() => void play(p.id)}
                    className="shrink-0 rounded-input border border-accent-cyan/60 bg-accent-cyan/10 px-2.5 py-1 font-mono text-[10px] tracking-[0.08em] text-accent-cyan transition-colors duration-fast hover:bg-accent-cyan/20 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    ▶ PLAY
                  </button>
                </li>
              ))}
              {presets.length === 0 && !offline && (
                <li className="font-mono text-[10px] text-ink-secondary" role="status">
                  LOADING PRESETS…
                </li>
              )}
            </ul>
          </div>
        )}
      </div>

      {/* banner narasi saat demo berjalan (viewport-anchored, di bawah TopBar) */}
      {active && state && (
        <div
          role="status"
          aria-live="polite"
          className="pointer-events-none fixed left-1/2 top-[68px] z-30 w-[min(640px,calc(100vw-2rem))] -translate-x-1/2"
        >
          <div className="pointer-events-auto flex items-center gap-3 rounded-panel border border-status-amber/50 bg-surface-overlay/95 px-4 py-2.5 backdrop-blur">
            <span className="h-2 w-2 shrink-0 rounded-full bg-status-amber" aria-hidden />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-mono text-[11px] text-ink-primary">
                {state.name} — {state.label || 'preparing…'}
              </span>
              <span className="block font-mono text-[9px] tabular-nums text-ink-secondary">
                STEP {state.step}/{state.total_steps} · {fmtClock(state.elapsed_s)} /{' '}
                {fmtClock(state.duration_s)}
              </span>
            </span>
            <button
              type="button"
              onClick={() => void postDemoStop()}
              className="shrink-0 rounded-input border border-status-coral/60 bg-status-coral/10 px-2.5 py-1 font-mono text-[10px] tracking-[0.08em] text-status-coral transition-colors duration-fast hover:bg-status-coral/20"
            >
              ■ STOP
            </button>
          </div>
        </div>
      )}
    </>
  );
}

const DemoLauncher = memo(DemoLauncherImpl);
export default DemoLauncher;
