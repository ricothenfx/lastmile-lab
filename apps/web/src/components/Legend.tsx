'use client';

import { memo } from 'react';

/**
 * Legenda peta — ikon meniru simbol canvas LiveMap (motor / resto / rumah,
 * ADR D26). Warna 100% CSS variable dari tokens.ts → ikut tema aktif tanpa
 * re-render (DESIGN.md §2).
 */

function MotorIcon({ tone }: { tone: string }) {
  return (
    <svg aria-hidden viewBox="0 0 32 32" className="h-4 w-4 shrink-0" fill="none" stroke={`var(${tone})`} strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="7" cy="21" r="3.6" />
      <circle cx="25" cy="21" r="3.6" />
      <path d="M19.5 7h3M21 7l5.5 6L25 21M4 12.5h6.5M5.5 13.5 7 21m3.5-8.5 2 4.5h6l8-4" />
    </svg>
  );
}

function RestoIcon({ tone }: { tone: string }) {
  return (
    <svg aria-hidden viewBox="0 0 24 24" className="h-4 w-4 shrink-0" fill="none" stroke={`var(${tone})`} strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d="M7.4 4.5v4.7M10.6 4.5v4.7M7.4 9.2q0 2.6 1.6 2.6t1.6-2.6M9 11.8v7.7M15 19.5V4.5m0 0c2.8 1.8 2.8 5.5.2 7.3" />
    </svg>
  );
}

function HouseIcon({ tone }: { tone: string }) {
  return (
    <svg aria-hidden viewBox="0 0 24 24" className="h-4 w-4 shrink-0" fill="none" stroke={`var(${tone})`} strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 12.2 12 5l8 7.2M6.2 10.6v8.8h11.6v-8.8M10.2 19.4v-5.2h3.6v5.2" />
    </svg>
  );
}

function DashedLine({ tone }: { tone: string }) {
  return (
    <span
      aria-hidden
      className="inline-block h-0 w-5 shrink-0 border-t-2 border-dashed"
      style={{ borderColor: `var(${tone})` }}
    />
  );
}

/** Legenda peta — warna selalu dari token (DESIGN.md §2). */
function LegendImpl() {
  return (
    <aside
      aria-label="Map legend"
      className="absolute left-4 top-[68px] z-10 rounded-panel border border-line-subtle bg-surface-raised/85 px-4 py-3 backdrop-blur"
    >
      <div className="mb-2 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        Riders
      </div>
      <div className="grid grid-cols-2 gap-x-5 gap-y-1.5">
        <LegendRow icon={<MotorIcon tone="--accent-lime" />} label="Idle" />
        <LegendRow icon={<MotorIcon tone="--status-amber" />} label="To pickup" />
        <LegendRow icon={<MotorIcon tone="--accent-cyan" />} label="Pickup" />
        <LegendRow icon={<MotorIcon tone="--status-violet" />} label="Delivering" />
      </div>
      <div className="mb-2 mt-3 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        Orders
      </div>
      <div className="flex flex-col gap-1.5">
        <LegendRow icon={<RestoIcon tone="--accent-cyan" />} label="Waiting at restaurant" />
        <LegendRow icon={<RestoIcon tone="--status-amber" />} label="Assigned (pickup)" />
        <LegendRow icon={<HouseIcon tone="--status-violet" />} label="In transit (dropoff)" />
      </div>
      <div className="mb-2 mt-3 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        Map
      </div>
      <div className="flex flex-col gap-1.5">
        <LegendRow
          icon={
            <span
              aria-hidden
              className="inline-block h-3 w-3 shrink-0 rounded-full"
              style={{
                background:
                  'radial-gradient(circle, var(--status-coral) 0%, color-mix(in srgb, var(--status-amber) 40%, transparent) 55%, transparent 75%)',
              }}
            />
          }
          label="Order density (breathes with surge)"
        />
        <LegendRow icon={<DashedLine tone="--status-amber" />} label="Assignment (hover, or zoom in)" />
        <LegendRow icon={<RestoIcon tone="--status-amber" />} label="Restaurants (zoom in)" />
      </div>
      <p className="mt-3 border-t border-line-subtle pt-2 font-mono text-[9px] leading-relaxed text-ink-muted">
        Hover a motorcycle for quick info — click to inspect.
      </p>
    </aside>
  );
}

function LegendRow({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span className="flex items-center gap-2">
      {icon}
      <span className="text-[11px] text-ink-secondary">{label}</span>
    </span>
  );
}

const Legend = memo(LegendImpl);
export default Legend;
