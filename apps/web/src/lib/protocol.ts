/**
 * Tipe wire yang mirror pkg/model (backend Go) — key pendek, ukuran frame kecil.
 */
export interface RiderPt {
  i: number;
  s: number; // 0 idle · 1 to_pickup · 2 pickup · 3 delivering
  la: number;
  lo: number;
}

export interface OrderPt {
  i: string;
  s: number; // 0 waiting · 1 assigned · 2 in_transit
  pa: number; // pickup lat
  po: number; // pickup lon
  da: number; // dropoff lat
  do: number; // dropoff lon
}

export interface Link {
  r: number; // rider id
  o: string; // order id
}

export interface Stats {
  dl: number;
  ex: number;
  ac: number;
  id: number;
  stg?: string;
  up?: number;
  /** Fase 2 — field optional (kontrak tidak breaking; fixture lama aman). */
  cr?: number; // kumulatif order dibuat
  su?: number; // surge factor aktif (1 = normal, ≤10)
  we?: number; // weather factor aktif (1 = cerah, <1 hujan)
}

export interface Snapshot {
  t: number;
  seq: number;
  r: RiderPt[];
  o: OrderPt[];
  l: Link[];
  st: Stats;
}

export interface Fixture {
  meta: {
    city: string;
    source: string;
    bbox: number[];
    capturedAt: string;
    hz: number;
    riders: number;
    ratePerMin: number;
    seed: number;
    strategy: string;
  };
  frames: Snapshot[];
}

export const RIDER_STATUS = {
  idle: 0,
  toPickup: 1,
  pickup: 2,
  delivering: 3,
} as const;

export const ORDER_STATUS = {
  waiting: 0,
  assigned: 1,
  inTransit: 2,
} as const;
