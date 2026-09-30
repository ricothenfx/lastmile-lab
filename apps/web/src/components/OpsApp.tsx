'use client';

import { MotionConfig, useReducedMotion } from 'framer-motion';
import { useRef, useState } from 'react';
import Legend from './Legend';
import LiveMap, { type LiveStreamRef } from './LiveMap';
import ReplayBanner from './ReplayBanner';
import StrategyLab from './StrategyLab';
import SurgeConsole from './SurgeConsole';
import TopBar from './TopBar';
import { useOpsStream } from '@/lib/useOpsStream';

export default function OpsApp() {
  const stream = useOpsStream();
  const reduced = useReducedMotion() ?? false;
  const [labOpen, setLabOpen] = useState(false);

  // Ref stabil untuk loop render canvas (tanpa re-mount map tiap render React).
  const streamRef = useRef<LiveStreamRef>({
    getPair: stream.getPair,
    mode: stream.mode,
    reduced,
  });
  streamRef.current.getPair = stream.getPair;
  streamRef.current.mode = stream.mode;
  streamRef.current.reduced = reduced;

  return (
    <MotionConfig reducedMotion="user">
      <div className="flex h-dvh min-h-[540px] flex-col bg-surface-base">
        <TopBar mode={stream.mode} stats={stream.stats} />
        <main className="relative min-h-0 flex-1 overflow-hidden">
          <LiveMap streamRef={streamRef} />
          <ReplayBanner visible={stream.mode === 'replay'} />
          {stream.mode === 'connecting' && (
            <div className="pointer-events-none absolute inset-0 z-10 grid place-items-center">
              <p
                className="rounded-card border border-line-subtle bg-surface-raised/85 px-4 py-2 font-mono text-[12px] tracking-[0.08em] text-ink-secondary backdrop-blur"
                role="status"
              >
                ESTABLISHING UPLINK…
              </p>
            </div>
          )}
          {!labOpen && <Legend />}
          <StrategyLab onOpenChange={setLabOpen} />
          <SurgeConsole mode={stream.mode} stats={stream.stats} />
          <footer className="pointer-events-none absolute bottom-4 right-16 z-10 hidden md:block">
            <p className="text-[11px] text-ink-secondary">
              lastmile-lab · phase 3 · data © OpenStreetMap contributors
            </p>
          </footer>
        </main>
      </div>
    </MotionConfig>
  );
}
