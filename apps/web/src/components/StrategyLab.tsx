'use client';

/**
 * Strategy Lab (Fase 3) — duel A/B strategi dispatch pada skenario identik.
 *
 * RUN → POST /api/lab/run → polling /api/lab/results/{id} → hasil:
 * peta replay kembar (dua canvas kecil, render on-demand), tabel delta
 * (mono tabular), histogram overlay bin bersama. Playback pakai satu rAF
 * yang hanya hidup saat PLAY ditekan; reduced-motion = frame akhir statis.
 * Semua warna via token; semua angka mono tabular (DESIGN.md §3, §7).
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import { useReducedMotion } from 'framer-motion';
import HistOverlay from './strategy/HistOverlay';
import MiniReplay from './strategy/MiniReplay';
import {
  PRESETS,
  STRATEGIES,
  fetchLabResult,
  fetchLabResults,
  getGraph,
  postLabRun,
  type GraphJSON,
  type LabResult,
  type LabSummary,
} from '@/lib/lab';

const POLL_MS = 1200;
const FRAME_MS = 60; // kecepatan playback (2s sim per frame → 30×)
const DURATIONS = [120, 300, 600] as const;

type Phase = 'idle' | 'running' | 'done' | 'error';

function StrategyLabImpl() {
  const reduced = useReducedMotion() ?? false;
  const [open, setOpen] = useState(false);
  const [phase, setPhase] = useState<Phase>('idle');
  const [errorMsg, setErrorMsg] = useState('');
  const [progress, setProgress] = useState(0);
  const [stratA, setStratA] = useState<string>('fifo');
  const [stratB, setStratB] = useState<string>('optimal');
  const [preset, setPreset] = useState<string>('steady');
  const [seconds, setSeconds] = useState<number>(300);
  const [summaries, setSummaries] = useState<LabSummary[]>([]);
  const [result, setResult] = useState<LabResult | null>(null);
  const [graph, setGraph] = useState<GraphJSON | null>(null);
  const [playIdx, setPlayIdx] = useState<number | null>(null);
  const [playing, setPlaying] = useState(false);
  const runIdRef = useRef<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadGraph = useCallback(() => {
    getGraph()
      .then(setGraph)
      .catch(() => setGraph(null)); // tanpa jalan → titik di latar gelap tetap sah
  }, []);

  const showResult = useCallback(
    (r: LabResult) => {
      setResult(r);
      setPhase(r.status === 'error' ? 'error' : 'done');
      if (r.status === 'error') setErrorMsg(r.error ?? 'duel gagal');
      setPlaying(false);
      setPlayIdx(null);
      loadGraph();
    },
    [loadGraph],
  );

  const refreshSummaries = useCallback(async () => {
    try {
      setSummaries(await fetchLabResults());
      return true;
    } catch {
      return false;
    }
  }, []);

  // Polling duel aktif — interval mati saat tidak ada duel berjalan.
  useEffect(() => {
    if (phase !== 'running' || !runIdRef.current) return;
    const id = runIdRef.current;
    const tick = async () => {
      try {
        const list = await fetchLabResults();
        setSummaries(list);
        const mine = list.find((s) => s.id === id);
        if (!mine) return;
        if (mine.status === 'running') {
          setProgress(mine.progress_pct);
          return;
        }
        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = null;
        showResult(await fetchLabResult(id));
      } catch (err) {
        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = null;
        setPhase('error');
        setErrorMsg(err instanceof Error ? err.message : 'lab tidak terjangkau');
      }
    };
    void tick();
    pollRef.current = setInterval(tick, POLL_MS);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
      pollRef.current = null;
    };
  }, [phase, showResult]);

  const run = useCallback(async () => {
    setPhase('running');
    setErrorMsg('');
    setProgress(0);
    setResult(null);
    const out = await postLabRun({ strategy_a: stratA, strategy_b: stratB, preset, seconds });
    if (out.error || !out.id) {
      setPhase('error');
      setErrorMsg(out.error ?? 'lab tidak terjangkau');
      return;
    }
    runIdRef.current = out.id;
  }, [preset, seconds, stratA, stratB]);

  const loadExisting = useCallback(
    async (id: string) => {
      if (!id) return;
      try {
        showResult(await fetchLabResult(id));
      } catch (err) {
        setPhase('error');
        setErrorMsg(err instanceof Error ? err.message : 'gagal memuat hasil');
      }
    },
    [showResult],
  );

  // Buka panel → muat riwayat duel (tanpa auto-poll).
  useEffect(() => {
    if (open) void refreshSummaries();
  }, [open, refreshSummaries]);

  // Playback: satu rAF, hanya saat PLAY — berhenti sendiri di frame terakhir.
  const frames = result?.a?.frames ?? [];
  const framesB = result?.b?.frames ?? [];
  const lastIdx = Math.max(0, frames.length - 1);
  useEffect(() => {
    if (!playing || reduced || frames.length === 0) return;
    let raf = 0;
    let last = performance.now();
    let idx = playIdx ?? 0;
    const step = (now: number) => {
      if (now - last >= FRAME_MS) {
        last = now;
        idx += 1;
        setPlayIdx(idx);
        if (idx >= frames.length - 1) {
          setPlaying(false);
          return;
        }
      }
      raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
    // playIdx sengaja tidak di-deps — loop membaca via closure idx
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [playing, reduced, frames.length]);

  const curIdx = reduced || playIdx === null ? lastIdx : playIdx;
  const frameA = frames.length > 0 ? frames[Math.min(curIdx, lastIdx)] ?? null : null;
  const frameB = framesB.length > 0 ? framesB[Math.min(curIdx, framesB.length - 1)] ?? null : null;
  const simSec = frameA ? frameA.t / 1000 : 0;
  const totalSec = frames.length > 0 ? frames[lastIdx].t / 1000 : result?.seconds ?? 0;

  const exportJson = useCallback(() => {
    if (!result) return;
    const blob = new Blob([JSON.stringify(result, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `phase3-duel-${result.id}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [result]);

  const running = phase === 'running';

  return (
    <aside
      aria-label="Strategy Lab"
      className="pointer-events-auto absolute bottom-4 left-4 z-10 w-[352px] max-w-[calc(100vw-2rem)] rounded-panel border border-line-subtle bg-surface-raised/90 backdrop-blur"
    >
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between px-4 py-2.5"
      >
        <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
          Strategy Lab
        </span>
        <span className="flex items-center gap-2">
          {phase === 'error' && <span className="font-mono text-[10px] text-status-coral">ERR</span>}
          {running && <span className="font-mono text-[10px] text-status-amber">{progress}%</span>}
          <span aria-hidden className="font-mono text-[10px] text-ink-muted">
            {open ? '▾' : '▸'}
          </span>
        </span>
      </button>

      {open && (
        <div className="max-h-[78vh] overflow-y-auto px-4 pb-4">
          {/* form duel */}
          <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-1.5">
            <select
              aria-label="Strategi A"
              value={stratA}
              onChange={(e) => setStratA(e.target.value)}
              disabled={running}
              className="rounded-input border border-line-subtle bg-surface-overlay px-2 py-1.5 font-mono text-[11px] text-ink-primary disabled:opacity-40"
            >
              {STRATEGIES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
            <span className="font-mono text-[10px] text-ink-muted">VS</span>
            <select
              aria-label="Strategi B"
              value={stratB}
              onChange={(e) => setStratB(e.target.value)}
              disabled={running}
              className="rounded-input border border-line-subtle bg-surface-overlay px-2 py-1.5 font-mono text-[11px] text-ink-primary disabled:opacity-40"
            >
              {STRATEGIES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
          <div className="mt-1.5 grid grid-cols-[1fr_1fr_auto] gap-1.5">
            <select
              aria-label="Preset skenario"
              value={preset}
              onChange={(e) => setPreset(e.target.value)}
              disabled={running}
              className="rounded-input border border-line-subtle bg-surface-overlay px-2 py-1.5 font-mono text-[11px] text-ink-primary disabled:opacity-40"
            >
              {PRESETS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
            <select
              aria-label="Durasi duel"
              value={seconds}
              onChange={(e) => setSeconds(Number(e.target.value))}
              disabled={running}
              className="rounded-input border border-line-subtle bg-surface-overlay px-2 py-1.5 font-mono text-[11px] text-ink-primary disabled:opacity-40"
            >
              {DURATIONS.map((d) => (
                <option key={d} value={d}>
                  {d}s
                </option>
              ))}
            </select>
            <button
              type="button"
              onClick={() => void run()}
              disabled={running}
              className="rounded-input border border-accent-cyan/60 bg-accent-cyan/10 px-3 py-1.5 font-mono text-[11px] tracking-[0.08em] text-accent-cyan transition-colors duration-fast hover:bg-accent-cyan/20 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {running ? 'RUNNING…' : 'RUN'}
            </button>
          </div>

          {running && (
            <div className="mt-2" role="status">
              <div className="h-1 w-full overflow-hidden rounded-pill bg-surface-overlay">
                <div
                  className="h-full bg-accent-cyan transition-[width] duration-base"
                  style={{ width: `${Math.max(3, progress)}%` }}
                />
              </div>
              <p className="mt-1 font-mono text-[10px] text-ink-secondary">
                DUEL {stratA.toUpperCase()} VS {stratB.toUpperCase()} · VIRTUAL {seconds}S · {progress}%
              </p>
            </div>
          )}

          {phase === 'error' && (
            <p className="mt-2 font-mono text-[10px] leading-relaxed text-status-coral" role="alert">
              {errorMsg.toUpperCase()}
            </p>
          )}

          {/* hasil */}
          {result && phase === 'done' && result.a && result.b && (
            <>
              <p className="mt-3 font-mono text-[9px] leading-relaxed text-ink-muted">
                {result.strategy_a.toUpperCase()} VS {result.strategy_b.toUpperCase()} · PRESET{' '}
                {result.preset.toUpperCase()} · SEED {result.seed} · ARMADA {result.riders} ·{' '}
                {result.rate_per_min.toFixed(0)}/MENIT · WALL {((result.duration_wall_ms ?? 0) / 1000).toFixed(1)}S
              </p>

              <table className="mt-2 w-full border-collapse font-mono text-[11px] tabular-nums">
                <thead>
                  <tr className="text-left text-[9px] uppercase tracking-[0.08em] text-ink-secondary">
                    <th scope="col" className="py-1 pr-2 font-medium">METRIC</th>
                    <th scope="col" className="py-1 pr-2 text-right font-medium">A</th>
                    <th scope="col" className="py-1 pr-2 text-right font-medium">B</th>
                    <th scope="col" className="py-1 text-right font-medium">Δ B−A</th>
                  </tr>
                </thead>
                <tbody>
                  {deltaRows(result).map((row) => (
                    <tr key={row.label} className="border-t border-line-subtle">
                      <th scope="row" className="py-1 pr-2 text-left font-normal text-ink-secondary">
                        {row.label}
                      </th>
                      <td className="py-1 pr-2 text-right text-ink-primary">{row.a}</td>
                      <td className="py-1 pr-2 text-right text-ink-primary">{row.b}</td>
                      <td className={`py-1 text-right ${row.deltaCls}`}>{row.delta}</td>
                    </tr>
                  ))}
                </tbody>
              </table>

              <p className="mt-2 font-mono text-[9px] leading-relaxed text-ink-muted">
                COST/ORDER = KM ON-TASK PER ORDER TERKIRIM (1 UNIT = 1 KM) · UTIL = % WAKTU ARMADA BEBAN TUGAS
              </p>

              <div className="mt-2 border-t border-line-subtle pt-2">
                <p className="font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary">
                  Delivery time A vs B (bin bersama)
                </p>
                <HistOverlay hist={result.histogram ?? null} />
              </div>

              <div className="mt-1 grid grid-cols-2 gap-2">
                <MiniReplay frame={frameA} graph={graph} side="a" />
                <MiniReplay frame={frameB} graph={graph} side="b" />
              </div>

              {/* kontrol playback */}
              <div className="mt-2 flex items-center gap-2">
                {!reduced && (
                  <button
                    type="button"
                    onClick={() => {
                      if (playing) {
                        setPlaying(false);
                        return;
                      }
                      if (curIdx >= lastIdx) setPlayIdx(0);
                      setPlaying(true);
                    }}
                    disabled={frames.length < 2}
                    className="rounded-input border border-line-subtle px-2 py-1 font-mono text-[10px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:text-ink-primary disabled:opacity-40"
                  >
                    {playing ? '❚❚ PAUSE' : '▶ PLAY'}
                  </button>
                )}
                <input
                  type="range"
                  min={0}
                  max={Math.max(0, lastIdx)}
                  value={curIdx}
                  disabled={frames.length < 2}
                  aria-label="Posisi replay duel"
                  onChange={(e) => {
                    setPlaying(false);
                    setPlayIdx(Number(e.target.value));
                  }}
                  className="min-w-0 flex-1 cursor-pointer disabled:cursor-not-allowed disabled:opacity-40"
                  style={{ accentColor: 'var(--accent-cyan)' }}
                />
                <span className="whitespace-nowrap font-mono text-[10px] tabular-nums text-ink-secondary">
                  T+{simSec.toFixed(0)}/{totalSec.toFixed(0)}S
                </span>
              </div>
              {reduced && (
                <p className="mt-1 font-mono text-[9px] text-ink-muted">
                  REDUCED MOTION — FRAME AKHIR STATIS
                </p>
              )}

              <div className="mt-2 flex items-center gap-2">
                <button
                  type="button"
                  onClick={exportJson}
                  className="rounded-input border border-line-subtle px-2 py-1 font-mono text-[10px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:text-ink-primary"
                >
                  ⭳ EXPORT JSON
                </button>
                <select
                  aria-label="Riwayat duel"
                  value={result.id}
                  onChange={(e) => void loadExisting(e.target.value)}
                  className="min-w-0 flex-1 rounded-input border border-line-subtle bg-surface-overlay px-2 py-1 font-mono text-[10px] text-ink-primary"
                >
                  {summaries
                    .filter((s) => s.status === 'done' || s.status === 'error')
                    .map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.id} · {s.strategy_a}/{s.strategy_b} · {s.preset}
                      </option>
                    ))}
                  {!summaries.some((x) => x.id === result.id) && (
                    <option value={result.id}>{result.id}</option>
                  )}
                </select>
              </div>
            </>
          )}
        </div>
      )}
    </aside>
  );
}

interface Row {
  label: string;
  a: string;
  b: string;
  delta: string;
  deltaCls: string;
}

/** Baris delta first-class — arah "lebih baik" ditentukan per metrik. */
function deltaRows(r: LabResult): Row[] {
  const a = r.a;
  const b = r.b;
  if (!a || !b) return [];
  const f1 = (v: number) => v.toFixed(1);
  const secs = (ms: number) => `${(ms / 1000).toFixed(1)}s`;
  const mk = (
    label: string,
    va: number,
    vb: number,
    fmt: (v: number) => string,
    dir: 1 | -1 | 0,
  ): Row => {
    const d = vb - va;
    const delta = `${d >= 0 ? '+' : '−'}${fmt(Math.abs(d))}`;
    let deltaCls = 'text-ink-secondary';
    if (dir !== 0 && Math.abs(d) > 1e-9) {
      const bBetter = dir === 1 ? d > 0 : d < 0;
      deltaCls = bBetter ? 'text-accent-lime' : 'text-status-coral';
    }
    return { label, a: fmt(va), b: fmt(vb), delta, deltaCls };
  };
  return [
    mk('DELIVERED', a.delivered, b.delivered, (v) => v.toFixed(0), 1),
    mk('EXPIRED', a.expired, b.expired, (v) => v.toFixed(0), -1),
    mk('P50 DELIV', a.delivery_p50_ms, b.delivery_p50_ms, secs, -1),
    mk('P95 DELIV', a.delivery_p95_ms, b.delivery_p95_ms, secs, -1),
    mk('UTIL', a.utilization_pct, b.utilization_pct, (v) => `${f1(v)}%`, 0),
    mk('COST/ORDER', a.cost_per_order_km, b.cost_per_order_km, (v) => `${v.toFixed(2)}km`, -1),
    mk('THROUGHPUT', a.throughput_per_min, b.throughput_per_min, (v) => `${f1(v)}/m`, 1),
  ];
}

const StrategyLab = memo(StrategyLabImpl);
export default StrategyLab;
