// Ekspor POI kuliner dari graph routing rider-sim → layer peta statis
// (public/berlin/pois.geojson). Data yang sama, filosofi ADR D12: tanpa tile
// provider. Jalankan dari root repo:
//   node scripts/export-pois.mjs
import { readFileSync, writeFileSync } from 'node:fs';

const SRC = 'apps/services/rider-sim/data/berlin_graph.json';
const OUT = 'apps/web/public/berlin/pois.geojson';

const g = JSON.parse(readFileSync(SRC, 'utf8'));
const feats = g.pois.map((idx, i) => {
  const [lat, lon] = g.nodes[idx];
  return {
    type: 'Feature',
    id: i + 1,
    properties: { k: 'resto' },
    geometry: { type: 'Point', coordinates: [lon, lat] },
  };
});
const geo = {
  type: 'FeatureCollection',
  // Attribution wajib (ODbL) — sama dengan roads.geojson.
  attribution: '© OpenStreetMap contributors',
  features: feats,
};
writeFileSync(OUT, JSON.stringify(geo));
console.log(`pois: ${feats.length} POI → ${OUT}`);
