'use client';

import { AnimatePresence, motion } from 'framer-motion';

export default function ReplayBanner({ visible }: { visible: boolean }) {
  return (
    <AnimatePresence>
      {visible && (
        <motion.div
          role="status"
          initial={{ y: -12, opacity: 0 }}
          animate={{ y: 0, opacity: 1 }}
          exit={{ y: -12, opacity: 0 }}
          transition={{ duration: 0.2, ease: 'easeOut' }}
          className="pointer-events-none absolute left-1/2 top-3 z-20 -translate-x-1/2"
        >
          <div className="flex items-center gap-2 whitespace-nowrap rounded-pill border border-status-coral/40 bg-surface-overlay/90 px-4 py-1.5 backdrop-blur">
            <span className="h-2 w-2 rounded-full bg-status-coral" />
            <span className="font-mono text-[11px] tracking-[0.06em] text-status-coral">
              REPLAY MODE
            </span>
            <span className="hidden text-[11px] text-ink-secondary sm:inline">
              live feed unavailable — playing recorded demo data
            </span>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
