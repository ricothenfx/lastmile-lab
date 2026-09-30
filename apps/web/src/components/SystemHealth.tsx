'use client';

/**
 * System Health (Fase 4) — visualisasi pipeline order→Kafka→consumer +
 * grid status node (healthz polling via /api/kpi) + health events +
 * Chaos Console (allowlist lastmile-*) + Incident Timeline (MTTD/MTTR).
 *
 * Canvas pipeline menjalankan rAF HANYA saat panel terbuka di tab PIPELINE
 * dan feed hidup — nol rAF saat idle (DoD 60fps). reduced-motion = diagram
 * statis tanpa partikel. Semua warna via token.
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react';
import { useReducedMotion } from 'framer-motion';
import {
  fetchChaosIncidents,
  fetchChaosState,
  postChaosKill,
  type ChaosIncidents,
  type ChaosTarget,
  type GridNode,
  type KpiPayload,
} from '@/lib/kpi';
import { tokens } from '@/lib/tokens';

const CH_H = 104;
const POLL_CHAOS_MS = 1500;

type Tab = 'pipeline' | 'chaos';

// ---- model node pipeline ----

interface PipeNode {
  key: string;
  label: string;
  row: 0 | 1;
  status: () => 'up' | 'down' | 'standby';
}

function nodeStatus(kpi: KpiPayload | null, name: string): 'up' | 'down' | 'standby' {
  const n = kpi?.grid.find((g) => g.name === name);
  if (!n) return 'standby';
  return n.status;
}

// ---- canvas pipeline ----

function PipelineCanvas({ kpi, stale }: { kpi: KpiPayload | null; stale: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const reduced = useReducedMotion() ?? false;
  const kpiRef = useRef(kpi);
  const staleRef = useRef(stale);
  const staticDrawRef = useRef<(() => void) | null>(null);
  kpiRef.current = kpi;
  staleRef.current = stale;

  // rAF hanya saat panel aktif (dipasang oleh efek di bawah) — loop partikel.
  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    const parent = canvas?.parentElement;
    if (!canvas || !ctx || !parent) return;

    let raf = 0;
    let particles: { edge: number; t: number; speed: number }[] = [];
    let lastSpawn = 0;
    let running = true;

    const layout = () => {
      const w = Math.max(220, parent.clientWidth);
      return { w, h: CH_H };
    };

    const nodeXs = (w: number, row: 0 | 1, count: number, idx: number) => {
      // row0: 5 node pipeline (ORDERS..RIDER-SIM), row1: 3 node feed (RIDER-SIM..PULSE)
      const nw = 44;
      const pad = 6;
      const span = w - pad * 2 - nw;
      if (row === 0) return pad + (span / (count - 1)) * idx;
      return pad + (span / (count - 1)) * idx;
    };
    const rowY = [24, 74];

    const drawStatic = () => {
      const { w, h } = layout();
      const dpr = Math.min(2, window.devicePixelRatio || 1);
      if (canvas.width !== Math.round(w * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(h * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);
      const k = kpiRef.current;
      const dimmed = staleRef.current;

      const pipelineDown =
        ['order-ingestion', 'dispatch-consumer'].every(
          (n) => nodeStatus(k, n) !== 'up',
        ) ?? true;
      const nodes: { label: string; row: 0 | 1; idx: number; st: 'up' | 'down' | 'standby' }[] = [
        { label: 'ORDERS', row: 0, idx: 0, st: (k && !dimmed && (k.orders_per_min ?? 0) > 0) || nodeStatus(k, 'order-ingestion') === 'up' ? 'up' : pipelineDown ? 'standby' : 'down' },
        { label: 'INGEST', row: 0, idx: 1, st: pipelineDown ? 'standby' : nodeStatus(k, 'order-ingestion') },
        { label: 'KAFKA', row: 0, idx: 2, st: pipelineDown ? 'standby' : 'up' },
        { label: 'CONSUME', row: 0, idx: 3, st: pipelineDown ? 'standby' : nodeStatus(k, 'dispatch-consumer') },
        { label: 'RIDER-SIM', row: 0, idx: 4, st: nodeStatus(k, 'rider-sim') },
        { label: 'RIDER-SIM', row: 1, idx: 0, st: nodeStatus(k, 'rider-sim') },
        { label: 'WS-GW', row: 1, idx: 1, st: nodeStatus(k, 'ws-gateway') },
        { label: 'PULSE', row: 1, idx: 2, st: 'up' },
      ];

      // edges: [row, idxFrom, idxTo, active]
      const edges: { a: number; b: number; active: boolean; color: string }[] = [
        { a: 0, b: 1, active: nodes[0].st === 'up' && nodes[1].st === 'up', color: tokens.color.accentCyan },
        { a: 1, b: 2, active: nodes[1].st === 'up' && nodes[2].st === 'up', color: tokens.color.accentCyan },
        { a: 2, b: 3, active: nodes[2].st === 'up' && nodes[3].st === 'up', color: tokens.color.accentCyan },
        { a: 3, b: 4, active: nodes[3].st === 'up' && nodes[4].st === 'up', color: tokens.color.accentCyan },
        { a: 5, b: 6, active: nodes[5].st === 'up' && nodes[6].st === 'up', color: tokens.color.statusViolet },
        { a: 6, b: 7, active: nodes[6].st === 'up', color: tokens.color.statusViolet },
      ];
      const cx = (row: 0 | 1, idx: number, count: number) => nodeXs(w, row, count, idx) + 22;
      const cy = (row: 0 | 1) => rowY[row] + 11;
      for (const e of edges) {
        const a = nodes[e.a];
        const b = nodes[e.b];
        const caCount = a.row === 0 ? 5 : 3;
        const cbCount = b.row === 0 ? 5 : 3;
        const x1 = cx(a.row, a.idx, caCount);
        const y1 = cy(a.row);
        const x2 = cx(b.row, b.idx, cbCount);
        const y2 = cy(b.row);
        ctx.beginPath();
        ctx.moveTo(x1, y1);
        ctx.lineTo(x2, y2);
        ctx.strokeStyle = e.active ? `${e.color}55` : tokens.color.borderSubtle;
        ctx.lineWidth = 1.5;
        ctx.stroke();
      }
      for (const n of nodes) {
        const count = n.row === 0 ? 5 : 3;
        const x = nodeXs(w, n.row, count, n.idx);
        const y = rowY[n.row];
        const border =
          n.st === 'up'
            ? n.row === 0 && n.idx > 0 && n.idx < 4
              ? tokens.color.accentCyan
              : tokens.color.accentLime
            : n.st === 'down'
              ? tokens.color.statusCoral
              : tokens.color.borderSubtle;
        ctx.beginPath();
        ctx.roundRect(x, y, 44, 22, 4);
        ctx.fillStyle = n.st === 'standby' ? `${tokens.color.bgBase}cc` : `${tokens.color.bgOverlay}cc`;
        ctx.fill();
        ctx.strokeStyle = border;
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.fillStyle = n.st === 'standby' ? tokens.color.textMuted : tokens.color.textPrimary;
        ctx.font = '7px ui-monospace, monospace';
        ctx.textAlign = 'center';
        ctx.fillText(n.label, x + 22, y + 14);
      }
      return { nodes, edges, cx, cy };
    };

    const draw = (now: number) => {
      const { w } = layout();
      const stat = drawStatic();
      const k = kpiRef.current;
      const opm = k && !staleRef.current ? k.orders_per_min ?? 0 : 0;
      const frameUp = stat.nodes[5].st === 'up' && stat.nodes[6].st === 'up';

      // spawn partikel — laju dari throughput nyata (min visual 1/s saat up)
      if (now - lastSpawn > 220) {
        lastSpawn = now;
        stat.edges.forEach((e, i) => {
          if (!e.active) return;
          const ratePerSec = i < 4 ? Math.max(0.5, Math.min(6, opm / 20)) : frameUp ? 2.5 : 0;
          if (Math.random() < ratePerSec * 0.22) {
            particles.push({ edge: i, t: 0, speed: 0.35 + Math.random() * 0.2 });
          }
        });
      }
      for (const p of particles) {
        const e = stat.edges[p.edge];
        if (!e) continue;
        const a = stat.nodes[e.a];
        const b = stat.nodes[e.b];
        const x1 = stat.cx(a.row, a.idx, a.row === 0 ? 5 : 3);
        const y1 = stat.cy(a.row);
        const x2 = stat.cx(b.row, b.idx, b.row === 0 ? 5 : 3);
        const y2 = stat.cy(b.row);
        ctx.beginPath();
        ctx.arc(x1 + (x2 - x1) * p.t, y1 + (y2 - y1) * p.t, 2, 0, Math.PI * 2);
        ctx.fillStyle = e.color;
        ctx.fill();
      }
      particles = particles.filter((p) => (p.t += p.speed * 0.045) < 1);

      if (running) raf = requestAnimationFrame(draw);
    };

    // partikel hanya saat: reduced-motion mati. Loop berhenti saat unmount.
    staticDrawRef.current = drawStatic;
    if (reduced) {
      drawStatic();
    } else {
      raf = requestAnimationFrame(draw);
    }
    return () => {
      running = false;
      cancelAnimationFrame(raf);
    };
    // kpi/stale dibaca via ref (dipbarui tiap render) — loop tak perlu reset.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reduced]);

  // reduced-motion: redraw statis saat data berubah — tanpa rAF.
  useEffect(() => {
    if (reduced) staticDrawRef.current?.();
  }, [kpi, stale, reduced]);

  return (
    <div
      className="relative w-full rounded-card border border-line-subtle bg-surface-base"
      style={{ height: CH_H }}
      role="img"
      aria-label="Visualisasi pipeline order → Kafka → consumer → rider-sim → ws-gateway → UI"
    >
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" aria-hidden />
    </div>
  );
}

// ---- grid status node ----

function NodeGrid({ grid }: { grid: GridNode[] }) {
  const color = (s: GridNode['status']) =>
    s === 'up' ? tokens.color.accentLime : s === 'down' ? tokens.color.statusCoral : tokens.color.textMuted;
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {grid.map((n) => (
        <span
          key={n.name}
          title={`${n.name} · ${n.status}${n.latency_ms ? ` · ${n.latency_ms}ms` : ''}`}
          className="flex items-center gap-1.5 rounded-pill border border-line-subtle px-2 py-0.5"
        >
          <span aria-hidden className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color(n.status) }} />
          <span className={`font-mono text-[9px] tracking-[0.04em] ${n.status === 'standby' ? 'text-ink-muted' : 'text-ink-secondary'}`}>
            {n.name.toUpperCase()}
            {n.status === 'up' && n.latency_ms > 0 && (
              <span className="ml-1 tabular-nums text-ink-muted">{n.latency_ms}ms</span>
            )}
          </span>
        </span>
      ))}
    </div>
  );
}

// ---- chaos console ----

function KillButton({
  target,
  disabled,
  onFire,
}: {
  target: ChaosTarget;
  disabled: boolean;
  onFire: (t: string) => Promise<{ ok: boolean; error?: string }>;
}) {
  const [armed, setArmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
  }, []);
  const healthy = target.status === 'healthy';
  const off = disabled || !healthy || busy;
  const click = async () => {
    if (off) return;
    if (!armed) {
      setArmed(true);
      timer.current = setTimeout(() => setArmed(false), 3000);
      return;
    }
    if (timer.current) clearTimeout(timer.current);
    setArmed(false);
    setBusy(true);
    await onFire(target.name);
    setBusy(false);
  };
  const dot =
    target.status === 'healthy'
      ? tokens.color.accentLime
      : target.status === 'down'
        ? tokens.color.statusCoral
        : tokens.color.textMuted;
  return (
    <button
      type="button"
      onClick={click}
      disabled={off}
      aria-label={`Kill node ${target.name}${armed ? ' — klik lagi untuk konfirmasi' : ''}`}
      className={`flex items-center justify-between gap-1 rounded-input border px-2 py-1.5 font-mono text-[9px] tracking-[0.04em] transition-colors duration-fast disabled:cursor-not-allowed disabled:opacity-40 ${
        armed
          ? 'border-status-coral bg-status-coral/20 text-status-coral'
          : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
      }`}
    >
      <span className="flex items-center gap-1 truncate">
        <span aria-hidden className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: dot }} />
        {target.name.toUpperCase()}
      </span>
      <span aria-hidden>{armed ? 'CONFIRM?' : 'KILL'}</span>
    </button>
  );
}

function fmtClock(ms: number): string {
  const d = new Date(ms);
  return d.toLocaleTimeString('en-GB', { hour12: false });
}

function fmtDur(ms: number): string {
  if (ms >= 60_000) return `${(ms / 60_000).toFixed(1)}m`;
  return `${(ms / 1000).toFixed(1)}s`;
}

// ---- panel utama ----

interface Props {
  kpiState: { kpi: KpiPayload | null; stale: boolean };
}

function SystemHealthImpl({ kpiState }: Props) {
  const { kpi, stale } = kpiState;
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<Tab>('pipeline');
  const [chaosTargets, setChaosTargets] = useState<ChaosTarget[] | null>(null);
  const [incidents, setIncidents] = useState<ChaosIncidents | null>(null);
  const [killMsg, setKillMsg] = useState('');

  const chaosUp = kpi?.grid.find((n) => n.name === 'chaos')?.status === 'up';

  // Polling chaos hanya saat panel terbuka (interval mati saat tertutup).
  useEffect(() => {
    if (!open) return;
    let alive = true;
    const tick = async () => {
      const [t, i] = await Promise.all([fetchChaosState(), fetchChaosIncidents()]);
      if (!alive) return;
      setChaosTargets(t);
      setIncidents(i);
    };
    void tick();
    const iv = setInterval(tick, POLL_CHAOS_MS);
    return () => {
      alive = false;
      clearInterval(iv);
    };
  }, [open]);

  const fire = useCallback(async (t: string) => {
    const res = await postChaosKill(t);
    setKillMsg(res.ok ? `KILL ${t.toUpperCase()} TERKIRIM — self-heal dihitung` : `GAGAL: ${res.error ?? '?'}`);
    return res;
  }, []);

  // Health events dari incident (turun → naik).
  const events: { ts: number; text: string; bad: boolean }[] = [];
  for (const inc of incidents?.incidents ?? []) {
    events.push({ ts: inc.t_detect || inc.t_start, text: `${inc.target.toUpperCase()} DOWN (${inc.kind})`, bad: true });
    if (inc.t_recover > 0) {
      events.push({ ts: inc.t_recover, text: `${inc.target.toUpperCase()} RECOVERED`, bad: false });
    }
  }
  events.sort((a, b) => b.ts - a.ts);

  return (
    <aside
      aria-label="System Health"
      className="pointer-events-auto absolute bottom-4 right-4 z-10 w-[352px] max-w-[calc(100vw-2rem)] rounded-panel border border-line-subtle bg-surface-raised/90 backdrop-blur"
    >
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between px-4 py-2.5"
      >
        <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
          System Health
        </span>
        <span className="flex items-center gap-2">
          {(kpi?.incidents_open ?? 0) > 0 && (
            <span className="font-mono text-[9px] text-status-coral">
              {kpi?.incidents_open} OPEN
            </span>
          )}
          <span aria-hidden className="font-mono text-[10px] text-ink-muted">
            {open ? '▾' : '▸'}
          </span>
        </span>
      </button>

      {open && (
        <div className="px-4 pb-4">
          <div className="grid grid-cols-2 gap-1.5" role="tablist" aria-label="Tab System Health">
            {(['pipeline', 'chaos'] as Tab[]).map((t) => (
              <button
                key={t}
                type="button"
                role="tab"
                aria-selected={tab === t}
                onClick={() => setTab(t)}
                className={`rounded-input border px-2 py-1 font-mono text-[10px] tracking-[0.08em] transition-colors duration-fast ${
                  tab === t
                    ? 'border-accent-cyan/60 bg-accent-cyan/10 text-accent-cyan'
                    : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
                }`}
              >
                {t === 'pipeline' ? 'PIPELINE' : 'CHAOS'}
              </button>
            ))}
          </div>

          {tab === 'pipeline' && (
            <>
              <div className="mt-2">
                <PipelineCanvas kpi={kpi} stale={stale} />
              </div>
              <NodeGrid grid={kpi?.grid ?? []} />
              <div className="mt-2 border-t border-line-subtle pt-2">
                <p className="font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary">
                  Health events
                </p>
                {events.length === 0 ? (
                  <p className="mt-1 font-mono text-[9px] text-ink-muted">
                    {chaosUp ? 'TIDAK ADA INCIDENT' : 'CHAOS STANDBY — AKTIFKAN PROFILE CHAOS'}
                  </p>
                ) : (
                  <ul className="mt-1 max-h-[72px] space-y-0.5 overflow-y-auto">
                    {events.slice(0, 8).map((e, i) => (
                      <li key={`${e.ts}-${i}`} className="flex items-baseline gap-2 font-mono text-[9px] tabular-nums">
                        <span className="shrink-0 text-ink-muted">{fmtClock(e.ts)}</span>
                        <span className={e.bad ? 'text-status-coral' : 'text-accent-lime'}>{e.text}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}

          {tab === 'chaos' && (
            <>
              <p className="mt-2 font-mono text-[9px] leading-relaxed text-ink-muted">
                KILL = SIGKILL CONTAINER (ALLOWLIST lastmile-* STATELESS) · SELF-HEAL VIA
                RESTART POLICY UNLESS-STOPPED · INFRA STATE TIDAK BISA DISENTUH
              </p>
              {chaosTargets ? (
                <div className="mt-2 grid grid-cols-2 gap-1.5">
                  {chaosTargets.map((t) => (
                    <KillButton key={t.name} target={t} disabled={!chaosUp} onFire={fire} />
                  ))}
                </div>
              ) : (
                <p className="mt-2 font-mono text-[9px] text-ink-muted">
                  {chaosUp ? 'MEMUAT TARGET…' : 'CHAOS STANDBY — AKTIFKAN PROFILE CHAOS'}
                </p>
              )}
              {killMsg && (
                <p className="mt-1.5 font-mono text-[9px] text-status-amber" role="status">
                  {killMsg.toUpperCase()}
                </p>
              )}
              <div className="mt-2 border-t border-line-subtle pt-2">
                <div className="flex flex-wrap gap-x-3 gap-y-0.5 font-mono text-[9px] tabular-nums text-ink-secondary">
                  <span>
                    MTTD {(incidents?.summary.mttd_avg_ms ?? 0) > 0 ? fmtDur(incidents!.summary.mttd_avg_ms) : '—'}
                  </span>
                  <span>
                    MTTR {(incidents?.summary.mttr_avg_ms ?? 0) > 0 ? fmtDur(incidents!.summary.mttr_avg_ms) : '—'}
                  </span>
                  <span>AVAIL {(incidents?.summary.availability_pct ?? 100).toFixed(2)}%</span>
                  <span className={incidents?.summary.budget_ok === false ? 'text-status-coral' : 'text-accent-lime'}>
                    BUDGET {incidents?.summary.budget_ok === false ? 'EXCEEDED' : incidents ? 'OK' : '—'}
                  </span>
                </div>
              </div>
              <p className="mt-2 font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary">
                Incident timeline
              </p>
              {(incidents?.incidents.length ?? 0) === 0 ? (
                <p className="mt-1 font-mono text-[9px] text-ink-muted">
                  {chaosUp ? 'BELUM ADA INCIDENT' : 'CHAOS STANDBY'}
                </p>
              ) : (
                <ul className="mt-1 max-h-[168px] space-y-1 overflow-y-auto">
                  {(incidents?.incidents ?? []).slice(0, 20).map((inc) => {
                    const isOpen = inc.t_recover <= 0;
                    return (
                      <li key={inc.id} className="rounded-card border border-line-subtle px-2 py-1">
                        <div className="flex items-center justify-between gap-2 font-mono text-[9px]">
                          <span className="flex min-w-0 items-center gap-1.5">
                            <span aria-hidden className="h-1.5 w-1.5 shrink-0 rounded-full"
                              style={{ backgroundColor: isOpen ? tokens.color.statusCoral : tokens.color.accentLime }} />
                            <span className="truncate text-ink-primary">{inc.target}</span>
                            <span className={`shrink-0 ${inc.kind === 'chaos-kill' ? 'text-status-amber' : 'text-ink-muted'}`}>
                              {inc.kind === 'chaos-kill' ? 'KILL' : 'HEALTH'}
                            </span>
                          </span>
                          <span className="shrink-0 tabular-nums text-ink-muted">{fmtClock(inc.t_start)}</span>
                        </div>
                        <div className="mt-0.5 flex items-center justify-between gap-2 font-mono text-[9px] tabular-nums">
                          <span className="text-ink-secondary">
                            MTTD {inc.t_detect > 0 ? fmtDur(inc.t_detect - inc.t_start) : '…'}
                            {inc.t_recover > 0 && ` · MTTR ${fmtDur(inc.t_recover - inc.t_detect)}`}
                          </span>
                          {isOpen ? (
                            <span className="text-status-coral">OPEN</span>
                          ) : (
                            <span className="text-accent-lime">RECOVERED</span>
                          )}
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </>
          )}
        </div>
      )}
    </aside>
  );
}

const SystemHealth = memo(SystemHealthImpl);
export default SystemHealth;
