'use client';

import { MotionConfig, useReducedMotion } from 'framer-motion';
import { useEffect, useRef, useState } from 'react';
import DemoLauncher from './DemoLauncher';
import HelpOverlay from './HelpOverlay';
import KpiDeck from './KpiDeck';
import Legend from './Legend';
import LiveMap, { type LiveStreamRef } from './LiveMap';
import ReplayBanner from './ReplayBanner';
import ReplayPanel from './ReplayPanel';
import StrategyLab from './StrategyLab';
import SurgeConsole from './SurgeConsole';
import SystemHealth from './SystemHealth';
import TopBar from './TopBar';
import type { ReplayOverride } from '@/lib/replay';
import { useKpi } from '@/lib/useKpi';
import { useOpsStream } from '@/lib/useOpsStream';

const GUIDE_SEEN_KEY = 'pulse.guide.seen.v1';

export default function OpsApp() {
  const stream = useOpsStream();
  const kpiState = useKpi();
  const reduced = useReducedMotion() ?? false;
  const [labOpen, setLabOpen] = useState(false);
  const [deckOpen, setDeckOpen] = useState(true);
  // Panduan penggunaan: auto-buka sekali untuk pengunjung pertama, selalu
  // bisa dibuka lewat tombol "? GUIDE" di TopBar. Static overlay, tanpa rAF.
  const [guideOpen, setGuideOpen] = useState(false);
  useEffect(() => {
    try {
      if (!window.localStorage.getItem(GUIDE_SEEN_KEY)) {
        setGuideOpen(true);
        window.localStorage.setItem(GUIDE_SEEN_KEY, '1');
      }
    } catch {
      /* storage diblokir — panduan tetap ada via tombol */
    }
  }, []);
  useEffect(() => {
    if (!guideOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setGuideOpen(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [guideOpen]);

  // Ref stabil untuk loop render canvas (tanpa re-mount map tiap render React).
  const streamRef = useRef<LiveStreamRef>({
    getPair: stream.getPair,
    mode: stream.mode,
    reduced,
  });
  // Override replay (Fase 5): pasangan replay menang atas live/fixture;
  // saat null aliran normal dipakai — tanpa rAF baru, render on-demand.
  const overrideRef = useRef<ReplayOverride>({ getPair: () => null });
  streamRef.current.getPair = (now) => overrideRef.current.getPair(now) ?? stream.getPair(now);
  streamRef.current.mode = stream.mode;
  streamRef.current.reduced = reduced;

  return (
    <MotionConfig reducedMotion="user">
      <div className="flex h-dvh min-h-[540px] flex-col bg-surface-base">
        <TopBar mode={stream.mode} stats={stream.stats} incidentsOpen={kpiState.kpi?.incidents_open ?? null} onGuide={() => setGuideOpen(true)} />
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
          {!labOpen && !deckOpen && <Legend />}
          <KpiDeck kpiState={kpiState} onOpenChange={setDeckOpen} />
          <StrategyLab onOpenChange={setLabOpen} />
          <SurgeConsole mode={stream.mode} stats={stream.stats} />
          <SystemHealth kpiState={kpiState} />
          <HelpOverlay open={guideOpen} onClose={() => setGuideOpen(false)} />
          {/* dock kontrol bawah (Fase 5): Golden Demo + Replay & Inspect */}
          <div className="pointer-events-none absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 items-center gap-2">
            {/* atribusi data wajib (ODbL) — dipindah dari footer lama */}
            <span className="hidden font-mono text-[9px] tracking-[0.06em] text-ink-muted lg:inline">
              data © OpenStreetMap
            </span>
            <DemoLauncher />
            <ReplayPanel streamRef={streamRef} overrideRef={overrideRef} />
          </div>
        </main>
      </div>
    </MotionConfig>
  );
}
