'use client';

/**
 * Replay & Inspect panel (Fase 5).
 *
 * Scrub timeline ke detik mana pun + PLAY/PAUSE (×1/×4/×16) atas sesi
 * rekaman rider-sim (buffer 15 menit @ 5 Hz) — atau fixture fase 1 saat
 * backend mati (fallback tetap hidup 100%).
 *
 * RENDER ON-DEMAND (pola MiniReplay fase 3): panel TIDAK punya rAF sendiri.
 * Scrub/pause menggambar hanya saat frame berubah (guard key di LiveMap);
 * PLAY memajukan playhead dari wall-clock sehingga loop rAF peta yang SUDAH
 * ada yang merender — tanpa rAF idle baru. Reduced-motion = scrub statis.
 *
 * Inspect: klik rider → kartu detail (status, order dibawa, pickup/dropoff,
 * ALASAN keputusan dispatch dari ring decisions); klik order → status + umur.
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import { useReducedMotion } from 'framer-motion';
import { ORDER_STATUS, RIDER_STATUS, type Snapshot } from '@/lib/protocol';
import {
  fetchFixtureFrames,
  fetchIncidents,
  fetchReplayDump,
  fetchReplaySessions,
  frameIndexAt,
  type ChaosIncident,
  type MapPick,
  type ReplayOverride,
  type ReplaySessionInfo,
} from '@/lib/replay';
import type { LiveStreamRef } from './LiveMap';

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? 'http://127.0.0.1:3010';
const SPEEDS = [1, 4, 16] as const;

type Source =
  | { kind: 'session'; frames: Snapshot[]; t0: number; info: ReplaySessionInfo; decisions: ReplayDumpLite['decisions'] }
  | { kind: 'fixture'; frames: Snapshot[]; t0: number }
  | null;

type ReplayDumpLite = Awaited<ReturnType<typeof fetchReplayDump>>;

const RIDER_LABEL = ['IDLE', 'TO PICKUP', 'PICKUP', 'DELIVERING'] as const;
const RIDER_CLS = [
  'text-accent-lime',
  'text-status-amber',
  'text-accent-cyan',
  'text-status-violet',
] as const;
const ORDER_LABEL = ['WAITING', 'ASSIGNED', 'IN TRANSIT'] as const;
const ORDER_CLS = ['text-accent-cyan', 'text-status-amber', 'text-status-violet'] as const;

function fmtT(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

interface Props {
  streamRef: React.MutableRefObject<LiveStreamRef>;
  overrideRef: React.MutableRefObject<ReplayOverride>;
}

function ReplayPanelImpl({ streamRef, overrideRef }: Props) {
  const reduced = useReducedMotion() ?? false;
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [errorMsg, setErrorMsg] = useState('');
  const [source, setSource] = useState<Source>(null);
  const [incidents, setIncidents] = useState<ChaosIncident[]>([]);
  const [simT, setSimT] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState<number>(1);
  const [selected, setSelected] = useState<MapPick | null>(null);

  const sourceRef = useRef<Source>(null);
  const simTRef = useRef(0);
  const playingRef = useRef(false);
  const speedRef = useRef(1);
  const anchor = useRef({ simT: 0, wall: 0 });
  const engagedRef = useRef(false); // override aktif (scrub/play) vs map live
  const firstSeen = useRef<Map<string, number>>(new Map());

  const span = source ? source.frames[source.frames.length - 1].t - source.t0 : 0;
  const active = open && status === 'ready';

  // Pasang sumber + first-seen index (umur order).
  const adoptSource = useCallback((s: Source) => {
    sourceRef.current = s;
    firstSeen.current = new Map();
    if (s) {
      for (const f of s.frames) {
        for (const o of f.o) {
          if (!firstSeen.current.has(o.i)) firstSeen.current.set(o.i, f.t);
        }
      }
      simTRef.current = s.t0;
      anchor.current = { simT: s.t0, wall: performance.now() };
    }
    setSource(s);
    setSimT(s ? s.t0 : 0);
  }, []);

  const load = useCallback(async () => {
    setStatus('loading');
    setErrorMsg('');
    setPlaying(false);
    playingRef.current = false;
    engagedRef.current = false;
    setSelected(null);
    streamRef.current.selected = null;
    try {
      const list = await fetchReplaySessions();
      const info = list[0];
      if (!info || info.frames === 0) throw new Error('replay buffer empty — waiting for recordings');
      const dump = await fetchReplayDump(info.id);
      if (!dump.frames?.length) throw new Error('empty frame dump');
      adoptSource({ kind: 'session', frames: dump.frames, t0: dump.t0, info, decisions: dump.decisions });
      fetchIncidents()
        .then(setIncidents)
        .catch(() => setIncidents([]));
      setStatus('ready');
    } catch {
      // Backend mati / belum ada buffer → fixture fase 1 (fallback wajib).
      try {
        const fx = await fetchFixtureFrames();
        adoptSource({ kind: 'fixture', frames: fx.frames, t0: fx.t0 });
        setIncidents([]);
        setStatus('ready');
      } catch (err) {
        setStatus('error');
        setErrorMsg(err instanceof Error ? err.message : 'replay source unreachable');
      }
    }
  }, [adoptSource]);

  // Muat sekali saat panel dibuka; lepas override saat ditutup.
  useEffect(() => {
    if (open && status === 'idle') void load();
    if (!open) {
      setPlaying(false);
      playingRef.current = false;
      engagedRef.current = false;
      setSelected(null);
      streamRef.current.onMapClick = null;
      streamRef.current.selected = null;
    }
  }, [open, status, load, streamRef]);

  // Override getPair: playhead maju dari wall-clock (tanpa rAF panel).
  useEffect(() => {
    overrideRef.current.getPair = (now: number) => {
      const src = sourceRef.current;
      if (!open || !engagedRef.current || !src || src.frames.length === 0) return null;
      const tEnd = src.frames[src.frames.length - 1].t;
      let t: number;
      if (playingRef.current) {
        t = anchor.current.simT + (now - anchor.current.wall) * speedRef.current;
        if (t >= tEnd) t = tEnd;
        simTRef.current = t;
      } else {
        t = simTRef.current;
      }
      const i = frameIndexAt(src.frames, t);
      const prev = src.frames[i];
      const next = src.frames[Math.min(i + 1, src.frames.length - 1)];
      const a = next.t > prev.t ? (t - prev.t) / (next.t - prev.t) : 0;
      return { prev, next, alpha: reduced ? 1 : Math.min(1, Math.max(0, a)), key: t };
    };
    return () => {
      overrideRef.current.getPair = () => null;
    };
  }, [open, reduced, overrideRef]);

  // Pelabel waktu + auto-pause di ujung — setInterval murah, bukan rAF.
  useEffect(() => {
    if (!active) return;
    const iv = setInterval(() => {
      if (playingRef.current) {
        setSimT(simTRef.current);
        const src = sourceRef.current;
        if (src && simTRef.current >= src.frames[src.frames.length - 1].t) {
          playingRef.current = false;
          setPlaying(false);
        }
      }
    }, 400);
    return () => clearInterval(iv);
  }, [active]);

  // Hit-test klik peta → kartu inspect.
  useEffect(() => {
    if (!active) return;
    streamRef.current.onMapClick = (pick) => {
      setSelected(pick);
      streamRef.current.selected = pick;
    };
    return () => {
      streamRef.current.onMapClick = null;
      streamRef.current.selected = null;
    };
  }, [active, streamRef]);

  const seek = useCallback(
    (t: number) => {
      playingRef.current = false;
      setPlaying(false);
      const src = sourceRef.current;
      if (!src) return;
      engagedRef.current = true; // scrub mengambil alih peta
      const tEnd = src.frames[src.frames.length - 1].t;
      const clamped = Math.min(tEnd, Math.max(src.t0, t));
      simTRef.current = clamped;
      anchor.current = { simT: clamped, wall: performance.now() };
      setSimT(clamped);
    },
    [],
  );

  const togglePlay = useCallback(() => {
    const src = sourceRef.current;
    if (!src) return;
    engagedRef.current = true;
    if (playingRef.current) {
      playingRef.current = false;
      setPlaying(false);
      return;
    }
    const tEnd = src.frames[src.frames.length - 1].t;
    if (simTRef.current >= tEnd) seek(src.t0); // putar ulang dari awal
    anchor.current = { simT: simTRef.current, wall: performance.now() };
    playingRef.current = true;
    setPlaying(true);
  }, [seek]);

  const exitToLive = useCallback(() => {
    playingRef.current = false;
    setPlaying(false);
    engagedRef.current = false; // peta kembali live
    setSelected(null);
    streamRef.current.selected = null;
  }, [streamRef]);

  // ---- derivasi kartu inspect ----
  const cur: Snapshot | null = source
    ? source.frames[frameIndexAt(source.frames, simT)] ?? null
    : null;
  const selectedRider =
    selected?.kind === 'rider' && cur
      ? cur.r.find((r) => r.i === selected.id) ?? null
      : null;
  const selectedOrder =
    selected && cur
      ? cur.o.find((o) => o.i === selected.id) ?? null
      : null;
  const linkedRiderForOrder =
    selectedOrder && cur ? cur.l.find((l) => l.o === selectedOrder.i) ?? null : null;
  const riderDecision =
    selectedRider && source?.kind === 'session'
      ? latestDecision(source.decisions, selectedRider.i, simT)
      : null;
  const orderAgeMs =
    selectedOrder && firstSeen.current.has(selectedOrder.i) ? simT - firstSeen.current.get(selectedOrder.i)! : null;

  const isFixture = source?.kind === 'fixture';

  return (
    <>
      {/* tombol dock */}
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        title="Scrub the 15-min recording buffer, ×1–×16, click dots to inspect"
        aria-label="Open replay and inspect panel"
        className={`pointer-events-auto rounded-pill border px-3 py-1.5 font-mono text-[11px] tracking-[0.08em] backdrop-blur transition-colors duration-fast ${
          open
            ? 'border-accent-cyan/60 bg-accent-cyan/10 text-accent-cyan'
            : 'border-line-subtle bg-surface-raised/90 text-ink-secondary hover:text-ink-primary'
        }`}
      >
        ⟲ REPLAY
      </button>

      {/* panel */}
      {open && (
        <div
          aria-label="Replay dan Inspect"
          className="pointer-events-auto fixed bottom-[64px] left-1/2 z-30 w-[min(760px,calc(100vw-2rem))] -translate-x-1/2 rounded-panel border border-line-subtle bg-surface-raised/95 p-4 backdrop-blur"
        >
          <div className="mb-2 flex items-center justify-between">
            <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
              Replay &amp; Inspect {source ? (source.kind === 'fixture' ? '· FIXTURE (OFFLINE)' : '· LIVE SESSION') : ''}
            </span>
            <button
              type="button"
              onClick={() => setOpen(false)}
              aria-label="Close replay panel"
              className="font-mono text-[11px] text-ink-muted transition-colors duration-fast hover:text-ink-primary"
            >
              ✕
            </button>
          </div>

          {status === 'loading' && (
            <p className="py-6 text-center font-mono text-[11px] text-ink-secondary" role="status">
              LOADING REPLAY BUFFER…
            </p>
          )}
          {status === 'error' && (
            <div className="py-4 text-center" role="alert">
              <p className="font-mono text-[11px] text-status-coral">{errorMsg.toUpperCase()}</p>
              <button
                type="button"
                onClick={() => void load()}
                className="mt-2 rounded-input border border-line-subtle px-2 py-1 font-mono text-[10px] text-ink-secondary transition-colors duration-fast hover:text-ink-primary"
              >
                ↻ RETRY
              </button>
            </div>
          )}

          {status === 'ready' && source && (
            <>
              {/* timeline + marker incident */}
              <div className="relative">
                <input
                  type="range"
                  min={source.t0}
                  max={Math.max(source.t0, span + source.t0)}
                  step={100}
                  value={simT}
                  aria-label="Replay timeline position"
                  aria-valuetext={`T+${fmtT(simT - source.t0)}`}
                  onChange={(e) => seek(Number(e.target.value))}
                  className="w-full cursor-pointer"
                  style={{ accentColor: 'var(--accent-cyan)' }}
                />
                {source.kind === 'session' && incidents.length > 0 && span > 0 && (
                  <div aria-hidden className="pointer-events-none relative h-2 w-full">
                    {incidents.map((inc) => {
                      const t = source.t0 + (inc.t_start - source.info.wall_first_ms);
                      if (t < source.t0 || t > source.t0 + span) return null;
                      const pct = ((t - source.t0) / span) * 100;
                      return (
                        <span
                          key={inc.id}
                          title={`${inc.kind} ${inc.target}`}
                          className={`absolute top-0 h-2 w-[3px] rounded-pill ${
                            inc.kind === 'chaos-kill' ? 'bg-status-coral' : 'bg-status-amber'
                          }`}
                          style={{ left: `${pct}%` }}
                        />
                      );
                    })}
                  </div>
                )}
              </div>

              {/* kontrol */}
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {!reduced && (
                  <button
                    type="button"
                    onClick={togglePlay}
                    disabled={span <= 0}
                    className="rounded-input border border-accent-cyan/60 bg-accent-cyan/10 px-3 py-1 font-mono text-[10px] tracking-[0.08em] text-accent-cyan transition-colors duration-fast hover:bg-accent-cyan/20 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    {playing ? '❚❚ PAUSE' : '▶ PLAY'}
                  </button>
                )}
                {!reduced &&
                  SPEEDS.map((s) => (
                    <button
                      key={s}
                      type="button"
                      aria-pressed={speed === s}
                      onClick={() => {
                        setSpeed(s);
                        speedRef.current = s;
                        anchor.current = { simT: simTRef.current, wall: performance.now() };
                      }}
                      className={`rounded-input border px-2 py-1 font-mono text-[10px] tabular-nums transition-colors duration-fast ${
                        speed === s
                          ? 'border-accent-cyan/60 bg-accent-cyan/10 text-accent-cyan'
                          : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
                      }`}
                    >
                      ×{s}
                    </button>
                  ))}
                <span className="ml-auto font-mono text-[11px] tabular-nums text-ink-primary">
                  T+{fmtT(simT - source.t0)} / {fmtT(span)}
                </span>
                {source.kind !== 'fixture' && (
                  <button
                    type="button"
                    onClick={exitToLive}
                    className="rounded-input border border-line-subtle px-2 py-1 font-mono text-[10px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:text-ink-primary"
                  >
                    ● LIVE
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => void load()}
                  className="rounded-input border border-line-subtle px-2 py-1 font-mono text-[10px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:text-ink-primary"
                >
                  ↻ REFRESH
                </button>
              </div>

              <p className="mt-1.5 font-mono text-[9px] leading-relaxed text-ink-muted">
                {source.kind === 'fixture'
                  ? 'BACKEND OFFLINE — PHASE 1 FIXTURE REPLAY (45 S LOOP @ 5 HZ). SCRUB STILL WORKS.'
                  : `SESSION ${source.info.meta.id.toUpperCase()} · ${source.info.frames} FRAMES · ${source.info.meta.hz} HZ · BUFFER ${Math.round(source.info.seconds_retained / 60)} MIN ${(source.info.seconds_retained % 60)} S · SCRUB ACCURACY ≤ ${(1000 / source.info.meta.hz / 1000).toFixed(1)} S · SEED ${source.info.meta.seed} · ${source.info.meta.strategy.toUpperCase()}`}
                {' · '}
                CLICK A RIDER/ORDER ON THE MAP TO INSPECT
              </p>

              {/* kartu inspect */}
              {!selected && (
                <p className="mt-2 font-mono text-[10px] text-ink-muted">
                  Nothing selected yet — click a rider or order dot on the map.
                </p>
              )}
              {selectedRider && (
                <div className="mt-2 rounded-card border border-line-subtle bg-surface-overlay/60 p-3" role="region" aria-label="Detail rider">
                  <div className="flex items-baseline justify-between">
                    <span className="font-mono text-[12px] font-semibold text-ink-primary">
                      RIDER r{selectedRider.i}
                    </span>
                    <span className={`font-mono text-[10px] tracking-[0.08em] ${RIDER_CLS[selectedRider.s] ?? 'text-ink-secondary'}`}>
                      {RIDER_LABEL[selectedRider.s] ?? 'UNKNOWN'}
                    </span>
                  </div>
                  {(() => {
                    const link = cur?.l.find((l) => l.r === selectedRider.i);
                    const ord = link && cur ? cur.o.find((o) => o.i === link.o) ?? null : null;
                    if (!ord) {
                      return (
                        <p className="mt-1 font-mono text-[10px] text-ink-secondary">
                          Not carrying an order — idle wandering.
                        </p>
                      );
                    }
                    return (
                      <div className="mt-1 grid gap-x-4 gap-y-0.5 font-mono text-[10px] tabular-nums text-ink-secondary sm:grid-cols-2">
                        <span>
                          ORDER <span className="text-ink-primary">{ord.i}</span> ·{' '}
                          <span className={ORDER_CLS[ord.s] ?? 'text-ink-secondary'}>
                            {ORDER_LABEL[ord.s] ?? '—'}
                          </span>
                        </span>
                        <span>
                          AGE <span className="text-ink-primary">{fmtT(simT - (firstSeen.current.get(ord.i) ?? simT))}</span>
                        </span>
                        <span>
                          PICKUP {ord.pa.toFixed(4)}, {ord.po.toFixed(4)}
                        </span>
                        <span>
                          DROPOFF {ord.da.toFixed(4)}, {ord['do'].toFixed(4)}
                        </span>
                      </div>
                    );
                  })()}
                  <div className="mt-2 border-t border-line-subtle pt-2">
                    <p className="font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary">
                      Dispatch decision reason
                    </p>
                    {riderDecision ? (
                      <>
                        <p className="mt-1 font-mono text-[10px] leading-relaxed text-ink-primary">
                          “{riderDecision.reason}”
                        </p>
                        <p className="mt-0.5 font-mono text-[9px] tabular-nums text-ink-muted">
                          {riderDecision.strategy.toUpperCase()} · ORDER {riderDecision.order} ·{' '}
                          {Math.round(riderDecision.dist_m)} M · T+{fmtT(riderDecision.t - source.t0)}
                        </p>
                      </>
                    ) : (
                      <p className="mt-1 font-mono text-[10px] text-ink-muted">
                        {source.kind === 'fixture'
                          ? '— (fixture carries no decision ring)'
                          : 'No decision recorded for this rider at this point in time.'}
                      </p>
                    )}
                  </div>
                </div>
              )}
              {selectedOrder && (
                <div className="mt-2 rounded-card border border-line-subtle bg-surface-overlay/60 p-3" role="region" aria-label="Detail order">
                  <div className="flex items-baseline justify-between">
                    <span className="font-mono text-[12px] font-semibold text-ink-primary">
                      ORDER {selectedOrder.i}
                    </span>
                    <span className={`font-mono text-[10px] tracking-[0.08em] ${ORDER_CLS[selectedOrder.s] ?? 'text-ink-secondary'}`}>
                      {ORDER_LABEL[selectedOrder.s] ?? '—'}
                    </span>
                  </div>
                  <div className="mt-1 grid gap-x-4 gap-y-0.5 font-mono text-[10px] tabular-nums text-ink-secondary sm:grid-cols-2">
                    <span>
                      AGE{' '}
                      <span className="text-ink-primary">
                        {orderAgeMs !== null ? fmtT(orderAgeMs) : '—'}
                      </span>{' '}
                      (since first seen in feed)
                    </span>
                    <span>
                      RIDER{' '}
                      <span className="text-ink-primary">
                        {linkedRiderForOrder ? `r${linkedRiderForOrder.r}` : '—'}
                      </span>
                    </span>
                    <span>
                      PICKUP {selectedOrder.pa.toFixed(4)}, {selectedOrder.po.toFixed(4)}
                    </span>
                    <span>
                      DROPOFF {selectedOrder.da.toFixed(4)}, {selectedOrder['do'].toFixed(4)}
                    </span>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </>
  );
}

/** Keputusan terakhir untuk rider pada/ sebelum simT (ring ascending). */
function latestDecision(
  decisions: { seq: number; t: number; strategy: string; order: string; rider: number; dist_m: number; reason: string }[],
  riderId: number,
  simT: number,
) {
  for (let i = decisions.length - 1; i >= 0; i--) {
    const d = decisions[i];
    if (d.t <= simT && d.rider === riderId) return d;
  }
  return null;
}

const ReplayPanel = memo(ReplayPanelImpl);
export default ReplayPanel;
