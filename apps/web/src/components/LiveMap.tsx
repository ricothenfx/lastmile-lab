'use client';

import { memo, useEffect, useRef, useState } from 'react';
import type { Map as MLMap, StyleSpecification } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { riderStatusColor, tokens } from '@/lib/tokens';
import { fmtDist, haversineM } from '@/lib/geo';
import {
  ORDER_STATUS,
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

// Fase 8 — zona panas (heatmap densitas order yang mengikuti surge).
const HEAT_CELL_DEG = 0.005; // ±500 m
const HEAT_MIN_ORDERS = 2; // sel dengan < 2 order tidak digambar (tidak ada panas)
const HEAT_PERIOD_MS = 4000; // periode napas

// Fase 8 — delivery burst (sukses) & expiry fade (gagal).
const BURST_LIFE_MS = 700;
const EXPIRE_LIFE_MS = 450;
const BURST_MAX = 64;
// Lompatan waktu (scrub mundur/maju besar) → reset tanpa spawn burst.
const BURST_GAP_MS = 8000;

const RIDER_LABEL = ['IDLE', 'TO PICKUP', 'PICKUP', 'DELIVERING'] as const;
const RIDER_CLS = [
  'text-accent-lime',
  'text-status-amber',
  'text-accent-cyan',
  'text-status-violet',
] as const;
const ORDER_LABEL = ['WAITING', 'ASSIGNED', 'IN TRANSIT'] as const;
const ORDER_CLS = ['text-accent-cyan', 'text-status-amber', 'text-status-violet'] as const;

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

/** Sprite radial lebar untuk blob zona panas (fase 8). */
function makeHeatSprite(color: string): HTMLCanvasElement {
  const S = 128;
  const c = document.createElement('canvas');
  c.width = S;
  c.height = S;
  const g = c.getContext('2d')!;
  const grad = g.createRadialGradient(S / 2, S / 2, 2, S / 2, S / 2, S / 2);
  grad.addColorStop(0, color + 'cc');
  grad.addColorStop(0.45, color + '44');
  grad.addColorStop(1, color + '00');
  g.fillStyle = grad;
  g.fillRect(0, 0, S, S);
  return c;
}

function buildStyle(): StyleSpecification {
  const t = tokens.color;
  return {
    version: 8,
    // Glyph self-hosted (fase 8): fontstack "Inter" dari public/fonts/inter/
    // — tanpa font/tile provider eksternal (ADR D12 tetap utuh).
    glyphs: '/fonts/{fontstack}/{range}.pbf',
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
        // Glow jalan major saat zoom (DESIGN.md token map.road "glow halus").
        id: 'roads-glow',
        type: 'line',
        source: 'roads',
        minzoom: 11,
        filter: ['==', ['get', 'c'], 1],
        paint: {
          'line-color': t.mapRoadGlow,
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 3, 15, 10, 17, 16],
          'line-opacity': ['interpolate', ['linear'], ['zoom'], 13, 0, 13.8, 0.16, 15.5, 0.3],
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
      {
        // Nama jalan major — muncul mulai z13 (fase 8; data `n` dari OSM).
        id: 'roads-label-major',
        type: 'symbol',
        source: 'roads',
        minzoom: 13,
        filter: ['all', ['has', 'n'], ['==', ['get', 'c'], 1]],
        layout: {
          'symbol-placement': 'line',
          'symbol-sort-key': 0,
          'text-field': ['get', 'n'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 13, 10, 17, 14.5],
          'text-letter-spacing': 0.05,
          'text-max-width': 8,
        },
        paint: {
          'text-color': t.mapLabelMajor,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.3,
          'text-opacity': ['interpolate', ['linear'], ['zoom'], 13, 0, 13.7, 1],
        },
      },
      {
        // Nama jalan minor — muncul mulai z14, prioritas di bawah major.
        id: 'roads-label-minor',
        type: 'symbol',
        source: 'roads',
        minzoom: 14,
        filter: ['all', ['has', 'n'], ['==', ['get', 'c'], 0]],
        layout: {
          'symbol-placement': 'line',
          'symbol-sort-key': 1,
          'text-field': ['get', 'n'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 14, 8.5, 17, 12],
          'text-letter-spacing': 0.03,
          'text-max-width': 8,
        },
        paint: {
          'text-color': t.mapLabel,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.1,
          'text-opacity': ['interpolate', ['linear'], ['zoom'], 14, 0, 14.7, 1],
        },
      },
    ],
  };
}

/** Entitas di bawah pointer (hover). */
interface HoverState {
  kind: 'rider' | 'order';
  id: number | string;
  x: number;
  y: number;
}

interface Burst {
  lat: number;
  lon: number;
  start: number;
  kind: 'delivered' | 'expired';
}

/** Turunan data kartu inspect live — dipanggil saat render kartu saja. */
function deriveInspect(
  pair: FramePair | null,
  pick: MapPick | null,
  firstSeen: Map<string, number>,
) {
  if (!pair?.next || !pick) return null;
  const snap: Snapshot = pair.next;
  if (pick.kind === 'rider') {
    const r = snap.r.find((x: RiderPt) => x.i === pick.id);
    if (!r) return null;
    const link = snap.l.find((l) => l.r === r.i);
    const ord = link ? snap.o.find((o: OrderPt) => o.i === link.o) ?? null : null;
    const target = ord
      ? ord.s === ORDER_STATUS.inTransit
        ? [ord.da, ord.do]
        : [ord.pa, ord.po]
      : null;
    return {
      kind: 'rider' as const,
      rider: r,
      order: ord,
      distM: target ? haversineM(r.la, r.lo, target[0], target[1]) : null,
      orderAgeS:
        ord && firstSeen.has(ord.i) ? Math.max(0, Math.round((snap.t - firstSeen.get(ord.i)!) / 1000)) : null,
    };
  }
  const o = snap.o.find((x: OrderPt) => x.i === pick.id);
  if (!o) return null;
  const link = snap.l.find((l) => l.o === o.i);
  const rider = link ? snap.r.find((r: RiderPt) => r.i === link.r) ?? null : null;
  return {
    kind: 'order' as const,
    order: o,
    riderId: rider?.i ?? null,
    routeM: haversineM(o.pa, o.po, o.da, o.do),
    ageS: firstSeen.has(o.i) ? Math.max(0, Math.round((snap.t - firstSeen.get(o.i)!) / 1000)) : null,
  };
}

function fmtAge(s: number | null): string {
  if (s === null) return '—';
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

function LiveMapImpl({ streamRef }: { streamRef: React.MutableRefObject<LiveStreamRef> }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const tooltipRef = useRef<HTMLDivElement>(null);
  const hoverRef = useRef<HoverState | null>(null);
  const liveSelRef = useRef<MapPick | null>(null);
  const hasReplayCbRef = useRef(false);
  const firstSeenRef = useRef<Map<string, number>>(new Map());
  // State hanya berubah saat identitas entitas berubah — bukan per gerakan mouse.
  const [hover, setHover] = useState<HoverState | null>(null);
  const [liveSel, setLiveSel] = useState<MapPick | null>(null);
  const [replayActive, setReplayActive] = useState(false);
  const [cardTick, setCardTick] = useState(0);

  // Kartu live terbuka → tick 1 Hz agar data (umur order, jarak) tetap segar.
  useEffect(() => {
    if (!liveSel) return;
    const iv = setInterval(() => setCardTick((v) => v + 1), 1000);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        liveSelRef.current = null;
        setLiveSel(null);
        streamRef.current.selected = null;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => {
      clearInterval(iv);
      window.removeEventListener('keydown', onKey);
    };
  }, [liveSel, streamRef]);

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
    const heatAmber = makeHeatSprite(tokens.color.statusAmber);
    const heatCoral = makeHeatSprite(tokens.color.statusCoral);
    const trails = new Map<number, number[][]>();
    const pulseSeen = new Map<string, number>();
    // Fase 8 — state antar-frame untuk burst & zona panas.
    const knownOrders = new Map<string, { s: number; lat: number; lon: number }>();
    const bursts: Burst[] = [];
    let burstDelivered = 0;
    let burstExpired = 0;
    let lastFrameT: number | null = null;
    let dpr = 1;

    // Guard render on-demand (Fase 5): frame replay statis (pause/scrub)
    // hanya digambar ulang saat key frame berubah ATAU peta/viewport
    // berubah — tanpa kerja per-rAF saat diam, tanpa rAF baru.
    let lastDrawnKey: number | null = null;
    let lastSelKey: MapPick | null | undefined = undefined;
    let dirty = true;
    // Cache px terakhir untuk hit-test klik/hover (inspect rider/order).
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

    /** Posisi tooltip mengikuti pointer + membalik dekat tepi kanan/bawah. */
    const placeTooltip = (x: number, y: number) => {
      const el = tooltipRef.current;
      if (!el) return;
      const w = container.clientWidth;
      const h = container.clientHeight;
      const flipX = x > w - 240;
      const flipY = y > h - 120;
      el.style.transform = `translate(${flipX ? x - 14 : x + 14}px, ${flipY ? y - 14 : y + 14}px) translate(${flipX ? '-100%' : '0'}, ${flipY ? '-100%' : '0'})`;
    };

    const setHoverState = (h: HoverState | null) => {
      const prev = hoverRef.current;
      if (prev?.kind === h?.kind && prev?.id === h?.id) return; // identitas sama
      hoverRef.current = h;
      setHover(h);
    };

    const clearHover = () => {
      setHoverState(null);
      if (map) map.getCanvas().style.cursor = '';
    };

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
      // Hook verifikasi E2E: instansi peta untuk probe style/label headless.
      (window as unknown as { __lmMap?: unknown }).__lmMap = map;

      const lerp = (a: number, b: number, f: number) => a + (b - a) * f;

      map.on('move', () => {
        dirty = true;
      });

      // Hover (fase 8): hit-test entitas di bawah pointer. Data posisi px
      // (pickData) sudah dihitung tiap frame — hit-test O(n) kecil, tanpa
      // kerja render tambahan. Tooltip mengikuti pointer via DOM langsung.
      map.on('mousemove', (e) => {
        const t0 = performance.now();
        if (!pickData) {
          clearHover();
          return;
        }
        let hit: HoverState | null = null;
        let bestR: { id: number; d: number } | null = null;
        for (const [id, [x, y]] of pickData.riders) {
          const d = Math.hypot(x - e.point.x, y - e.point.y);
          if (d < 16 && (!bestR || d < bestR.d)) bestR = { id, d };
        }
        if (bestR) {
          hit = { kind: 'rider', id: bestR.id, x: e.point.x, y: e.point.y };
        } else {
          let bestO: { id: string; d: number } | null = null;
          for (const o of pickData.orders) {
            const d = Math.hypot(o.x - e.point.x, o.y - e.point.y);
            if (d < 12 && (!bestO || d < bestO.d)) bestO = { id: o.id, d };
          }
          if (bestO) hit = { kind: 'order', id: bestO.id, x: e.point.x, y: e.point.y };
        }
        (window as unknown as { __lmHoverMs?: number }).__lmHoverMs = performance.now() - t0;
        setHoverState(hit);
        if (hit) {
          placeTooltip(e.point.x, e.point.y);
          map!.getCanvas().style.cursor = 'pointer';
        } else {
          map!.getCanvas().style.cursor = '';
        }
      });
      map.on('mouseout', clearHover);
      map.on('dragstart', clearHover);

      // Inspect: klik rider/order. Saat override replay memasang handler,
      // klik diserahkan ke replay (kartu reason Fase 5); selain itu kartu
      // inspect LIVE milik LiveMap ini (fase 8).
      map.on('click', (e) => {
        if (!pickData) return;
        let bestR: { id: number; d: number } | null = null;
        for (const [id, [x, y]] of pickData.riders) {
          const d = Math.hypot(x - e.point.x, y - e.point.y);
          if (d < 16 && (!bestR || d < bestR.d)) bestR = { id, d };
        }
        let pick: MapPick | null = null;
        if (bestR) {
          pick = { kind: 'rider', id: bestR.id };
        } else {
          let bestO: { id: string; d: number } | null = null;
          for (const o of pickData.orders) {
            const d = Math.hypot(o.x - e.point.x, o.y - e.point.y);
            if (d < 12 && (!bestO || d < bestO.d)) bestO = { id: o.id, d };
          }
          pick = bestO ? { kind: 'order', id: bestO.id } : null;
        }
        const cb = streamRef.current.onMapClick;
        if (cb) {
          cb(pick);
          return;
        }
        liveSelRef.current = pick;
        setLiveSel(pick);
        streamRef.current.selected = pick;
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

        // --- zona panas (fase 8): densitas order aktif per grid ±500 m ---
        // Intensitas mengikuti surge nyata (st.su); bernapas pelan; reduced-
        // motion = statis. Digambar sebelum entitas (di bawah semua dot).
        const su = pair.next.st?.su ?? 1;
        {
          const surgeT = Math.min(1, Math.max(0, (su - 1) / 9));
          const cells = new Map<string, { n: number; lat: number; lon: number }>();
          for (const o of pair.next.o) {
            const lat = o.s === ORDER_STATUS.inTransit ? o.da : o.pa;
            const lon = o.s === ORDER_STATUS.inTransit ? o.do : o.po;
            const k = `${Math.floor(lat / HEAT_CELL_DEG)}:${Math.floor(lon / HEAT_CELL_DEG)}`;
            const c = cells.get(k);
            if (c) c.n += 1;
            else cells.set(k, { n: 1, lat, lon });
          }
          let maxN = 0;
          let alphaSum = 0;
          {
            const breath =
              reduced
                ? 1
                : 1 + 0.16 * Math.sin((now * 2 * Math.PI) / HEAT_PERIOD_MS) * Math.min(1, surgeT * 3);
            ctx.globalCompositeOperation = 'lighter';
            for (const c of cells.values()) {
              if (c.n < HEAT_MIN_ORDERS) continue;
              if (c.n > maxN) maxN = c.n;
              const p = map.project([c.lon, c.lat]);
              if (p.x < -120 || p.y < -120 || p.x > w + 120 || p.y > h + 120) continue;
              const base = 0.05 + 0.3 * surgeT;
              const boost = Math.min(0.14, (c.n - HEAT_MIN_ORDERS) * 0.015);
              const a = (base + boost) * breath;
              alphaSum += a;
              const size = 90 + Math.min(40, c.n * 4);
              ctx.globalAlpha = a;
              ctx.drawImage(heatAmber, p.x - size / 2, p.y - size / 2, size, size);
              const mix = Math.min(1, surgeT * 1.6);
              if (mix > 0) {
                ctx.globalAlpha = a * mix;
                ctx.drawImage(heatCoral, p.x - size / 2, p.y - size / 2, size, size);
              }
            }
            ctx.globalCompositeOperation = 'source-over';
            ctx.globalAlpha = 1;
          }
          (window as unknown as { __lmHeat?: unknown }).__lmHeat = {
            cells: Array.from(cells.values()).filter((c) => c.n >= HEAT_MIN_ORDERS).length,
            max: maxN,
            total: pair.next.o.length,
            su,
            reduced,
            alphaSum: Math.round(alphaSum * 1000) / 1000,
          };
        }

        // --- delivery burst / expiry fade (fase 8) ---
        // Order in-transit yang menghilang = terkirim (burst sukses);
        // waiting/assigned yang menghilang = expired (fade coral singkat).
        {
          const t = pair.next.t;
          if (lastFrameT !== null && (t < lastFrameT || t - lastFrameT > BURST_GAP_MS)) {
            knownOrders.clear();
            bursts.length = 0;
          }
          lastFrameT = t;
          const seen = new Set<string>();
          for (const o of pair.next.o) {
            seen.add(o.i);
            knownOrders.set(o.i, {
              s: o.s,
              lat: o.s === ORDER_STATUS.inTransit ? o.da : o.pa,
              lon: o.s === ORDER_STATUS.inTransit ? o.do : o.po,
            });
          }
          for (const [id, k] of knownOrders) {
            if (seen.has(id)) continue;
            if (!reduced && bursts.length < BURST_MAX) {
              if (k.s === ORDER_STATUS.inTransit) {
                bursts.push({ lat: k.lat, lon: k.lon, start: now, kind: 'delivered' });
                burstDelivered += 1;
              } else {
                bursts.push({ lat: k.lat, lon: k.lon, start: now, kind: 'expired' });
                burstExpired += 1;
              }
            }
            knownOrders.delete(id);
          }
          for (let i = bursts.length - 1; i >= 0; i--) {
            const b = bursts[i];
            const life = b.kind === 'delivered' ? BURST_LIFE_MS : EXPIRE_LIFE_MS;
            const age = now - b.start;
            if (age > life) {
              bursts.splice(i, 1);
              continue;
            }
            const f = age / life;
            const p = map.project([b.lon, b.lat]);
            if (b.kind === 'delivered') {
              ctx.strokeStyle = tokens.color.statusViolet;
              ctx.globalAlpha = (1 - f) * 0.75;
              ctx.lineWidth = 2;
              ctx.beginPath();
              ctx.arc(p.x, p.y, 4 + f * 16, 0, Math.PI * 2);
              ctx.stroke();
              ctx.strokeStyle = tokens.color.accentCyan;
              const f2 = Math.max(0, f - 0.18) / 0.82;
              ctx.globalAlpha = (1 - f2) * 0.5;
              ctx.beginPath();
              ctx.arc(p.x, p.y, 4 + f2 * 11, 0, Math.PI * 2);
              ctx.stroke();
              ctx.fillStyle = tokens.color.accentCyan;
              ctx.globalAlpha = (1 - f) * 0.9;
              ctx.beginPath();
              ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
              ctx.fill();
            } else {
              ctx.strokeStyle = tokens.color.statusCoral;
              ctx.globalAlpha = (1 - f) * 0.4;
              ctx.lineWidth = 1.6;
              ctx.beginPath();
              ctx.arc(p.x, p.y, 4 + f * 8, 0, Math.PI * 2);
              ctx.stroke();
            }
          }
          ctx.globalAlpha = 1;
          (window as unknown as { __lmBursts?: unknown }).__lmBursts = {
            delivered: burstDelivered,
            expired: burstExpired,
            active: bursts.length,
          };
        }

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

        // --- cincin highlight entri terpilih (inspect) ---
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

        // --- cincin hover (fase 8) — feedback entitas di bawah pointer ---
        const hov = hoverRef.current;
        if (hov) {
          let hx = 0;
          let hy = 0;
          let hFound = false;
          if (hov.kind === 'rider') {
            const pos = positions.get(hov.id as number);
            if (pos) {
              const p = map.project([pos[1], pos[0]]);
              hx = p.x;
              hy = p.y;
              hFound = true;
            }
          } else {
            const ho = orderById.get(hov.id as string);
            if (ho) {
              const target =
                ho.s === ORDER_STATUS.inTransit ? [ho.da, ho.do] : [ho.pa, ho.po];
              const p = map.project([target[1], target[0]]);
              hx = p.x;
              hy = p.y;
              hFound = true;
            }
          }
          if (hFound) {
            ctx.strokeStyle = tokens.color.accentCyan;
            ctx.lineWidth = 1.5;
            ctx.beginPath();
            ctx.arc(hx, hy, 7.5, 0, Math.PI * 2);
            ctx.stroke();
          }
        }

        // --- indeks umur order (untuk kartu inspect live & replay) ---
        if (firstSeenRef.current.size > 2000) firstSeenRef.current.clear();
        for (const o of pair.next.o) {
          if (!firstSeenRef.current.has(o.i)) firstSeenRef.current.set(o.i, pair.next.t);
        }

        // --- sinkronisasi kartu live vs panel replay (fase 8) ---
        // Replay panel memasang onMapClick saat aktif; kartu live disembunyikan
        // dan seleksi live dilepas agar tidak dobel dengan kartu reason.
        const cb = !!streamRef.current.onMapClick;
        if (cb !== hasReplayCbRef.current) {
          hasReplayCbRef.current = cb;
          setReplayActive(cb);
          if (cb) {
            liveSelRef.current = null;
            setLiveSel(null);
            streamRef.current.selected = null;
          }
        }

        // Hook deterministik untuk verifikasi E2E: posisi px rider/order
        // terakhir (hit-test hover/klik diuji headless).
        (window as unknown as { __lmPick?: unknown }).__lmPick = {
          riders: Array.from(pickData.riders.entries(), ([id, [x, y]]) => ({ id, x, y })).slice(0, 80),
          orders: pickData.orders.slice(0, 80),
        };
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
      knownOrders.clear();
      bursts.length = 0;
      firstSeenRef.current.clear();
      canvas.remove();
      map?.remove();
      map = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- tooltip hover (sibling peta, posisi via transform — tanpa re-render
  // per gerakan mouse; konten hanya berubah saat identitas entitas berganti).
  const hoverData = hover
    ? deriveInspect(streamRef.current.getPair(performance.now()), { kind: hover.kind, id: hover.id }, firstSeenRef.current)
    : null;

  return (
    <>
      <div
        ref={containerRef}
        className="absolute inset-0"
        // Inline (bukan utility): CSS maplibre (.maplibregl-map) mendefinisikan
        // position: relative yang bisa menggeser urutan cascade di atas utility
        // Tailwind — tanpa ini tinggi kontainer 0 dan peta tidak tergambar.
        style={{ position: 'absolute' }}
        aria-label="Live Ops Map — Berlin"
        title="Live Berlin ops: dots = riders (hover or click one for detail), street names appear when you zoom in, breathing glow = order density following surge"
        role="img"
      />
      {hover && (
        <div
          ref={tooltipRef}
          data-testid="map-tooltip"
          aria-hidden
          className="pointer-events-none absolute left-0 top-0 z-[2] w-max max-w-[230px] rounded-card border border-line-subtle bg-surface-overlay/95 px-2.5 py-2 shadow-lg backdrop-blur"
          style={{ transform: `translate(${hover.x + 14}px, ${hover.y + 14}px)` }}
        >
          {hoverData ? (
            hoverData.kind === 'rider' ? (
              <RiderTip d={hoverData} />
            ) : (
              <OrderTip d={hoverData} />
            )
          ) : (
            <span className="font-mono text-[10px] text-ink-muted">OFF FEED</span>
          )}
        </div>
      )}
      {liveSel && !replayActive && (
        <LiveInspectCard
          data={deriveInspect(streamRef.current.getPair(performance.now()), liveSel, firstSeenRef.current)}
          entityLabel={liveSel.kind === 'rider' ? `rider r${liveSel.id}` : `order ${liveSel.id}`}
          onClose={() => {
            liveSelRef.current = null;
            setLiveSel(null);
            streamRef.current.selected = null;
          }}
        />
      )}
    </>
  );
}

function RiderTip({ d }: { d: Exclude<NonNullable<ReturnType<typeof deriveInspect>>, { kind: 'order' }> }) {
  return (
    <>
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-mono text-[11px] font-semibold text-ink-primary">RIDER r{d.rider.i}</span>
        <span className={`font-mono text-[9px] tracking-[0.08em] ${RIDER_CLS[d.rider.s] ?? 'text-ink-secondary'}`}>
          {RIDER_LABEL[d.rider.s] ?? 'UNKNOWN'}
        </span>
      </div>
      {d.order ? (
        <p className="mt-0.5 font-mono text-[9px] leading-relaxed text-ink-secondary">
          ORDER <span className="text-ink-primary">{d.order.i}</span> ·{' '}
          <span className={ORDER_CLS[d.order.s] ?? 'text-ink-secondary'}>
            {ORDER_LABEL[d.order.s] ?? '—'}
          </span>
          {d.distM !== null && (
            <>
              {' · '}
              <span className="text-ink-primary">{fmtDist(d.distM)}</span> to{' '}
              {d.order.s === ORDER_STATUS.inTransit ? 'dropoff' : 'pickup'}
            </>
          )}
        </p>
      ) : (
        <p className="mt-0.5 font-mono text-[9px] text-ink-secondary">Idle — no order assigned.</p>
      )}
    </>
  );
}

function OrderTip({ d }: { d: Exclude<NonNullable<ReturnType<typeof deriveInspect>>, { kind: 'rider' }> }) {
  return (
    <>
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-mono text-[11px] font-semibold text-ink-primary">ORDER {d.order.i}</span>
        <span className={`font-mono text-[9px] tracking-[0.08em] ${ORDER_CLS[d.order.s] ?? 'text-ink-secondary'}`}>
          {ORDER_LABEL[d.order.s] ?? '—'}
        </span>
      </div>
      <p className="mt-0.5 font-mono text-[9px] leading-relaxed text-ink-secondary">
        {d.riderId !== null ? (
          <>
            RIDER <span className="text-ink-primary">r{d.riderId}</span> ·{' '}
          </>
        ) : (
          'UNASSIGNED · '
        )}
        ROUTE <span className="text-ink-primary">{fmtDist(d.routeM)}</span> · AGE{' '}
        <span className="text-ink-primary">{fmtAge(d.ageS)}</span>
      </p>
    </>
  );
}

/** Kartu inspect live (klik dot saat mode live) — fase 8. */
function LiveInspectCard({
  data,
  entityLabel,
  onClose,
}: {
  data: ReturnType<typeof deriveInspect>;
  entityLabel: string;
  onClose: () => void;
}) {
  return (
    <div
      data-testid="live-inspect"
      role="region"
      aria-label="Live map inspect"
      className="absolute bottom-4 left-4 z-[2] w-[262px] rounded-panel border border-line-subtle bg-surface-raised/95 p-3 backdrop-blur"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-mono text-[12px] font-semibold uppercase text-ink-primary">
          {data ? (data.kind === 'rider' ? `RIDER r${data.rider.i}` : `ORDER ${data.order.i}`) : entityLabel}
        </span>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close live inspect card"
          className="font-mono text-[11px] text-ink-muted transition-colors duration-fast hover:text-ink-primary"
        >
          ✕
        </button>
      </div>
      {!data ? (
        <p className="mt-1.5 font-mono text-[10px] text-ink-secondary">
          Entity left the live feed (delivered or expired).
        </p>
      ) : data.kind === 'rider' ? (
        <div className="mt-1.5 grid gap-x-4 gap-y-0.5 font-mono text-[10px] tabular-nums text-ink-secondary sm:grid-cols-1">
          <span>
            STATUS{' '}
            <span className={`${RIDER_CLS[data.rider.s] ?? 'text-ink-secondary'}`}>
              {RIDER_LABEL[data.rider.s] ?? 'UNKNOWN'}
            </span>
          </span>
          {data.order ? (
            <>
              <span>
                ORDER <span className="text-ink-primary">{data.order.i}</span> ·{' '}
                <span className={ORDER_CLS[data.order.s] ?? 'text-ink-secondary'}>
                  {ORDER_LABEL[data.order.s] ?? '—'}
                </span>
              </span>
              <span>
                AGE <span className="text-ink-primary">{fmtAge(data.orderAgeS)}</span> (since first seen)
              </span>
              <span>
                RIDER → <span className="text-ink-primary">{fmtDist(data.distM ?? NaN)}</span> to{' '}
                {data.order.s === ORDER_STATUS.inTransit ? 'dropoff' : 'pickup'}
              </span>
              <span>
                PICKUP {data.order.pa.toFixed(4)}, {data.order.po.toFixed(4)}
              </span>
              <span>
                DROPOFF {data.order.da.toFixed(4)}, {data.order['do'].toFixed(4)}
              </span>
            </>
          ) : (
            <span>
              Idle — no order assigned. Live feed carries no decision ring; open Replay &amp; Inspect for
              dispatch reasons.
            </span>
          )}
        </div>
      ) : (
        <div className="mt-1.5 grid gap-x-4 gap-y-0.5 font-mono text-[10px] tabular-nums text-ink-secondary sm:grid-cols-1">
          <span>
            STATUS{' '}
            <span className={`${ORDER_CLS[data.order.s] ?? 'text-ink-secondary'}`}>
              {ORDER_LABEL[data.order.s] ?? '—'}
            </span>
          </span>
          <span>
            RIDER{' '}
            <span className="text-ink-primary">
              {data.riderId !== null ? `r${data.riderId}` : '— (unassigned)'}
            </span>
          </span>
          <span>
            AGE <span className="text-ink-primary">{fmtAge(data.ageS)}</span> (since first seen)
          </span>
          <span>
            ROUTE <span className="text-ink-primary">{fmtDist(data.routeM)}</span> (pickup → dropoff)
          </span>
          <span>
            PICKUP {data.order.pa.toFixed(4)}, {data.order.po.toFixed(4)}
          </span>
          <span>
            DROPOFF {data.order.da.toFixed(4)}, {data.order['do'].toFixed(4)}
          </span>
        </div>
      )}
    </div>
  );
}

// memo: props stabil (ref) → subtree ini tidak ikut re-render saat stats tick.
export default memo(LiveMapImpl);
