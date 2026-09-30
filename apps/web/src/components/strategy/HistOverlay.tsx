'use client';

/**
 * HistOverlay — histogram overlay delivery time A vs B pada bin BERSAMA
 * (satu sumbu, dihitung server — internal/duel.buildHistogram).
 * Render statis sekali per data/ukuran — tanpa loop animasi (DoD 60fps idle).
 */

import { memo, useEffect, useRef } from 'react';
import { tokens } from '@/lib/tokens';
import type { LabHistogram } from '@/lib/lab';

interface Props {
  hist: LabHistogram | null;
}

function HistOverlayImpl({ hist }: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const wrap = wrapRef.current;
    const canvas = canvasRef.current;
    if (!wrap || !canvas) return;

    const draw = () => {
      const w = Math.max(80, wrap.clientWidth);
      const h = Math.max(70, wrap.clientHeight);
      const dpr = Math.min(2, window.devicePixelRatio || 1);
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      const padL = 2;
      const padB = 14;
      const plotW = w - padL * 2;
      const plotH = h - padB - 4;

      // grid dasar
      ctx.strokeStyle = tokens.color.borderSubtle;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(padL, 4 + plotH);
      ctx.lineTo(padL + plotW, 4 + plotH);
      ctx.stroke();

      if (!hist || hist.edges_ms.length < 2) {
        ctx.fillStyle = tokens.color.textMuted;
        ctx.font = '10px ui-monospace, monospace';
        ctx.fillText('NO DELIVERY DATA', padL + 4, 4 + plotH / 2);
        return;
      }

      const bins = hist.count_a.length;
      const maxCount = Math.max(1, ...hist.count_a, ...hist.count_b);
      const binW = plotW / bins;
      const barW = Math.max(1.5, binW / 2 - 1);

      const bar = (counts: number[], offset: number, color: string) => {
        ctx.fillStyle = color + '55';
        ctx.strokeStyle = color;
        ctx.lineWidth = 1;
        for (let i = 0; i < bins; i++) {
          const bh = (counts[i] / maxCount) * plotH;
          if (bh <= 0) continue;
          const x = padL + i * binW + offset;
          const y = 4 + plotH - bh;
          ctx.fillRect(x, y, barW, bh);
          ctx.strokeRect(x, y, barW, bh);
        }
      };
      bar(hist.count_a, 0.5, tokens.color.accentCyan);
      bar(hist.count_b, binW / 2 + 0.5, tokens.color.statusViolet);

      // sumbu waktu: min → max (bin bersama)
      ctx.fillStyle = tokens.color.textSecondary;
      ctx.font = '9px ui-monospace, monospace';
      const lo = hist.edges_ms[0] / 1000;
      const hi = hist.edges_ms[hist.edges_ms.length - 1] / 1000;
      ctx.fillText(`${lo.toFixed(1)}s`, padL, h - 3);
      const label = `${hi.toFixed(1)}s`;
      ctx.fillText(label, padL + plotW - ctx.measureText(label).width, h - 3);
    };

    draw();
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [hist]);

  return (
    <div ref={wrapRef} className="relative h-[92px] w-full">
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" aria-label="Histogram delivery time A vs B" role="img" />
      <div className="pointer-events-none absolute right-1 top-0 flex gap-2 font-mono text-[9px] tracking-[0.08em]">
        <span className="flex items-center gap-1 text-ink-secondary">
          <span aria-hidden className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: tokens.color.accentCyan }} />
          A
        </span>
        <span className="flex items-center gap-1 text-ink-secondary">
          <span aria-hidden className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: tokens.color.statusViolet }} />
          B
        </span>
      </div>
    </div>
  );
}

const HistOverlay = memo(HistOverlayImpl);
export default HistOverlay;
