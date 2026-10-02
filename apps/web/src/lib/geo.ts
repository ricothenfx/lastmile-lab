/** Utilitas geo kecil bersama (fase 8) — jarak haversine + format. */

/** Jarak great-circle meter antara dua titik (lat, lon). */
export function haversineM(
  aLat: number,
  aLon: number,
  bLat: number,
  bLon: number,
): number {
  const r = 6371000;
  const dLat = ((bLat - aLat) * Math.PI) / 180;
  const dLon = ((bLon - aLon) * Math.PI) / 180;
  const la = (aLat * Math.PI) / 180;
  const lb = (bLat * Math.PI) / 180;
  const h =
    Math.sin(dLat / 2) ** 2 + Math.cos(la) * Math.cos(lb) * Math.sin(dLon / 2) ** 2;
  return 2 * r * Math.asin(Math.sqrt(h));
}

/** Format jalan pendek: "820 m" / "1.4 km". */
export function fmtDist(m: number): string {
  if (!Number.isFinite(m) || m < 0) return '—';
  return m >= 1000 ? `${(m / 1000).toFixed(1)} km` : `${Math.round(m)} m`;
}
