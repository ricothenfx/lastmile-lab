'use client';

/**
 * GuideOverlay — in-app usage guide (first-run + "?" button in TopBar).
 * Static render (no animation) — zero rAF budget, reduced-motion safe.
 * Colors/typography 100% via design tokens, same as other panels.
 */

import { memo } from 'react';

const SECTIONS: { h: string; rows: [string, string][] }[] = [
  {
    h: 'What am I looking at',
    rows: [
      ['PULSE', 'A control room for a simulated last-mile delivery fleet in Berlin: 100 riders, orders flowing through a Kafka pipeline, all of it running live. Every number on screen is a real metric from that system — nothing is decorative.'],
    ],
  },
  {
    h: 'The map',
    rows: [
      ['Rider dots', 'idle (lime) · to pickup (amber) · pickup (cyan) · delivering (violet). They move on a live 10 Hz feed.'],
      ['Order dots', 'cyan with a radar pulse = waiting for a rider; small violet dot = dropoff in progress.'],
      ['Dashed lines', 'a live assignment: amber = rider heading to pickup, violet = rider delivering.'],
      ['Hover a dot', 'quick info card: rider status, carried order, distance to target — right on the map.'],
      ['Click a dot', 'live: an inspect card with order, age and route. Replay: the full card including the dispatch decision reason.'],
      ['Street names', 'zoom in (scroll) — major roads label first (from 13), minor streets from zoom 14.'],
      ['Breathing glow', 'order density per ~500 m cell; it brightens and breathes faster as surge rises. No orders = no heat.'],
      ['Delivery burst', 'a short violet ring = an order delivered; a faint coral ring = an order expired.'],
    ],
  },
  {
    h: 'Top bar',
    rows: [
      ['Active / Idle', 'riders on-task vs waiting for work.'],
      ['Delivered / Expired', 'completed vs missed TTL — expired eats revenue.'],
      ['Incidents', 'open chaos incidents (node down, self-healing).'],
      ['INTERVIEW', 'the /interview page: questions an interviewer would ask about this system, answered with ADRs and measured numbers.'],
      ['LIVE pill', 'LIVE = websocket stream; REPLAY = recorded fallback when the stream is down; CONNECTING = handshake.'],
    ],
  },
  {
    h: 'KPI command deck',
    rows: [
      ['Delivery P50 / P95', 'median & tail delivery time vs the SLO (< 6 min). Coral border = SLO violated.'],
      ['Utilization / Cost', 'share of fleet time on task; on-task km per delivered order.'],
      ['P99 Dispatch', 'how fast the dispatch engine decides — target < 50 ms (pure algorithm, no LLM).'],
      ['Queue / Kafka lag', 'orders waiting for dispatch; pipeline backlog.'],
      ['Gauge', 'combined SLO health: lime all good, amber unknown, coral violation.'],
    ],
  },
  {
    h: 'Surge console',
    rows: [
      ['×1.0 slider', 'demand multiplier (×1–×10) — drags the live simulation instantly.'],
      ['RAIN / FLASH SALE', 'one-click scenario presets: rain slows riders 40%, flash sale floods demand.'],
    ],
  },
  {
    h: 'Strategy lab',
    rows: [
      ['DUEL', 'race two dispatch strategies (fifo vs optimal) on the same scenario and seed, then compare delivered/expired/p50/cost.'],
      ['ADVISOR (if visible)', 'an LLM proposes up to 3 plans; the simulator (same seed) computes the prediction; execution stays a human two-click button. Hidden until the copilot backend is enabled.'],
    ],
  },
  {
    h: 'System health',
    rows: [
      ['PIPELINE', 'service grid (up/down), live health events.'],
      ['CHAOS', 'SIGKILL a stateless container and watch self-heal — recovery time lands in the timeline.'],
      ['COPILOT (if visible)', 'Ask Ops: answers must cite internal metrics; uncited answers are rejected, never shown.'],
    ],
  },
  {
    h: 'Bottom dock',
    rows: [
      ['▶ DEMO', 'guided 90 s story: demand surge → rain → chaos kill → self-heal, narrated step by step.'],
      ['REPLAY', 'scrub the 15-minute recording buffer at ×1–×16, pause anywhere, inspect any entity. Works even while offline (fixture loop).'],
    ],
  },
];

function HelpOverlayImpl({ open, onClose }: { open: boolean; onClose: () => void }) {
  if (!open) return null;
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="How to use Pulse"
      className="fixed inset-0 z-50 grid place-items-center bg-surface-base/95 p-4 backdrop-blur-md"
      onClick={onClose}
    >
      <div
        className="max-h-[86vh] w-[min(760px,100vw-2rem)] overflow-y-auto rounded-panel border border-line-subtle bg-surface-raised p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="font-mono text-[13px] font-bold tracking-[0.15em] text-ink-primary">
              HOW TO USE PULSE
            </p>
            <p className="mt-0.5 font-mono text-[10px] leading-relaxed text-ink-muted">
              Every panel below runs on real data from the live pipeline — this guide explains what each one means.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close guide"
            className="shrink-0 rounded-input border border-line-subtle px-2 py-1 font-mono text-[11px] text-ink-secondary transition-colors duration-fast hover:text-ink-primary"
          >
            ✕ ESC
          </button>
        </div>

        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          {SECTIONS.map((s) => (
            <section key={s.h} className="rounded-card border border-line-subtle bg-surface-base/50 px-3 py-2.5">
              <h2 className="font-mono text-[10px] uppercase tracking-[0.12em] text-accent-cyan">
                {s.h}
              </h2>
              <dl className="mt-1.5 space-y-1.5">
                {s.rows.map(([k, v]) => (
                  <div key={k}>
                    <dt className="font-mono text-[10px] uppercase tracking-[0.06em] text-ink-secondary">
                      {k}
                    </dt>
                    <dd className="text-[11px] leading-relaxed text-ink-secondary">{v}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>

        <p className="mt-4 font-mono text-[9px] leading-relaxed text-ink-muted">
          TIP — this whole stack is open source: deterministic dispatch core, order service,
          chaos injector, 15-min replay recorder and a citation-locked ops copilot. Press ESC
          or click outside to close this guide.
        </p>
      </div>
    </div>
  );
}

const HelpOverlay = memo(HelpOverlayImpl);
export default HelpOverlay;
