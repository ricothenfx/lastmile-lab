'use client';

/**
 * AdvisorPanel (Fase 7) — Plan Advisor di tab ADVISOR Strategy Lab.
 * HANYA dirender bila copilot enabled (pemanggil yang mengontrol); tanpa key
 * komponen ini tidak pernah muncul di DOM. LLM mengusulkan ≤3 plan; setiap
 * plan di-dry-run simulator (angka prediksi nyata); EXECUTE per plan dengan
 * konfirmasi 2 langkah (pola chaos) — eksekusi via endpoint kontrol existing.
 */

import { useState } from 'react';
import {
  executeAction,
  postCopilotAdvise,
  type CopilotAdvise,
  type CopilotPlanResult,
} from '@/lib/copilot';

export default function AdvisorPanel() {
  const [phase, setPhase] = useState<'idle' | 'busy' | 'done' | 'error'>('idle');
  const [advise, setAdvise] = useState<CopilotAdvise | null>(null);
  const [errorMsg, setErrorMsg] = useState('');
  const [armedIdx, setArmedIdx] = useState<number | null>(null);
  const [execMsg, setExecMsg] = useState('');

  const run = async () => {
    setPhase('busy');
    setErrorMsg('');
    setExecMsg('');
    setArmedIdx(null);
    const out = await postCopilotAdvise();
    if (out.error || !out.data) {
      setPhase('error');
      setErrorMsg(out.error ?? 'advise failed');
      return;
    }
    setAdvise(out.data);
    setPhase('done');
  };

  const execute = async (pr: CopilotPlanResult, idx: number) => {
    if (armedIdx !== idx) {
      setArmedIdx(idx); // langkah 1: arm
      return;
    }
    setArmedIdx(null); // langkah 2: konfirmasi
    const parts: string[] = [];
    for (const a of pr.plan.actions) {
      const res = await executeAction(a);
      parts.push(
        `${a.kind.toUpperCase()} ${res.ok ? 'OK' : res.skipped ? 'SKIP' : 'FAIL'}${res.error ? ` (${res.error})` : ''}`,
      );
    }
    setExecMsg(parts.join(' · '));
  };

  const busy = phase === 'busy';

  return (
    <div data-testid="copilot-advisor">
      <p className="font-mono text-[9px] leading-relaxed text-ink-muted">
        THE LLM PROPOSES ≤3 PLANS — THE SIMULATOR (DUEL, SAME SEED) COMPUTES THE PREDICTIONS.
        EXECUTION STAYS A HUMAN BUTTON.
      </p>
      <button
        type="button"
        onClick={() => void run()}
        disabled={busy}
        className="mt-1.5 w-full rounded-input border border-status-violet/60 bg-status-violet/10 px-3 py-1.5 font-mono text-[11px] tracking-[0.08em] text-status-violet transition-colors duration-fast hover:bg-status-violet/20 disabled:cursor-not-allowed disabled:opacity-40"
      >
        {busy ? 'ADVISING + DRY-RUN…' : phase === 'done' ? 'REGENERATE' : 'GENERATE PLANS'}
      </button>

      {phase === 'error' && (
        <p className="mt-2 font-mono text-[10px] leading-relaxed text-status-coral" role="alert">
          {errorMsg.toUpperCase()}
        </p>
      )}

      {advise && (
        <>
          <p className="mt-2 font-mono text-[9px] tabular-nums text-ink-secondary">
            DRY-RUN {advise.seconds}s · SEED {advise.seed} · BASELINE {advise.base_rate_per_min.toFixed(0)}/MIN
          </p>
          <div className="mt-1.5 space-y-2">
            {advise.plans.map((pr, i) => (
              <PlanCard
                key={`${pr.plan.name}-${i}`}
                idx={i}
                pr={pr}
                armed={armedIdx === i}
                onExecute={() => void execute(pr, i)}
              />
            ))}
          </div>
          {execMsg && (
            <p className="mt-1.5 font-mono text-[9px] text-status-amber" role="status">
              {execMsg}
            </p>
          )}
        </>
      )}
    </div>
  );
}

function PlanCard({
  idx,
  pr,
  armed,
  onExecute,
}: {
  idx: number;
  pr: CopilotPlanResult;
  armed: boolean;
  onExecute: () => void;
}) {
  const b = pr.baseline;
  const p = pr.predicted;
  const delta = (bv: number, pv: number, inverse = false) => {
    if (!b || !p || bv === 0) return null;
    const pct = ((pv - bv) / bv) * 100;
    const good = inverse ? pct < 0 : pct > 0;
    return (
      <span className={Math.abs(pct) < 0.5 ? 'text-ink-muted' : good ? 'text-accent-lime' : 'text-status-coral'}>
        {pct >= 0 ? '+' : ''}
        {pct.toFixed(0)}%
      </span>
    );
  };

  return (
    <div className="rounded-card border border-line-subtle px-2 py-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="min-w-0 truncate font-mono text-[10px] text-ink-primary">
          {String.fromCharCode(65 + idx)}. {pr.plan.name}
        </span>
        <button
          type="button"
          onClick={onExecute}
          aria-label={`Execute plan ${pr.plan.name}${armed ? ' — click again to confirm' : ''}`}
          className={`shrink-0 rounded-input border px-2 py-0.5 font-mono text-[9px] tracking-[0.06em] transition-colors duration-fast ${
            armed
              ? 'border-status-coral bg-status-coral/20 text-status-coral'
              : 'border-line-subtle text-ink-secondary hover:text-ink-primary'
          }`}
        >
          {armed ? 'CONFIRM?' : 'EXECUTE'}
        </button>
      </div>
      <p className="mt-0.5 font-mono text-[9px] leading-relaxed text-ink-secondary">{pr.plan.rationale}</p>
      <div className="mt-1 flex flex-wrap gap-1">
        {pr.plan.actions.map((a, i) => (
          <span
            key={i}
            className="rounded-pill border border-line-subtle px-1.5 py-px font-mono text-[8px] uppercase tracking-[0.06em] text-ink-muted"
          >
            {a.kind} {Object.values(a.params).join(' ')}
          </span>
        ))}
      </div>
      {p && b ? (
        <table className="mt-1.5 w-full font-mono text-[9px] tabular-nums">
          <thead>
            <tr className="text-ink-muted">
              <th className="text-left font-normal">METRIC</th>
              <th className="text-right font-normal">BASE</th>
              <th className="text-right font-normal">PLAN</th>
              <th className="text-right font-normal">Δ</th>
            </tr>
          </thead>
          <tbody className="text-ink-secondary">
            <Row label="delivered" base={b.delivered} plan={p.delivered} fmt={(v) => String(v)} d={delta(b.delivered, p.delivered)} />
            <Row label="expired" base={b.expired} plan={p.expired} fmt={(v) => String(v)} d={delta(b.expired, p.expired, true)} />
            <Row label="p50" base={b.delivery_p50_ms} plan={p.delivery_p50_ms} fmt={ms} d={delta(b.delivery_p50_ms, p.delivery_p50_ms, true)} />
            <Row label="p95" base={b.delivery_p95_ms} plan={p.delivery_p95_ms} fmt={ms} d={delta(b.delivery_p95_ms, p.delivery_p95_ms, true)} />
            <Row label="cost/order" base={b.cost_per_order_km} plan={p.cost_per_order_km} fmt={km} d={delta(b.cost_per_order_km, p.cost_per_order_km, true)} />
          </tbody>
        </table>
      ) : (
        pr.note && (
          <p className="mt-1 font-mono text-[8px] leading-relaxed text-ink-muted">{pr.note}</p>
        )
      )}
    </div>
  );
}

function Row({
  label,
  base,
  plan,
  fmt,
  d,
}: {
  label: string;
  base: number;
  plan: number;
  fmt: (v: number) => string;
  d: React.ReactNode;
}) {
  return (
    <tr>
      <td className="text-left">{label}</td>
      <td className="text-right">{fmt(base)}</td>
      <td className="text-right text-ink-primary">{fmt(plan)}</td>
      <td className="text-right">{d ?? '—'}</td>
    </tr>
  );
}

const ms = (v: number) => (v >= 1000 ? `${(v / 1000).toFixed(1)}s` : `${v.toFixed(0)}ms`);
const km = (v: number) => `${v.toFixed(2)}km`;
