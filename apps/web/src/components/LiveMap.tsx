'use client';

import { memo, useEffect, useRef } from 'react';
import type { Map as MLMap, StyleSpecification } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { riderStatusColor, tokens } from '@/lib/tokens';
import {
  ORDER_STATUS,
  RIDER_STATUS,
  type OrderPt,
  type RiderPt,
  type Snapshot,
} from '@/lib/protocol';
import type { FramePair, MapPick } from '@/lib/replay';
import type { OpsStream } from '@/lib/useOpsStream';

export interface LiveStreamRef {
  /** Bisa mengembalikan pasangan replay (override) atau pasangan live. */
  getPair: (now: number) => FramePair;
  mode: OpsStream['mode'];
  reduced: boolean;
  /** Terpasang saat override replay aktif — LiveMap hit-test klik peta. */
  onMapClick?: ((pick: MapPick | null) => void) | null;
  /** Entitas terpilih (kartu inspect) — digambar cincin highlight. */
  selected?: MapPick | null;
}

/** BBox Berlin inner-city — harus sinkron dengan apps/services graph meta. */
const BBOX: [[number, number], [number, number]] = [
  [13.37, 52.484],
  [13.461, 52.538],
];
const CENTER: [number, number] = [13.4155, 52.511];
const BASE_ZOOM = 12.15;

const GLOW_PX = 56;
const TRAIL_MAX = 14;
const PULSE_MS = 1600;

function makeGlowSprite(color: string): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = GLOW_PX;
  c.height = GLOW_PX;
  const g = c.getContext('2d')!;
  const grad = g.createRadialGradient(
    GLOW_PX / 2,
    GLOW_PX / 2,
    2,
    GLOW_PX / 2,
    GLOW_PX / 2,
    GLOW_PX / 2,
  );
  grad.addColorStop(0, color);
  grad.addColorStop(0.35, color + '55');
  grad.addColorStop(1, color + '00');
  g.fillStyle = grad;
  g.fillRect(0, 0, GLOW_PX, GLOW_PX);
  return c;
}

function buildStyle(): StyleSpecification {
  const t = tokens.color;
  return {
    version: 8,
    sources: {
      water: { type: 'geojson', data: '/berlin/water.geojson' },
      roads: { type: 'geojson', data: '/berlin/roads.geojson' },
    },
    layers: [
      { id: 'bg', type: 'background', paint: { 'background-color': t.bgBase } },
      {
        id: 'water-fill',
        type: 'fill',
        source: 'water',
        filter: ['==', ['geometry-type'], 'Polygon'],
        paint: { 'fill-color': t.mapWater, 'fill-opacity': 0.92 },
      },
      {
        id: 'water-line',
        type: 'line',
        source: 'water',
        filter: ['==', ['geometry-type'], 'LineString'],
        paint: {
          'line-color': t.mapWater,
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 3, 15, 12],
        },
      },
      {
        id: 'roads-minor',
        type: 'line',
        source: 'roads',
        filter: ['==', ['get', 'c'], 0],
        paint: {
          'line-color': t.mapRoad,
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 0.6, 14, 1.6, 17, 3.4],
          'line-opacity': 0.72,
        },
      },
      {
        id: 'roads-major',
        type: 'line',
        source: 'roads',
        filter: ['==', ['get', 'c'], 1],
        paint: {
          'line-color': t.mapRoad,
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 1.2, 14, 2.8, 17, 6],
          'line-opacity': 1,
        },
      },
    ],
  };
}

function LiveMapImpl({ streamRef }: { streamRef: React.MutableRefObject<LiveStreamRef> }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    let cancelled = false;
    let map: MLMap | null = null;
    let raf = 0;
    let ro: ResizeObserver | null = null;

    const canvas = document.createElement('canvas');
    canvas.style.position = 'absolute';
    canvas.style.inset = '0';
    canvas.style.width = '100%';
    canvas.style.height = '100%';
    canvas.style.pointerEvents = 'none';
    // Wajib: maplibre meng-append canvas WebGL-nya SETELAH overlay ini
    // (sama-sama absolute) — tanpa z-index eksplisit peta menutupi semua
    // dot/garis yang digambar (bug produksi 2026-10-02: buffer berisi
    // puluhan ribu piksel entitas tapi tak terlihat).
    canvas.style.zIndex = '1';
    container.appendChild(canvas);
    const ctx = canvas.getContext('2d')!;

    const glows = riderStatusColor.map(makeGlowSprite);
    const trails = new Map<number, number[][]>();
    const pulseSeen = new Map<string, number>();
    let dpr = 1;

    // Guard render on-demand (Fase 5): frame replay statis (pause/scrub)
    // hanya digambar ulang saat key frame berubah ATAU peta/viewport
    // berubah — tanpa kerja per-rAF saat diam, tanpa rAF baru.
    let lastDrawnKey: number | null = null;
    let lastSelKey: MapPick | null | undefined = undefined;
    let dirty = true;
    // Cache px terakhir untuk hit-test klik (inspect rider/order).
    let pickData: {
      riders: Map<number, [number, number]>;
      orders: { id: string; x: number; y: number }[];
    } | null = null;

    const resize = () => {
      dpr = Math.min(2, window.devicePixelRatio || 1);
      canvas.width = Math.round(container.clientWidth * dpr);
      canvas.height = Math.round(container.clientHeight * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      dirty = true;
    };
    resize();
    ro = new ResizeObserver(resize);
    ro.observe(container);

    (async () => {
      const maplibregl = await import('maplibre-gl');
      if (cancelled) return;
      map = new maplibregl.Map({
        container,
        style: buildStyle(),
        center: CENTER,
        zoom: BASE_ZOOM,
        minZoom: 10.8,
        maxZoom: 17,
        maxBounds: BBOX,
        attributionControl: false,
        dragRotate: false,
        pitchWithRotate: false,
        fadeDuration: 0,
      });
      map.addControl(
        new maplibregl.AttributionControl({
          compact: true,
          customAttribution: '© OpenStreetMap contributors',
        }),
        'bottom-right',
      );
      map.touchZoomRotate.disableRotation();

      const lerp = (a: number, b: number, f: number) => a + (b - a) * f;

      map.on('move', () => {
        dirty = true;
      });

      // Inspect (Fase 5): klik rider/order pada frame aktif — hanya saat
      // override replay memasang handler (mode live dibiarkan untuk drag peta).
      map.on('click', (e) => {
        const cb = streamRef.current.onMapClick;
        if (!cb || !pickData) return;
        let bestR: { id: number; d: number } | null = null;
        for (const [id, [x, y]] of pickData.riders) {
          const d = Math.hypot(x - e.point.x, y - e.point.y);
          if (d < 16 && (!bestR || d < bestR.d)) bestR = { id, d };
        }
        if (bestR) {
          cb({ kind: 'rider', id: bestR.id });
          return;
        }
        let bestO: { id: string; d: number } | null = null;
        for (const o of pickData.orders) {
          const d = Math.hypot(o.x - e.point.x, o.y - e.point.y);
          if (d < 12 && (!bestO || d < bestO.d)) bestO = { id: o.id, d };
        }
        cb(bestO ? { kind: 'order', id: bestO.id } : null);
      });

      const riderPos = (
        prevR: RiderPt | undefined,
        nextR: RiderPt,
        alpha: number,
      ): [number, number] => {
        if (!prevR) return [nextR.la, nextR.lo];
        return [lerp(prevR.la, nextR.la, alpha), lerp(prevR.lo, nextR.lo, alpha)];
      };

      const draw = () => {
        raf = requestAnimationFrame(draw);
        if (!map || !map.isStyleLoaded()) return;
        const { getPair, reduced } = streamRef.current;
        const now = performance.now();
        const pair = getPair(now);
        const w = container.clientWidth;
        const h = container.clientHeight;
        const key = pair && typeof pair.key === 'number' ? pair.key : null;
        const selKey = streamRef.current.selected;
        if (key !== null && key === lastDrawnKey && selKey === lastSelKey && !dirty) return;
        lastDrawnKey = key;
        lastSelKey = selKey;
        dirty = false;
        ctx.clearRect(0, 0, w, h);
        if (!pair.next) {
          pickData = null;
          return;
        }

        const alpha = reduced ? 1 : pair.alpha;
        const prevIdx = new Map<number, RiderPt>();
        if (pair.prev) for (const r of pair.prev.r) prevIdx.set(r.i, r);

        const positions = new Map<number, [number, number]>();
        for (const r of pair.next.r) {
          positions.set(r.i, riderPos(prevIdx.get(r.i), r, alpha));
        }
        pickData = { riders: new Map(), orders: [] };

        // --- trailing glow (geo trail, diproyeksi tiap frame) ---
        if (!reduced) {
          for (const [id, pos] of positions) {
            let tr = trails.get(id);
            if (!tr) {
              tr = [];
              trails.set(id, tr);
            }
            const last = tr[tr.length - 1];
            if (!last || Math.abs(last[0] - pos[0]) > 2e-5 || Math.abs(last[1] - pos[1]) > 2e-5) {
              tr.push([pos[0], pos[1]]);
              if (tr.length > TRAIL_MAX) tr.shift();
            }
          }
          ctx.lineCap = 'round';
          for (const r of pair.next.r) {
            const tr = trails.get(r.i);
            if (!tr || tr.length < 2) continue;
            const color = riderStatusColor[r.s] ?? tokens.color.accentLime;
            ctx.strokeStyle = color;
            for (let i = 1; i < tr.length; i++) {
              const p0 = map.project([tr[i - 1][1], tr[i - 1][0]]);
              const p1 = map.project([tr[i][1], tr[i][0]]);
              ctx.globalAlpha = (i / tr.length) * 0.22;
              ctx.lineWidth = 2.4 * (i / tr.length);
              ctx.beginPath();
              ctx.moveTo(p0.x, p0.y);
              ctx.lineTo(p1.x, p1.y);
              ctx.stroke();
            }
          }
          ctx.globalAlpha = 1;
        } else {
          trails.clear();
        }

        // --- garis assignment dashed (rider → pickup / dropoff) ---
        const orderById = new Map<string, OrderPt>();
        for (const o of pair.next.o) orderById.set(o.i, o);
        ctx.setLineDash([6, 6]);
        ctx.lineWidth = 1.5;
        for (const link of pair.next.l) {
          const o = orderById.get(link.o);
          const pos = positions.get(link.r);
          if (!o || !pos) continue;
          const target =
            o.s === ORDER_STATUS.inTransit ? [o.da, o.do] : [o.pa, o.po];
          const tp = map.project([target[1], target[0]]);
          const rp = map.project([pos[1], pos[0]]);
          const transit = o.s === ORDER_STATUS.inTransit;
          ctx.strokeStyle = transit ? tokens.color.statusViolet : tokens.color.statusAmber;
          ctx.globalAlpha = 0.5;
          if (!reduced) ctx.lineDashOffset = -(now / 40) % 12;
          ctx.beginPath();
          ctx.moveTo(rp.x, rp.y);
          ctx.lineTo(tp.x, tp.y);
          ctx.stroke();
        }
        ctx.setLineDash([]);
        ctx.globalAlpha = 1;

        // --- order: pulsa radar saat masuk + titik pickup/dropoff ---
        for (const o of pair.next.o) {
          const waiting = o.s !== ORDER_STATUS.inTransit;
          if (waiting) {
            const p = map.project([o.po, o.pa]);
            pickData.orders.push({ id: o.i, x: p.x, y: p.y });
            if (!pulseSeen.has(o.i)) {
              pulseSeen.set(o.i, now);
              if (pulseSeen.size > 900) pulseSeen.clear();
            }
            const started = pulseSeen.get(o.i) ?? now;
            if (!reduced) {
              const age = now - started;
              if (age < PULSE_MS * 2) {
                for (const offset of [0, PULSE_MS * 0.35]) {
                  const t = (age - offset) / PULSE_MS;
                  if (t < 0 || t > 1) continue;
                  ctx.globalAlpha = 0.55 * (1 - t);
                  ctx.strokeStyle = tokens.color.accentCyan;
                  ctx.lineWidth = 1.6;
                  ctx.beginPath();
                  ctx.arc(p.x, p.y, 4 + t * 16, 0, Math.PI * 2);
                  ctx.stroke();
                }
              }
            }
            ctx.globalAlpha = 0.95;
            ctx.fillStyle =
              o.s === ORDER_STATUS.assigned ? tokens.color.statusAmber : tokens.color.accentCyan;
            ctx.beginPath();
            ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
            ctx.fill();
          } else {
            const p = map.project([o.do, o.da]);
            pickData.orders.push({ id: o.i, x: p.x, y: p.y });
            ctx.globalAlpha = 0.5;
            ctx.fillStyle = tokens.color.statusViolet;
            ctx.beginPath();
            ctx.arc(p.x, p.y, 2.4, 0, Math.PI * 2);
            ctx.fill();
          }
        }
        ctx.globalAlpha = 1;

        // --- rider: glow sprite + titik status ---
        ctx.globalCompositeOperation = 'lighter';
        for (const r of pair.next.r) {
          const pos = positions.get(r.i);
          if (!pos) continue;
          const p = map.project([pos[1], pos[0]]);
          pickData.riders.set(r.i, [p.x, p.y]);
          const color = riderStatusColor[r.s] ?? tokens.color.accentLime;
          const glow = glows[r.s] ?? glows[0];
          ctx.drawImage(glow, p.x - 13, p.y - 13, 26, 26);
          ctx.globalCompositeOperation = 'source-over';
          ctx.fillStyle = color;
          ctx.beginPath();
          ctx.arc(p.x, p.y, 3.4, 0, Math.PI * 2);
          ctx.fill();
          ctx.globalCompositeOperation = 'lighter';
        }
        ctx.globalCompositeOperation = 'source-over';

        // --- cincin highlight entri terpilih (inspect, Fase 5) ---
        const sel = streamRef.current.selected;
        if (sel) {
          let sx = 0;
          let sy = 0;
          let found = false;
          if (sel.kind === 'rider') {
            const pos = positions.get(sel.id as number);
            if (pos) {
              const p = map.project([pos[1], pos[0]]);
              sx = p.x;
              sy = p.y;
              found = true;
            }
          } else {
            const so = orderById.get(sel.id as string);
            if (so) {
              const target =
                so.s === ORDER_STATUS.inTransit ? [so.da, so.do] : [so.pa, so.po];
              const p = map.project([target[1], target[0]]);
              sx = p.x;
              sy = p.y;
              found = true;
            }
          }
          if (found) {
            ctx.strokeStyle = tokens.color.statusCoral;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(sx, sy, 9, 0, Math.PI * 2);
            ctx.stroke();
          }
        }

        // Hook deterministik untuk verifikasi E2E (hanya saat override
        // replay aktif — klik inspect): posisi px rider/order terakhir.
        if (streamRef.current.onMapClick) {
          (window as unknown as { __lmPick?: unknown }).__lmPick = {
            riders: Array.from(pickData.riders.entries(), ([id, [x, y]]) => ({ id, x, y })).slice(0, 80),
            orders: pickData.orders.slice(0, 80),
          };
        }
      };

      map.on('load', () => {
        if (cancelled) return;
        resize();
        raf = requestAnimationFrame(draw);
      });
    })();

    return () => {
      cancelled = true;
      cancelAnimationFrame(raf);
      ro?.disconnect();
      trails.clear();
      pulseSeen.clear();
      canvas.remove();
      map?.remove();
      map = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      ref={containerRef}
      className="absolute inset-0"
      // Inline (bukan utility): CSS maplibre (.maplibregl-map) mendefinisikan
      // position: relative yang bisa menggeser urutan cascade di atas utility
      // Tailwind — tanpa ini tinggi kontainer 0 dan peta tidak tergambar.
      style={{ position: 'absolute' }}
      aria-label="Live Ops Map — Berlin"
      title="Live Berlin ops: dots = riders (idle lime / to-pickup amber / pickup cyan / delivering violet), pulsing cyan = waiting orders, dashed line = assignment"
      role="img"
    />
  );
}

// memo: props stabil (ref) → subtree ini tidak ikut re-render saat stats tick.
export default memo(LiveMapImpl);
