'use client';

import { memo, useEffect, useRef, useState } from 'react';
import type { Map as MLMap, StyleSpecification } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { riderStatusColor, tokens, type Palette } from '@/lib/tokens';
import { isLightTheme, subscribeTheme } from '@/lib/theme';
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

// Fase 10 — bahasa ikon (ADR D26): satu jenis entitas = satu bentuk.
// Rider = motor (menghadap arah gerak), order waiting/assigned = ikon restoran
// di titik pickup, order in-transit = rumah di titik dropoff.
const SPRITE_SCALE = 2;
const MOTOR_PX_ACTIVE = 23;
const MOTOR_PX_IDLE = 19;
const RESTO_PX = 16;
const HOUSE_PX = 15;
// Garis assignment penuh hanya saat user benar-benar zoom-in. Catatan:
// maxBounds meng-clamp zoom MINIMUM ke ~13.4 pada viewport lebar (bbox harus
// memenuhi layar) — jadi threshold harus di ATAS clamp, bukan di sekitarnya.
const LINE_ZOOM_MIN = 14.5;

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

/** Kanvas sprite tajam (dirender 2×, digambar terskala). */
function makeSprite(units: number, draw: (g: CanvasRenderingContext2D) => void): HTMLCanvasElement {
  const S = units * SPRITE_SCALE;
  const c = document.createElement('canvas');
  c.width = S;
  c.height = S;
  const g = c.getContext('2d')!;
  g.scale(SPRITE_SCALE, SPRITE_SCALE);
  g.lineCap = 'round';
  g.lineJoin = 'round';
  draw(g);
  return c;
}

/** Motor (skuter) menghadap KANAN — angle 0 = +x; dirotasi per heading rider. */
function strokeMotor(g: CanvasRenderingContext2D, color: string): void {
  g.strokeStyle = color;
  g.lineWidth = 2.6;
  g.beginPath();
  g.arc(7, 21, 3.6, 0, Math.PI * 2); // roda belakang
  g.stroke();
  g.beginPath();
  g.arc(25, 21, 3.6, 0, Math.PI * 2); // roda depan
  g.stroke();
  g.beginPath();
  g.moveTo(19.5, 7); // stang
  g.lineTo(22.5, 7);
  g.moveTo(21, 7); // kolom kemudi → garpu depan
  g.lineTo(26.5, 13);
  g.lineTo(25, 21);
  g.moveTo(4, 12.5); // jok
  g.lineTo(10.5, 12.5);
  g.moveTo(5.5, 13.5); // rangka jok → hub roda belakang
  g.lineTo(7, 21);
  g.moveTo(10.5, 12.5); // jok → dek → kolom belakang
  g.lineTo(12.5, 17);
  g.lineTo(18.5, 17);
  g.lineTo(26.5, 13);
  g.stroke();
}

/** Alat makan (garpu + pisau) — simbol restoran / titik pickup order. */
function strokeResto(g: CanvasRenderingContext2D, color: string): void {
  g.strokeStyle = color;
  g.lineWidth = 1.9;
  g.beginPath();
  g.moveTo(7.4, 4.5); // garpu: 2 tine
  g.lineTo(7.4, 9.2);
  g.moveTo(10.6, 4.5);
  g.lineTo(10.6, 9.2);
  g.moveTo(7.4, 9.2); // mangkuk garpu
  g.quadraticCurveTo(7.4, 11.8, 9, 11.8);
  g.quadraticCurveTo(10.6, 11.8, 10.6, 9.2);
  g.moveTo(9, 11.8); // gagang garpu
  g.lineTo(9, 19.5);
  g.moveTo(15, 19.5); // pisau: mata pisau melengkung
  g.lineTo(15, 4.5);
  g.moveTo(15, 4.5);
  g.bezierCurveTo(17.8, 6.3, 17.8, 10, 15.2, 11.8);
  g.stroke();
}

/** Rumah — simbol pelanggan / titik dropoff order. */
function strokeHouse(g: CanvasRenderingContext2D, color: string): void {
  g.strokeStyle = color;
  g.lineWidth = 2.1;
  g.beginPath();
  g.moveTo(4, 12.2); // atap
  g.lineTo(12, 5);
  g.lineTo(20, 12.2);
  g.moveTo(6.2, 10.6); // badan
  g.lineTo(6.2, 19.4);
  g.lineTo(17.8, 19.4);
  g.lineTo(17.8, 10.6);
  g.moveTo(10.2, 19.4); // pintu
  g.lineTo(10.2, 14.2);
  g.lineTo(13.8, 14.2);
  g.lineTo(13.8, 19.4);
  g.stroke();
}

function buildStyle(t: Palette): StyleSpecification {
  return {
    version: 8,
    // Glyph self-hosted (fase 8): fontstack "Inter" dari public/fonts/Inter/
    // — tanpa font/tile provider eksternal (ADR D12 tetap utuh).
    glyphs: '/fonts/{fontstack}/{range}.pbf',
    sources: {
      water: { type: 'geojson', data: '/berlin/water.geojson' },
      roads: { type: 'geojson', data: '/berlin/roads.geojson' },
      pois: { type: 'geojson', data: '/berlin/pois.geojson' },
      places: { type: 'geojson', data: '/berlin/places.geojson' },
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
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 0.9, 14, 1.8, 17, 3.4],
          'line-opacity': 0.78,
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
          'line-opacity': ['interpolate', ['linear'], ['zoom'], 12, 0, 13, 0.14, 15.5, 0.3],
        },
      },
      {
        id: 'roads-major',
        type: 'line',
        source: 'roads',
        filter: ['==', ['get', 'c'], 1],
        paint: {
          'line-color': t.mapRoad,
          'line-width': ['interpolate', ['linear'], ['zoom'], 11, 1.4, 14, 2.8, 17, 6],
          'line-opacity': 1,
        },
      },
      {
        // POI kuliner (konteks: kepadatan restoran) — samar, mulai z13.
        id: 'poi-resto',
        type: 'circle',
        source: 'pois',
        minzoom: 13,
        paint: {
          'circle-color': t.statusAmber,
          'circle-radius': ['interpolate', ['linear'], ['zoom'], 13, 1.4, 16, 2.6],
          'circle-opacity': ['interpolate', ['linear'], ['zoom'], 13, 0, 13.8, 0.45],
          'circle-stroke-width': 0,
        },
      },
      {
        // Label kawasan — orientasi besar di zoom rendah, mundur saat label
        // jalan mengambil alih.
        id: 'places-district',
        type: 'symbol',
        source: 'places',
        filter: ['==', ['get', 'kind'], 'district'],
        layout: {
          'symbol-sort-key': 0,
          'text-field': ['get', 'name'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 11, 11.5, 13, 14],
          'text-letter-spacing': 0.22,
          'text-max-width': 7,
        },
        paint: {
          'text-color': t.mapLabelMajor,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.4,
          'text-opacity': [
            'interpolate', ['linear'], ['zoom'], 10.9, 0, 11.5, 0.95, 13.4, 0.95, 14.2, 0,
          ],
        },
      },
      {
        id: 'places-place',
        type: 'symbol',
        source: 'places',
        filter: ['==', ['get', 'kind'], 'place'],
        layout: {
          'symbol-sort-key': 1,
          'text-field': ['get', 'name'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 11.5, 9.5, 13, 11.5],
          'text-letter-spacing': 0.12,
          'text-max-width': 7,
        },
        paint: {
          'text-color': t.mapLabel,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.2,
          'text-opacity': [
            'interpolate', ['linear'], ['zoom'], 11.4, 0, 12, 0.9, 13.2, 0.9, 14, 0,
          ],
        },
      },
      {
        // Nama jalan major — terlihat DI ZOOM DEFAULT (fade selesai z12.9;
        // sebelumnya baru mulai z13 → pengguna tak pernah melihat label).
        id: 'roads-label-major',
        type: 'symbol',
        source: 'roads',
        minzoom: 12.2,
        filter: ['all', ['has', 'n'], ['==', ['get', 'c'], 1]],
        layout: {
          'symbol-placement': 'line',
          'symbol-sort-key': 2,
          'text-field': ['get', 'n'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 12.2, 9.5, 17, 14.5],
          'text-letter-spacing': 0.05,
          'text-max-width': 8,
        },
        paint: {
          'text-color': t.mapLabelMajor,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.3,
          'text-opacity': ['interpolate', ['linear'], ['zoom'], 12.2, 0, 12.9, 1],
        },
      },
      {
        // Nama jalan minor — mulai z13.4, prioritas di bawah major.
        id: 'roads-label-minor',
        type: 'symbol',
        source: 'roads',
        minzoom: 13.4,
        filter: ['all', ['has', 'n'], ['==', ['get', 'c'], 0]],
        layout: {
          'symbol-placement': 'line',
          'symbol-sort-key': 3,
          'text-field': ['get', 'n'],
          'text-font': ['Inter'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 13.4, 8.5, 17, 12],
          'text-letter-spacing': 0.03,
          'text-max-width': 8,
        },
        paint: {
          'text-color': t.mapLabel,
          'text-halo-color': t.bgBase,
          'text-halo-width': 1.1,
          'text-opacity': ['interpolate', ['linear'], ['zoom'], 13.4, 0, 14.1, 1],
        },
      },
    ],
  };
}

/** Terapkan palet baru ke paint peta tanpa reload style (setPaintProperty). */
function applyThemeToMap(map: MLMap, t: Palette): void {
  const sets: [string, string, unknown][] = [
    ['bg', 'background-color', t.bgBase],
    ['water-fill', 'fill-color', t.mapWater],
    ['water-line', 'line-color', t.mapWater],
    ['roads-minor', 'line-color', t.mapRoad],
    ['roads-glow', 'line-color', t.mapRoadGlow],
    ['roads-major', 'line-color', t.mapRoad],
    ['poi-resto', 'circle-color', t.statusAmber],
    ['roads-label-major', 'text-color', t.mapLabelMajor],
    ['roads-label-major', 'text-halo-color', t.bgBase],
    ['roads-label-minor', 'text-color', t.mapLabel],
    ['roads-label-minor', 'text-halo-color', t.bgBase],
    ['places-district', 'text-color', t.mapLabelMajor],
    ['places-district', 'text-halo-color', t.bgBase],
    ['places-place', 'text-color', t.mapLabel],
    ['places-place', 'text-halo-color', t.bgBase],
  ];
  for (const [layer, prop, value] of sets) {
    try {
      map.setPaintProperty(layer, prop, value);
    } catch {
      /* style belum termuat — buildStyle berikutnya sudah pakai palet baru */
    }
  }
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
    let unsubTheme: (() => void) | null = null;

    const canvas = document.createElement('canvas');
    canvas.style.position = 'absolute';
    canvas.style.inset = '0';
    canvas.style.width = '100%';
    canvas.style.height = '100%';
    canvas.style.pointerEvents = 'none';
    // Wajib: maplibre meng-append canvas WebGL-nya SETELAH overlay ini
    // (sama-sama absolute) — tanpa z-index eksplisit peta menutup semua
    // dot/garis yang digambar (bug produksi 2026-10-02: buffer berisi
    // puluhan ribu piksel entitas tapi tak terlihat).
    canvas.style.zIndex = '1';
    container.appendChild(canvas);
    const ctx = canvas.getContext('2d')!;

    // --- sprite (dibangun dari palet aktif; di-rebuild saat tema berganti) ---
    let glows: HTMLCanvasElement[] = [];
    let heatAmber: HTMLCanvasElement;
    let heatCoral: HTMLCanvasElement;
    let motorSprites: HTMLCanvasElement[] = [];
    let restoSprites: HTMLCanvasElement[] = []; // [waiting=cyan, assigned=amber]
    let houseSprite: HTMLCanvasElement;
    const buildSprites = () => {
      const t = tokens.color;
      glows = riderStatusColor.map(makeGlowSprite);
      heatAmber = makeHeatSprite(t.statusAmber);
      heatCoral = makeHeatSprite(t.statusCoral);
      motorSprites = riderStatusColor.map((c) => makeSprite(32, (g) => strokeMotor(g, c)));
      restoSprites = [
        makeSprite(24, (g) => strokeResto(g, t.accentCyan)),
        makeSprite(24, (g) => strokeResto(g, t.statusAmber)),
      ];
      houseSprite = makeSprite(24, (g) => strokeHouse(g, t.statusViolet));
    };
    buildSprites();

    const trails = new Map<number, number[][]>();
    const pulseSeen = new Map<string, number>();
    // Tema aktif — dibaca via closure (bukan DOM per frame); komposit heatmap
    // & glow berbeda: 'lighter' hanya untuk latar gelap.
    let themeLight = isLightTheme();
    // Fase 8 — state antar-frame untuk burst & zona panas.
    const knownOrders = new Map<string, { s: number; lat: number; lon: number }>();
    const bursts: Burst[] = [];
    let burstDelivered = 0;
    let burstExpired = 0;
    let lastFrameT: number | null = null;
    let dpr = 1;
    // Fase 10 — heading terakhir per rider (radian, ruang layar; 0 = timur).
    const headings = new Map<number, number>();

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
        style: buildStyle(tokens.color),
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

      // Tema (ADR D26): terapkan palet baru ke paint peta + rebuild sprite.
      unsubTheme = subscribeTheme(() => {
        if (map) applyThemeToMap(map, tokens.color);
        buildSprites();
        themeLight = isLightTheme();
        dirty = true;
      });

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
        // Fase 10 — zoom mengatur densitas informasi: garis assignment hanya
        // saat zoom-in; di zoom rendah cukup heatmap + ikon.
        const zoom = map.getZoom();
        const showLines = zoom >= LINE_ZOOM_MIN;
        const focus = new Set<string>();
        if (hoverRef.current) {
          focus.add(hoverRef.current.kind === 'rider' ? `r${hoverRef.current.id}` : `o${hoverRef.current.id}`);
        }
        if (selKey) focus.add(selKey.kind === 'rider' ? `r${selKey.id}` : `o${selKey.id}`);
        let linesDrawn = 0;
        let motorCount = 0;
        let restoCount = 0;
        let houseCount = 0;

        // --- zona panas (fase 8): densitas order aktif per grid ±500 m ---
        // Intensitas mengikuti surge nyata (st.su); bernapas pelan; reduced-
        // motion = statis. Digambar sebelum entitas (di bawah semua ikon).
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
            // Light: 'multiply' (menggelapkan hangat, khas peta terang);
            // dark: 'lighter' (glow neun). Alpha diringankan di light.
            ctx.globalCompositeOperation = themeLight ? 'multiply' : 'lighter';
            const heatAlphaScale = themeLight ? 0.55 : 1;
            for (const c of cells.values()) {
              if (c.n < HEAT_MIN_ORDERS) continue;
              if (c.n > maxN) maxN = c.n;
              const p = map.project([c.lon, c.lat]);
              if (p.x < -120 || p.y < -120 || p.x > w + 120 || p.y > h + 120) continue;
              const base = 0.05 + 0.3 * surgeT;
              const boost = Math.min(0.14, (c.n - HEAT_MIN_ORDERS) * 0.015);
              const a = (base + boost) * breath * heatAlphaScale;
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

        // --- garis assignment (fase 10: declutter) ---
        // Default: HANYA untuk entitas yang di-hover/dipilih. Saat zoom-in
        // (>= LINE_ZOOM_MIN) semua garis ikut tampil.
        const orderById = new Map<string, OrderPt>();
        for (const o of pair.next.o) orderById.set(o.i, o);
        ctx.setLineDash([6, 6]);
        ctx.lineWidth = 1.5;
        for (const link of pair.next.l) {
          if (!showLines && !focus.has(`r${link.r}`) && !focus.has(`o${link.o}`)) continue;
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
          linesDrawn += 1;
        }
        ctx.setLineDash([]);
        ctx.globalAlpha = 1;

        // --- order: pulsa radar saat masuk + ikon restoran/rumah (fase 10) ---
        for (const o of pair.next.o) {
          if (o.s !== ORDER_STATUS.inTransit) {
            const p = map.project([o.po, o.pa]);
            pickData.orders.push({ id: o.i, x: p.x, y: p.y });
            // Pulsa radar hanya untuk order baru MENUNGGU (belum assigned).
            if (o.s === ORDER_STATUS.waiting && !pulseSeen.has(o.i)) {
              pulseSeen.set(o.i, now);
              if (pulseSeen.size > 900) pulseSeen.clear();
            }
            const started = pulseSeen.get(o.i);
            if (!reduced && o.s === ORDER_STATUS.waiting && started !== undefined) {
              const age = now - started;
              if (age < PULSE_MS * 2) {
                for (const offset of [0, PULSE_MS * 0.35]) {
                  const t = (age - offset) / PULSE_MS;
                  if (t < 0 || t > 1) continue;
                  ctx.globalAlpha = 0.55 * (1 - t);
                  ctx.strokeStyle = tokens.color.accentCyan;
                  ctx.lineWidth = 1.6;
                  ctx.beginPath();
                  ctx.arc(p.x, p.y, 5 + t * 16, 0, Math.PI * 2);
                  ctx.stroke();
                }
              }
            }
            // Ikon restoran: cyan = menunggu rider, amber = sudah assigned.
            const sprite = restoSprites[o.s === ORDER_STATUS.assigned ? 1 : 0];
            ctx.globalAlpha = 0.96;
            ctx.drawImage(sprite, p.x - RESTO_PX / 2, p.y - RESTO_PX / 2, RESTO_PX, RESTO_PX);
            restoCount += 1;
          } else {
            const p = map.project([o.do, o.da]);
            pickData.orders.push({ id: o.i, x: p.x, y: p.y });
            // Ikon rumah: dropoff / pelanggan sedang diantar.
            ctx.globalAlpha = 0.9;
            ctx.drawImage(houseSprite, p.x - HOUSE_PX / 2, p.y - HOUSE_PX / 2, HOUSE_PX, HOUSE_PX);
            houseCount += 1;
          }
        }
        ctx.globalAlpha = 1;

        // --- rider: glow (aktif) + motor menghadap arah gerak (fase 10) ---
        const targetByRider = new Map<number, [number, number]>();
        for (const link of pair.next.l) {
          const o = orderById.get(link.o);
          if (o) {
            targetByRider.set(
              link.r,
              o.s === ORDER_STATUS.inTransit ? [o.da, o.do] : [o.pa, o.po],
            );
          }
        }
        for (const r of pair.next.r) {
          const pos = positions.get(r.i);
          if (!pos) continue;
          const p = map.project([pos[1], pos[0]]);
          pickData.riders.set(r.i, [p.x, p.y]);
          // Heading: arah prev→posisi terinterpolasi di ruang layar (stabil
          // thd kamera — rotasi peta dinonaktifkan). Rider yang belum
          // bergerak menghadap target assignmentnya.
          const prevR = prevIdx.get(r.i);
          if (prevR) {
            const pp = map.project([prevR.lo, prevR.la]);
            const dx = p.x - pp.x;
            const dy = p.y - pp.y;
            if (Math.hypot(dx, dy) > 0.6) headings.set(r.i, Math.atan2(dy, dx));
          }
          if (!headings.has(r.i)) {
            const tgt = targetByRider.get(r.i);
            if (tgt) {
              const tp = map.project([tgt[1], tgt[0]]);
              headings.set(r.i, Math.atan2(tp.y - p.y, tp.x - p.x));
            }
          }
          const angle = headings.get(r.i) ?? 0;
          const idle = r.s === 0;
          if (!idle) {
            const glow = glows[r.s] ?? glows[0];
            // Dark: glow aditif; light: halo biasa (lighter → putih pudar).
            ctx.globalCompositeOperation = themeLight ? 'source-over' : 'lighter';
            ctx.globalAlpha = themeLight ? 0.5 : 1;
            ctx.drawImage(glow, p.x - 13, p.y - 13, 26, 26);
            ctx.globalCompositeOperation = 'source-over';
            ctx.globalAlpha = 1;
          }
          const size = idle ? MOTOR_PX_IDLE : MOTOR_PX_ACTIVE;
          ctx.save();
          ctx.translate(p.x, p.y);
          ctx.rotate(angle);
          ctx.globalAlpha = idle ? 0.62 : 1;
          ctx.drawImage(motorSprites[r.s] ?? motorSprites[0], -size / 2, -size / 2, size, size);
          ctx.restore();
          motorCount += 1;
        }
        ctx.globalAlpha = 1;

        // --- cincin highlight entri terpilih (inspect) ---
        if (selKey) {
          let sx = 0;
          let sy = 0;
          let found = false;
          if (selKey.kind === 'rider') {
            const pos = positions.get(selKey.id as number);
            if (pos) {
              const p = map.project([pos[1], pos[0]]);
              sx = p.x;
              sy = p.y;
              found = true;
            }
          } else {
            const so = orderById.get(selKey.id as string);
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
            ctx.arc(sx, sy, 10, 0, Math.PI * 2);
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
            ctx.arc(hx, hy, 9, 0, Math.PI * 2);
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
        // terakhir (hit-test hover/klik diuji headless) + statistik ikon.
        (window as unknown as { __lmPick?: unknown }).__lmPick = {
          riders: Array.from(pickData.riders.entries(), ([id, [x, y]]) => ({ id, x, y })).slice(0, 80),
          orders: pickData.orders.slice(0, 80),
        };
        (window as unknown as { __lmIcons?: unknown }).__lmIcons = {
          motor: motorCount,
          resto: restoCount,
          house: houseCount,
          lines: linesDrawn,
          zoom: Math.round(zoom * 100) / 100,
          riders: pair.next.r.length,
          orders: pair.next.o.length,
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
      unsubTheme?.();
      trails.clear();
      pulseSeen.clear();
      knownOrders.clear();
      bursts.length = 0;
      headings.clear();
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
        title="Live Berlin ops: motorcycles = riders (they face their direction of travel), restaurant icons = pickups, houses = dropoffs; hover or click one for detail; street names appear as you zoom"
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
