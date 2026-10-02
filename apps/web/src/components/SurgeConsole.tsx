'use client';

/**
 * Surge Console (Fase 2) — slider surge ×1→×10 + toggle hujan/flash-sale.
 *
 * Aksi → POST api-gateway /api/control/* → sim-control → rider-sim (+loadgen);
 * snapshot berikutnya (≤100 ms) membawa echo su/we → UI tampil < 1 detik
 * (feedback kausal, DESIGN.md §5.4). Angka tampil selalu berasal dari sistem
 * (echo snapshot atau nilai terkirim yang menunggu echo) — bukan angka hias.
 * Warna 100% via token; tanpa animasi rAF (aman 60fps & reduced-motion).
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import type { Snapshot } from '@/lib/protocol';
import type { OpsMode } from '@/lib/useOpsStream';

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://127.0.0.1:3010';
const ECHO_TIMEOUT_MS = 4000;
const RAIN_FACTOR = 0.6;
const FLASH_FACTOR = 8;

type PendingKind = 'surge' | 'weather';

function SurgeConsoleImpl({
  mode,
  stats,
}: {
  mode: OpsMode;
  stats: Snapshot['st'] | null;
}) {
  const [dragValue, setDragValue] = useState<number | null>(null);
  const [pending, setPending] = useState<Partial<Record<PendingKind, number>>>({});
  const [online, setOnline] = useState(true);
  const [flash, setFlash] = useState(false);
  const savedSurge = useRef(1);
  const timers = useRef<Partial<Record<PendingKind, ReturnType<typeof setTimeout>>>>({});

  const echoSurge = stats?.su;
  const echoWeather = stats?.we;
  const live = mode === 'live';
  const disabled = !live;

  // Rekonsiliasi echo: pending dicoret begitu snapshot melapor nilai sama.
  useEffect(() => {
    if (echoSurge !== undefined && pending.surge !== undefined) {
      if (Math.abs(echoSurge - pending.surge) < 0.01) {
        setPending((p) => ({ ...p, surge: undefined }));
      }
    }
    if (echoWeather !== undefined && pending.weather !== undefined) {
      if (Math.abs(echoWeather - pending.weather) < 0.01) {
        setPending((p) => ({ ...p, weather: undefined }));
      }
    }
  }, [echoSurge, echoWeather, pending.surge, pending.weather]);

  useEffect(() => {
    const ts = timers.current;
    return () => Object.values(ts).forEach(clearTimeout);
  }, []);

  const sendControl = useCallback(
    (kind: PendingKind, factor: number) => {
      setPending((p) => ({ ...p, [kind]: factor }));
      clearTimeout(timers.current[kind]);
      timers.current[kind] = setTimeout(() => {
        setPending((p) => ({ ...p, [kind]: undefined }));
        setOnline(false);
      }, ECHO_TIMEOUT_MS);
      fetch(`${API_BASE}/api/control/${kind}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ factor }),
      })
        .then((res) => {
          if (!res.ok) setOnline(false);
        })
        .catch(() => setOnline(false));
    },
    [],
  );

  const surge = dragValue ?? pending.surge ?? echoSurge ?? 1;
  const weather = pending.weather ?? echoWeather ?? 1;
  const raining = weather < 0.999;
  const syncing = pending.surge !== undefined || pending.weather !== undefined;

  const commitSurge = useCallback(() => {
    setDragValue((v) => {
      if (v !== null) sendControl('surge', v);
      return null;
    });
  }, [sendControl]);

  const toggleRain = () => sendControl('weather', raining ? 1 : RAIN_FACTOR);

  const toggleFlash = () => {
    if (flash) {
      setFlash(false);
      sendControl('surge', savedSurge.current);
    } else {
      savedSurge.current = surge;
      setFlash(true);
      sendControl('surge', FLASH_FACTOR);
    }
  };

  let status = { label: 'LIVE LINK', cls: 'text-accent-lime' };
  if (!live) status = { label: mode === 'replay' ? 'REPLAY — LIVE ONLY' : 'STANDBY', cls: 'text-ink-muted' };
  else if (!online) status = { label: 'CTRL OFFLINE', cls: 'text-status-coral' };
  else if (syncing) status = { label: 'SYNCING…', cls: 'text-status-amber' };

  return (
    <aside
      aria-label="Surge Console"
      className="pointer-events-auto absolute right-4 top-4 z-10 w-[224px] rounded-panel border border-line-subtle bg-surface-raised/90 p-4 backdrop-blur"
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
          Surge Console
        </span>
        <span
          aria-label="status"
          className={`font-mono text-[10px] tracking-[0.08em] ${status.cls}`}
        >
          {status.label}
        </span>
      </div>

      <div className="flex items-baseline justify-between">
        <span
          className="font-mono text-[28px] font-semibold leading-none tabular-nums text-ink-primary"
          aria-live="polite"
        >
          ×{surge.toFixed(1)}
        </span>
        {raining && (
          <span className="rounded-pill border border-status-amber/40 px-2 py-0.5 font-mono text-[10px] tracking-[0.08em] text-status-amber">
            RAIN
          </span>
        )}
      </div>

      <input
        type="range"
        min={1}
        max={10}
        step={0.5}
        value={surge}
        disabled={disabled}
        aria-label="Surge factor"
        title="Demand multiplier — applies to the live simulation instantly"
        onChange={(e) => setDragValue(Number(e.target.value))}
        onPointerUp={commitSurge}
        onTouchEnd={commitSurge}
        onKeyUp={(e) => {
          if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown'].includes(e.key)) {
            commitSurge();
          }
        }}
        className="mt-3 w-full cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
        style={{ accentColor: 'var(--accent-cyan)' }}
      />
      <div className="mt-1 flex justify-between font-mono text-[9px] text-ink-muted">
        <span>×1</span>
        <span>DEMAND</span>
        <span>×10</span>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2">
        <button
          type="button"
          aria-pressed={raining}
          onClick={toggleRain}
          disabled={disabled}
          title="Rain: riders slow down 40%"
          className={`rounded-input border px-2 py-1.5 font-mono text-[11px] tracking-[0.06em] transition-colors duration-fast disabled:cursor-not-allowed disabled:opacity-40 ${
            raining
              ? 'border-status-amber/60 bg-status-amber/10 text-status-amber'
              : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
          }`}
        >
          RAIN
        </button>
        <button
          type="button"
          aria-pressed={flash}
          onClick={toggleFlash}
          disabled={disabled}
          title="Flash sale: demand ×8"
          className={`rounded-input border px-2 py-1.5 font-mono text-[11px] tracking-[0.06em] transition-colors duration-fast disabled:cursor-not-allowed disabled:opacity-40 ${
            flash
              ? 'border-accent-cyan/60 bg-accent-cyan/10 text-accent-cyan'
              : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
          }`}
        >
          FLASH SALE
        </button>
      </div>

      <p className="mt-3 font-mono text-[9px] leading-relaxed text-ink-muted">
        EFFECT ×{(echoWeather ?? 1).toFixed(2)} SPEED · ×{(echoSurge ?? 1).toFixed(1)} DEMAND
        {stats?.cr !== undefined && ` · ${stats.cr} ORDERS BORN`}
      </p>
    </aside>
  );
}

const SurgeConsole = memo(SurgeConsoleImpl);
export default SurgeConsole;
