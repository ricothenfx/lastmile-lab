'use client';

/**
 * MiniReplay — satu kanvas peta replay duel Strategy Lab (Fase 3).
 *
 * Render ON-DEMAND: hanya menggambar ulang saat frame berganti (playback atau
 * scrub), bukan loop per frame saat idle — budget 60fps aman (DoD fase 3).
 * Latar jalan di-prerender sekali ke offscreen canvas; titik rider/order
 * memakai warna token yang sama dengan Live Ops Map (DESIGN.md §2).
 */

import { memo, useEffect, useRef } from 'react';
import { riderStatusColor, tokens } from '@/lib/tokens';
import { ORDER_STATUS, type Snapshot } from '@/lib/protocol';
import type { GraphJSON } from '@/lib/lab';

interface Props {
  frame: Snapshot | null;
  graph: GraphJSON | null;
  side: 'a' | 'b';
}

function MiniReplayImpl({ frame, graph, side }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const bgRef = useRef<HTMLCanvasElement | null>(null);
  const sizeRef = useRef({ w: 0, h: 0 });

  // Prerender latar jalan (sekali per ukuran/graph) + sinkron ukuran kanvas.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !graph) return;
    const parent = canvas.parentElement;
    if (!parent) return;

    const build = () => {
      const w = Math.max(80, parent.clientWidth);
      const h = Math.max(60, parent.clientHeight);
      sizeRef.current = { w, h };
      const dpr = Math.min(2, window.devicePixelRatio || 1);
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

      let minLat = Infinity;
      let maxLat = -Infinity;
      let minLon = Infinity;
      let maxLon = -Infinity;
      for (const [la, lo] of graph.nodes) {
        if (la < minLat) minLat = la;
        if (la > maxLat) maxLat = la;
        if (lo < minLon) minLon = lo;
        if (lo > maxLon) maxLon = lo;
      }
      const spanLat = Math.max(1e-6, maxLat - minLat);
      const spanLon = Math.max(1e-6, maxLon - minLon);
      // aspect-fit dengan padding 4px
      const pad = 4;
      const scale = Math.min((w - pad * 2) / spanLon, (h - pad * 2) / spanLat);
      const offX = (w - spanLon * scale) / 2;
      const offY = (h - spanLat * scale) / 2;
      const project = (la: number, lo: number): [number, number] => [
        offX + (lo - minLon) * scale,
        h - (offY + (la - minLat) * scale),
      ];
      projRef.current = project;

      const bg = document.createElement('canvas');
      bg.width = canvas.width;
      bg.height = canvas.height;
      const bctx = bg.getContext('2d');
      if (!bctx) return;
      bctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      bctx.fillStyle = tokens.color.bgBase;
      bctx.fillRect(0, 0, w, h);
      bctx.strokeStyle = tokens.color.mapRoad;
      bctx.globalAlpha = 0.85;
      bctx.lineWidth = 1;
      bctx.beginPath();
      for (const [a, b] of graph.edges) {
        const na = graph.nodes[a];
        const nb = graph.nodes[b];
        if (!na || !nb) continue;
        const [x0, y0] = project(na[0], na[1]);
        const [x1, y1] = project(nb[0], nb[1]);
        bctx.moveTo(x0, y0);
        bctx.lineTo(x1, y1);
      }
      bctx.stroke();
      bctx.globalAlpha = 1;
      bgRef.current = bg;
      // gambar ulang frame aktif dengan proyeksi baru
      drawRef.current?.();
    };

    build();
    const ro = new ResizeObserver(build);
    ro.observe(parent);
    return () => ro.disconnect();
  }, [graph]);

  const projRef = useRef<((la: number, lo: number) => [number, number]) | null>(null);
  const drawRef = useRef<(() => void) | null>(null);

  // Gambar frame — hanya saat frame/label berubah (on-demand).
  useEffect(() => {
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;
    const draw = () => {
      const { w, h } = sizeRef.current;
      const project = projRef.current;
      ctx.clearRect(0, 0, w, h);
      if (bgRef.current) ctx.drawImage(bgRef.current, 0, 0, w, h);
      if (!frame || !project) return;

      // order: waiting cyan · assigned amber (+ garis ke rider) · transit violet
      for (const o of frame.o) {
        const [px, py] = project(o.pa, o.po);
        const [dx, dy] = project(o.da, o.do);
        if (o.s === ORDER_STATUS.waiting) {
          ctx.fillStyle = tokens.color.accentCyan;
          ctx.beginPath();
          ctx.arc(px, py, 2, 0, Math.PI * 2);
          ctx.fill();
        } else {
          const rider = frame.r.find((r) => r.i === frame.l.find((l) => l.o === o.i)?.r);
          if (rider) {
            const [rx, ry] = project(rider.la, rider.lo);
            ctx.strokeStyle =
              o.s === ORDER_STATUS.inTransit ? tokens.color.statusViolet : tokens.color.statusAmber;
            ctx.globalAlpha = 0.45;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(rx, ry);
            ctx.lineTo(o.s === ORDER_STATUS.inTransit ? dx : px, o.s === ORDER_STATUS.inTransit ? dy : py);
            ctx.stroke();
            ctx.globalAlpha = 1;
          }
          if (o.s === ORDER_STATUS.assigned) {
            ctx.fillStyle = tokens.color.statusAmber;
            ctx.beginPath();
            ctx.arc(px, py, 2, 0, Math.PI * 2);
            ctx.fill();
          } else {
            ctx.fillStyle = tokens.color.statusViolet;
            ctx.beginPath();
            ctx.arc(dx, dy, 2, 0, Math.PI * 2);
            ctx.fill();
          }
        }
      }
      // rider: titik berwarna status
      for (const r of frame.r) {
        const [x, y] = project(r.la, r.lo);
        ctx.fillStyle = riderStatusColor[r.s] ?? tokens.color.accentLime;
        ctx.beginPath();
        ctx.arc(x, y, 2.4, 0, Math.PI * 2);
        ctx.fill();
      }
    };
    drawRef.current = draw;
    draw();
  }, [frame, side]);

  return (
    <div className="relative h-[120px] w-full overflow-hidden rounded-card border border-line-subtle bg-surface-base">
      <canvas ref={canvasRef} className="absolute inset-0 h-full w-full" aria-hidden />
      <span className="pointer-events-none absolute left-2 top-1.5 font-mono text-[9px] uppercase tracking-[0.12em] text-ink-secondary">
        {side === 'a' ? 'A' : 'B'}
      </span>
    </div>
  );
}

const MiniReplay = memo(MiniReplayImpl);
export default MiniReplay;
