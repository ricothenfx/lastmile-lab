'use client';

import { memo } from 'react';
import type { Snapshot } from '@/lib/protocol';
import type { OpsMode } from '@/lib/useOpsStream';
import StatusPill from './StatusPill';

const fmt = new Intl.NumberFormat('en-US');

function Ticker({ label, value }: { label: string; value: number | string }) {
  return (
    <span className="flex shrink-0 flex-col leading-tight">
      <span className="text-[10px] uppercase tracking-[0.08em] text-ink-secondary">
        {label}
      </span>
      <span className="font-mono text-[13px] font-semibold tabular-nums text-ink-primary">
        {typeof value === 'number' ? fmt.format(value) : value}
      </span>
    </span>
  );
}

function TopBarImpl({ mode, stats }: { mode: OpsMode; stats: Snapshot['st'] | null }) {
  return (
    <header className="z-20 flex h-14 shrink-0 items-center justify-between gap-3 border-b border-line-subtle bg-surface-raised px-4">
      <div className="flex min-w-0 items-baseline gap-3">
        <span className="whitespace-nowrap font-mono text-[15px] font-bold tracking-[0.15em] text-ink-primary">
          PULSE<span className="text-accent-cyan">·</span>
        </span>
        <span className="hidden truncate text-[11px] uppercase tracking-[0.08em] text-ink-secondary sm:inline">
          Live Ops — Berlin
        </span>
      </div>
      <div className="flex items-center gap-3 sm:gap-5">
        <div className="hidden items-center gap-5 md:flex">
          <Ticker label="Active" value={stats ? stats.ac : '—'} />
          <Ticker label="Idle" value={stats ? stats.id : '—'} />
          <Ticker label="Delivered" value={stats ? stats.dl : '—'} />
          <Ticker label="Expired" value={stats ? stats.ex : '—'} />
          <Ticker label="Strategy" value={stats?.stg ?? '—'} />
        </div>
        <div className="flex items-center gap-3 md:hidden">
          <Ticker label="Active" value={stats ? stats.ac : '—'} />
          <Ticker label="Delivered" value={stats ? stats.dl : '—'} />
        </div>
        <StatusPill mode={mode} />
      </div>
    </header>
  );
}

const TopBar = memo(TopBarImpl);
export default TopBar;
