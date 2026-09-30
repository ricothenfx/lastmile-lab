'use client';

/**
 * KPI Command Deck (Fase 4) — kartu live + streaming chart + SLO gauge.
 *
 * Semua angka dari /api/kpi (metrik nyata: ring engine, counters pipeline,
 * incident chaos) — tanpa data, kartu menampilkan "—" (bukan angka karangan).
 * Chart canvas menggambar HANYA saat data baru masuk (2 Hz, tanpa rAF —
 * budget 60fps idle aman). Kartu SLO yang langgar bergetar halus; mati
 * penuh saat prefers-reduced-motion. Warna 100% via token.
 */

import { memo, useEffect, useRef, useState } from 'react';
import { motion, useReducedMotion } from 'framer-motion';
import type { ChartPoint, GridNode, KpiPayload, SloItem } from '@/lib/kpi';
import { tokens } from '@/lib/tokens';

const P95_TARGET_MS = 360_000; // SLO: p95 delivery < 6 menit
const DISPATCH_TARGET_MS = 50; // target p99 dispatch (AGENTS.md)
const CHART_H = 84;

function fmtOrDash(
  v: number | null | undefined,
  fmt: (v: number) => string,
  valid: boolean,
): string {
  if (!valid || v === null || v === undefined || Number.isNaN(v)) return '—';
  return fmt(v);
}

// ---- SLO gauge (SVG semicircle — update hanya saat state berubah) ----

function gaugeState(slo: SloItem[]): { frac: number; color: string; label: string } {
  if (slo.length === 0) return { frac: 0, color: tokens.color.textMuted, label: '—' };
  const violated = slo.filter((s) => s.ok === false).length;
  const okCount = slo.filter((s) => s.ok === true).length;
  const unknown = slo.length - violated - okCount;
  if (violated > 0) {
    return { frac: (slo.length - violated) / slo.length, color: tokens.color.statusCoral, label: `${okCount}/${slo.length}` };
  }
  if (unknown > 0) {
    return { frac: okCount / slo.length, color: tokens.color.statusAmber, label: `${okCount}/${slo.length}` };
  }
  return { frac: 1, color: tokens.color.accentLime, label: `${okCount}/${slo.length}` };
}

function SLOGauge({ slo }: { slo: SloItem[] }) {
  const { frac, color, label } = gaugeState(slo);
  const r = 24;
  const arc = Math.PI * r;
  return (
    <div className="flex items-center gap-2" title="Status SLO gabungan">
      <svg width="64" height="38" viewBox="0 0 64 38" role="img" aria-label={`SLO ${label} terpenuhi`}>
        <path
          d={`M 6 34 A ${r} ${r} 0 0 1 58 34`}
          fill="none"
          stroke={tokens.color.borderSubtle}
          strokeWidth="6"
          strokeLinecap="round"
        />
        <path
          d={`M 6 34 A ${r} ${r} 0 0 1 58 34`}
          fill="none"
          stroke={color}
          strokeWidth="6"
          strokeLinecap="round"
          strokeDasharray={`${Math.max(0.01, frac * arc)} ${arc}`}
        />
        <text x="32" y="30" textAnchor="middle" fontSize="10" fill={tokens.color.textPrimary}
          style={{ fontFamily: 'var(--font-mono)', fontVariantNumeric: 'tabular-nums' }}>
          {label}
        </text>
      </svg>
      <span className="hidden font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary lg:inline">
        SLO
      </span>
    </div>
  );
}

// ---- kartu (getar halus saat SLO langgar — mati saat reduced motion) ----

function Card({
  label,
  value,
  sub,
  violated,
  tone,
}: {
  label: string;
  value: string;
  sub: string;
  violated?: boolean;
  tone?: 'coral';
}) {
  const reduced = useReducedMotion() ?? false;
  return (
    <motion.div
      animate={violated && !reduced ? { x: [0, -1.5, 1.5, -1, 0] } : { x: 0 }}
      transition={violated && !reduced ? { duration: 0.45, repeat: Infinity, repeatDelay: 1.4 } : { duration: 0.12 }}
      className={`rounded-card border px-2.5 py-2 ${
        violated ? 'border-status-coral/60' : 'border-line-subtle'
      } bg-surface-base/60`}
    >
      <p className="font-mono text-[9px] uppercase tracking-[0.1em] text-ink-secondary">{label}</p>
      <p
        className={`mt-0.5 font-mono text-[21px] font-semibold leading-none tabular-nums ${
          tone === 'coral' ? 'text-status-coral' : 'text-ink-primary'
        }`}
        aria-live={violated ? 'polite' : undefined}
      >
        {value}
      </p>
      <p className="mt-1 font-mono text-[8px] uppercase tracking-[0.06em] text-ink-muted">{sub}</p>
    </motion.div>
  );
}

// ---- streaming chart (gambar hanya saat data baru — tanpa rAF) ----

function StreamChart({ chart, fresh }: { chart: ChartPoint[]; fresh: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    const parent = canvas?.parentElement;
    if (!canvas || !ctx || !parent) return;
    const w = Math.max(60, parent.clientWidth);
    const h = CHART_H;
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    if (canvas.width !== Math.round(w * dpr)) {
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);

    // gridline halus 25/50/75%
    ctx.strokeStyle = tokens.color.borderSubtle;
    ctx.lineWidth = 1;
    for (const f of [0.25, 0.5, 0.75]) {
      ctx.beginPath();
      ctx.moveTo(0, h * f);
      ctx.lineTo(w, h * f);
      ctx.stroke();
    }

    const pts = chart.filter((p) => p.ordersPerMin !== null);
    if (pts.length < 2) {
      ctx.fillStyle = tokens.color.textMuted;
      ctx.font = '9px ui-monospace, monospace';
      ctx.textAlign = 'center';
      ctx.fillText(fresh ? 'COLLECTING…' : 'AWAITING TELEMETRY', w / 2, h / 2);
      return;
    }
    const max = Math.max(...pts.map((p) => p.ordersPerMin ?? 0), 5);
    const t0 = pts[0].ts;
    const t1 = pts[pts.length - 1].ts;
    const span = Math.max(1, t1 - t0);
    const x = (ts: number) => ((ts - t0) / span) * (w - 2) + 1;
    const y = (v: number) => h - 4 - (v / max) * (h - 10);

    // area + garis orders/min
    ctx.beginPath();
    ctx.moveTo(x(pts[0].ts), h);
    for (const p of pts) ctx.lineTo(x(p.ts), y(p.ordersPerMin ?? 0));
    ctx.lineTo(x(pts[pts.length - 1].ts), h);
    ctx.closePath();
    ctx.fillStyle = `${tokens.color.accentCyan}26`; // 15% alpha
    ctx.fill();
    ctx.beginPath();
    for (let i = 0; i < pts.length; i++) {
      const px = x(pts[i].ts);
      const py = y(pts[i].ordersPerMin ?? 0);
      if (i === 0) ctx.moveTo(px, py);
      else ctx.lineTo(px, py);
    }
    ctx.strokeStyle = tokens.color.accentCyan;
    ctx.lineWidth = 1.5;
    ctx.stroke();

    // label max + last
    ctx.fillStyle = tokens.color.textSecondary;
    ctx.font = '8px ui-monospace, monospace';
    ctx.textAlign = 'left';
    ctx.fillText(`${max.toFixed(0)}/m`, 4, 10);
    const last = pts[pts.length - 1].ordersPerMin ?? 0;
    ctx.textAlign = 'right';
    ctx.fillText(`${last.toFixed(1)}/m`, w - 4, 10);
  }, [chart, fresh]);

  return (
    <div className="relative w-full" style={{ height: CHART_H }}>
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" aria-hidden />
      <span className="pointer-events-none absolute bottom-1 left-1 font-mono text-[8px] uppercase tracking-[0.1em] text-ink-muted">
        ORDERS/MIN · 5 MIN @2HZ
      </span>
    </div>
  );
}

// ---- panel utama ----

interface Props {
  kpiState: { kpi: KpiPayload | null; stale: boolean; chart: ChartPoint[] };
  onOpenChange?: (open: boolean) => void;
}

function KpiDeckImpl({ kpiState, onOpenChange }: Props) {
  const { kpi, stale, chart } = kpiState;
  const [open, setOpen] = useState(true);
  const sim = kpi?.sim;
  const hasSim = !!sim && sim.reachable && !stale;

  const sloById = new Map((kpi?.slo ?? []).map((s) => [s.id, s]));
  const p95Violated = sloById.get('delivery_p95')?.ok === false;
  const dispViolated = hasSim && sim.p99_dispatch_ms >= DISPATCH_TARGET_MS;
  const anyViolated = (kpi?.slo ?? []).some((s) => s.ok === false);

  return (
    <aside
      aria-label="KPI Command Deck"
      className="pointer-events-auto absolute left-4 top-4 z-10 w-[600px] max-w-[min(600px,calc(100vw-16.5rem))] rounded-panel border border-line-subtle bg-surface-raised/90 backdrop-blur"
    >
      <div className="flex items-center justify-between px-4 py-2.5">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => {
            const next = !open;
            setOpen(next);
            onOpenChange?.(next);
          }}
          className="flex min-w-0 items-center gap-3"
        >
          <span className="whitespace-nowrap font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
            KPI Command Deck
          </span>
          <span
            className={`font-mono text-[9px] tracking-[0.08em] ${
              stale || !kpi
                ? 'text-ink-muted'
                : anyViolated
                  ? 'text-status-coral'
                  : kpi.source === 'partial'
                    ? 'text-status-amber'
                    : 'text-accent-lime'
            }`}
          >
            {stale || !kpi ? 'NO TELEMETRY' : kpi.source === 'partial' ? 'PARTIAL' : 'LIVE'}
          </span>
          <span aria-hidden className="font-mono text-[10px] text-ink-muted">
            {open ? '▾' : '▸'}
          </span>
        </button>
        <SLOGauge slo={kpi?.slo ?? []} />
      </div>

      {open && (
        <div className="px-4 pb-4">
          <div className="grid grid-cols-2 gap-2 min-[560px]:grid-cols-4">
            <Card
              label="Delivery P50"
              value={fmtOrDash(sim?.delivery_p50_ms, (v) => (v / 1000).toFixed(0) + 's', hasSim && sim.delivery_samples > 0)}
              sub="created → delivered"
            />
            <Card
              label="Delivery P95"
              value={fmtOrDash(sim?.delivery_p95_ms, (v) => (v / 1000).toFixed(0) + 's', hasSim && sim.delivery_samples > 0)}
              sub={`SLO < ${P95_TARGET_MS / 1000}s`}
              violated={p95Violated}
              tone={p95Violated ? 'coral' : undefined}
            />
            <Card
              label="Utilization"
              value={fmtOrDash(sim?.utilization_pct, (v) => v.toFixed(0) + '%', hasSim)}
              sub="armada on-task"
            />
            <Card
              label="Cost / Order"
              value={fmtOrDash(sim?.cost_per_order_km, (v) => v.toFixed(2) + 'km', hasSim && (sim?.orders_delivered ?? 0) > 0)}
              sub="km on-task / delivered"
            />
            <Card
              label="Orders / min"
              value={fmtOrDash(kpi?.orders_per_min, (v) => v.toFixed(1), !stale && kpi !== null)}
              sub="demand 60s window"
            />
            <Card
              label="P99 Dispatch"
              value={fmtOrDash(sim?.p99_dispatch_ms, (v) => (v >= 100 ? v.toFixed(0) : v.toFixed(1)) + 'ms', hasSim && sim.dispatch_calls > 0)}
              sub={`target < ${DISPATCH_TARGET_MS}ms`}
              violated={dispViolated}
              tone={dispViolated ? 'coral' : undefined}
            />
            <Card
              label="Queue"
              value={fmtOrDash(sim?.orders_waiting, (v) => v.toFixed(0), hasSim)}
              sub={kpi?.queue.ingestion_inflight !== null && kpi?.queue.ingestion_inflight !== undefined ? `ingest inflight ${kpi.queue.ingestion_inflight}` : 'order menunggu'}
            />
            <Card
              label="Kafka Lag"
              value={fmtOrDash(kpi?.pipeline.lag_msgs, (v) => v.toFixed(0), kpi?.pipeline.lag_msgs !== null && kpi?.pipeline.lag_msgs !== undefined)}
              sub={kpi?.pipeline.lag_msgs === null || kpi === null ? 'pipeline standby' : 'topic backlog'}
              tone={kpi && (kpi.pipeline.lag_msgs ?? 0) > 0 ? 'coral' : undefined}
            />
          </div>

          <div className="mt-2 rounded-card border border-line-subtle bg-surface-base/60 px-2 pt-2">
            <StreamChart chart={chart} fresh={!stale && !!kpi} />
          </div>

          <p className="mt-1.5 font-mono text-[8px] leading-relaxed text-ink-muted">
            SUMBER: ENGINE.METRICS + COUNTERS PIPELINE + INCIDENT CHAOS — BUKAN ANGKA HIAS
            {kpi && !stale && ` · GRID ${countUp(kpi.grid)}/${kpi.grid.length} UP`}
          </p>
        </div>
      )}
    </aside>
  );
}

function countUp(grid: GridNode[]): number {
  return grid.filter((n) => n.status === 'up').length;
}

const KpiDeck = memo(KpiDeckImpl);
export default KpiDeck;
