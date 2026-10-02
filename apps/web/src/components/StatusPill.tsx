'use client';

import { motion } from 'framer-motion';
import { memo } from 'react';
import type { OpsMode } from '@/lib/useOpsStream';

const PILL: Record<OpsMode, { label: string; dot: string; pulse: boolean }> = {
  live: { label: 'LIVE', dot: 'bg-accent-lime', pulse: true },
  connecting: { label: 'CONNECTING', dot: 'bg-status-amber', pulse: true },
  replay: { label: 'REPLAY', dot: 'bg-status-coral', pulse: false },
};

function StatusPillImpl({ mode }: { mode: OpsMode }) {
  const cfg = PILL[mode];
  return (
    <span
      role="status"
      aria-label={`Connection: ${cfg.label}`}
      className="flex shrink-0 items-center gap-2 rounded-pill border border-line-subtle bg-surface-overlay px-3 py-1"
    >
      {cfg.pulse ? (
        <motion.span
          className={`h-2 w-2 rounded-full ${cfg.dot}`}
          animate={{ opacity: [1, 0.35, 1], scale: [1, 1.25, 1] }}
          transition={{ duration: 2, repeat: Infinity, ease: 'easeOut' }}
        />
      ) : (
        <span className={`h-2 w-2 rounded-full ${cfg.dot}`} />
      )}
      <span className="font-mono text-[11px] tracking-[0.08em] text-ink-primary">
        {cfg.label}
      </span>
    </span>
  );
}

const StatusPill = memo(StatusPillImpl);
export default StatusPill;
