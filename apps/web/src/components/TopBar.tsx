'use client';

import Link from 'next/link';
import { memo } from 'react';
import type { Snapshot } from '@/lib/protocol';
import type { OpsMode } from '@/lib/useOpsStream';
import { toggleTheme } from '@/lib/theme';
import StatusPill from './StatusPill';

const fmt = new Intl.NumberFormat('en-US');

/** Ikon tema via CSS (bukan state) — nol re-render, nol hydration mismatch. */
function ThemeToggle() {
  return (
    <button
      type="button"
      onClick={toggleTheme}
      title="Switch between dark and light map & dashboard theme"
      aria-label="Toggle dark or light theme"
      className="flex h-[26px] w-[26px] items-center justify-center rounded-pill border border-line-subtle bg-surface-overlay text-ink-secondary transition-colors duration-fast hover:border-accent-cyan/60 hover:text-accent-cyan"
    >
      {/* Matahari: tampil saat dark (klik → light). Bulan: sebaliknya. */}
      <svg
        aria-hidden
        className="theme-icon-sun h-[14px] w-[14px]"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      >
        <circle cx="12" cy="12" r="4" />
        <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
      </svg>
      <svg
        aria-hidden
        className="theme-icon-moon h-[14px] w-[14px]"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
      </svg>
    </button>
  );
}

function Ticker({ label, value, tone }: { label: string; value: number | string; tone?: 'coral' }) {
  return (
    <span className="flex shrink-0 flex-col leading-tight">
      <span className="text-[10px] uppercase tracking-[0.08em] text-ink-secondary">
        {label}
      </span>
      <span
        className={`font-mono text-[13px] font-semibold tabular-nums ${
          tone === 'coral' ? 'text-status-coral' : 'text-ink-primary'
        }`}
      >
        {typeof value === 'number' ? fmt.format(value) : value}
      </span>
    </span>
  );
}

function TopBarImpl({
  mode,
  stats,
  incidentsOpen,
  onGuide,
}: {
  mode: OpsMode;
  stats: Snapshot['st'] | null;
  incidentsOpen: number | null;
  onGuide?: () => void;
}) {
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
          <Ticker
            label="Incidents"
            value={incidentsOpen ?? '—'}
            tone={incidentsOpen !== null && incidentsOpen > 0 ? 'coral' : undefined}
          />
          <Ticker label="Strategy" value={stats?.stg ?? '—'} />
        </div>
        <div className="flex items-center gap-3 md:hidden">
          <Ticker label="Active" value={stats ? stats.ac : '—'} />
          <Ticker label="Delivered" value={stats ? stats.dl : '—'} />
        </div>
        <ThemeToggle />
        <Link
          href="/interview"
          title="Questions an interviewer would ask — with answers from the repo"
          className="hidden rounded-pill border border-line-subtle bg-surface-overlay px-3 py-1 font-mono text-[11px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:border-accent-cyan/60 hover:text-accent-cyan sm:inline-block"
        >
          INTERVIEW
        </Link>
        {onGuide && (
          <button
            type="button"
            onClick={onGuide}
            title="How to use this dashboard"
            aria-label="Open usage guide"
            className="rounded-pill border border-line-subtle bg-surface-overlay px-3 py-1 font-mono text-[11px] tracking-[0.08em] text-ink-secondary transition-colors duration-fast hover:border-accent-cyan/60 hover:text-accent-cyan"
          >
            ? GUIDE
          </button>
        )}
        <StatusPill mode={mode} />
      </div>
    </header>
  );
}

const TopBar = memo(TopBarImpl);
export default TopBar;
