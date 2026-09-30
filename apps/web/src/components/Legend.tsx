'use client';

import { memo } from 'react';
import { riderStatusColor, tokens } from '@/lib/tokens';

function Dot({ color }: { color: string }) {
  return (
    <span
      aria-hidden
      className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
      style={{ backgroundColor: color, boxShadow: `0 0 6px ${color}66` }}
    />
  );
}

function DashedLine({ color }: { color: string }) {
  return (
    <span
      aria-hidden
      className="inline-block h-0 w-5 shrink-0 border-t-2 border-dashed"
      style={{ borderColor: color }}
    />
  );
}

/** Legenda peta — warna selalu dari token (DESIGN.md §2). */
function LegendImpl() {
  const t = tokens.color;
  return (
    <aside
      aria-label="Legenda peta"
      className="absolute bottom-4 left-4 z-10 rounded-panel border border-line-subtle bg-surface-raised/85 px-4 py-3 backdrop-blur"
    >
      <div className="mb-2 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        Riders
      </div>
      <div className="grid grid-cols-2 gap-x-5 gap-y-1.5">
        <LegendRow icon={<Dot color={riderStatusColor[0]} />} label="Idle" />
        <LegendRow icon={<Dot color={riderStatusColor[1]} />} label="To pickup" />
        <LegendRow icon={<Dot color={riderStatusColor[2]} />} label="Pickup" />
        <LegendRow icon={<Dot color={riderStatusColor[3]} />} label="Delivering" />
      </div>
      <div className="mb-2 mt-3 font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        Orders
      </div>
      <div className="flex flex-col gap-1.5">
        <LegendRow icon={<Dot color={t.accentCyan} />} label="Waiting (radar pulse)" />
        <LegendRow icon={<DashedLine color={t.statusAmber} />} label="To pickup" />
        <LegendRow icon={<DashedLine color={t.statusViolet} />} label="In transit" />
      </div>
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
