#!/usr/bin/env node
/**
 * One-shot (fase 8): tambah properti `n` (nama jalan OSM) ke
 * apps/web/public/berlin/roads.geojson TANPA menyentuh graph routing.
 *
 * Kendala inti: roads.geojson = edge HASIL SIMPLIFIKASI graphgen — ujungnya
 * node persimpangan (derajat ≠ 2), bukan node OSM berurutan, jadi pasangan
 * koordinat OSM tidak bisa di-match langsung. Solusinya: replikasi persis
 * logika simplifikasi graphgen (walk chain node derajat-2) di atas data
 * Overpass mentah, kumpulkan nama semua segmen dalam chain, ambil mayoritas,
 * lalu match ke fitur geojson via pasangan endpoint (presisi 5 desimal,
 * sama seperti round5() graphgen).
 *
 * Graph `berlin_graph.json` TIDAK disentuh — ini murni props visual.
 * Jalankan: node scripts/add-road-names.mjs  (butuh internet; idempoten)
 */
import { readFileSync, writeFileSync } from 'node:fs';

const PATH = 'apps/web/public/berlin/roads.geojson';
// Bbox graph (meta berlin_graph.json) — dipecah 4 kuadran karena mirror
// Overpass kerap 504 pada query besar. Way yang memotong batas kuadran
// dikembalikan utuh oleh Overpass → duplikat aman (map mendebug duplikat).
const QUADRANTS = [
  ['52.49', '13.38', '52.511', '13.415'],
  ['52.49', '13.415', '52.511', '13.45'],
  ['52.511', '13.38', '52.532', '13.415'],
  ['52.511', '13.415', '52.532', '13.45'],
];
const HIGHWAY_RE = '^(motorway|trunk|primary|secondary|tertiary|unclassified|residential|living_street|service|motorway_link|trunk_link|primary_link|secondary_link|tertiary_link)$';
const ENDPOINTS = [
  // overpass-api.de kerap 406 (WAF); mirror dipakai dengan backoff.
  'https://maps.mail.ru/osm/tools/overpass/api/interpreter',
  'https://overpass.kumi.systems/api/interpreter',
  'https://overpass-api.de/api/interpreter',
];

const round5 = (v) => Math.round(v * 1e5) / 1e5;
const key6 = (lat, lon) => `${Math.round(lat * 1e6)},${Math.round(lon * 1e6)}`;
// pasangan endpoint output (round5) — urutan bebas
const pairKey = (a, b) => {
  const p = [a, b].sort((s, t) => s[0] - t[0] || s[1] - t[1]);
  return `${p[0][0]},${p[0][1]}|${p[1][0]},${p[1][1]}`;
};

async function fetchQuadrant([s, w, n, e]) {
  const q = `[out:json][timeout:120];way["highway"~"${HIGHWAY_RE}"](${s},${w},${n},${e});out geom;`;
  for (let attempt = 1; attempt <= 4; attempt++) {
    for (const url of ENDPOINTS) {
      try {
        console.log(`  coba ${attempt} → ${new URL(url).host}`);
        const res = await fetch(url, { method: 'POST', body: q });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return await res.json();
      } catch (err) {
        console.error('    gagal:', err.message);
      }
    }
    if (attempt < 4) {
      const backoff = 20000 * attempt;
      console.log(`  menunggu ${backoff / 1000}s…`);
      await new Promise((r) => setTimeout(r, backoff));
    }
  }
  return null;
}

// ---- 1. kumpulkan way OSM dari 4 kuadran ----
const ways = new Map(); // way id → {geom:[{lat,lon}], name}
for (let qi = 0; qi < QUADRANTS.length; qi++) {
  console.log(`kuadran ${qi + 1}/4`);
  const data = await fetchQuadrant(QUADRANTS[qi]);
  if (!data) {
    console.error('kuadran gagal di semua endpoint — dibatalkan');
    process.exit(1);
  }
  for (const el of data.elements ?? []) {
    if (el.type !== 'way' || !Array.isArray(el.geometry) || el.geometry.length < 2) continue;
    ways.set(el.id, { geom: el.geometry, name: el.tags?.name ?? '' });
  }
  if (qi < QUADRANTS.length - 1) await new Promise((r) => setTimeout(r, 3000));
}
console.log(`ways unik: ${ways.size}`);

// ---- 2. raw graph (replika getNode graphgen: key = 1e6 dari koordinat mentah) ----
/** @type {Map<string, {out:[number,number], adj:{to:string,name:string,way:number}[]}>} */
const nodes = new Map();
const getNode = (c) => {
  const k = key6(c.lat, c.lon);
  let v = nodes.get(k);
  if (!v) {
    v = { out: [round5(c.lat), round5(c.lon)], adj: [] };
    nodes.set(k, v);
  }
  return k;
};
for (const [id, w] of ways) {
  const ks = w.geom.map(getNode);
  for (let i = 1; i < ks.length; i++) {
    if (ks[i] === ks[i - 1]) continue;
    const nm = w.name;
    nodes.get(ks[i - 1]).adj.push({ to: ks[i], name: nm, way: id });
    nodes.get(ks[i]).adj.push({ to: ks[i - 1], name: nm, way: id });
  }
}
console.log(`raw nodes: ${nodes.size}`);

// ---- 3. replika simplifikasi graphgen + mayoritas nama per chain ----
const majority = (names) => {
  const cnt = new Map();
  for (const n of names) {
    if (!n) continue;
    cnt.set(n, (cnt.get(n) ?? 0) + 1);
  }
  let best = null;
  let bestC = 0;
  for (const [n, c] of cnt) {
    if (c > bestC) {
      best = n;
      bestC = c;
    }
  }
  return best;
};

const namesByPair = new Map();
const seenPair = new Set();
let chains = 0;
for (const [vk, v] of nodes) {
  if (v.adj.length === 2) continue; // keep = derajat ≠ 2
  for (const start of v.adj) {
    const pk = [vk, start.to].sort().join('>');
    if (seenPair.has(pk)) continue;
    seenPair.add(pk);
    const names = [start.name];
    let prev = vk;
    let cur = start.to;
    while (true) {
      const cv = nodes.get(cur);
      if (cv.adj.length !== 2) break; // sampai node keep
      const nxt = cv.adj.find((e) => e.to !== prev);
      if (!nxt) break; // dead end — graphgen juga berhenti
      names.push(nxt.name);
      prev = cur;
      cur = nxt.to;
    }
    if (cur === vk) continue;
    chains++;
    const nm = majority(names);
    if (nm) namesByPair.set(pairKey(v.out, nodes.get(cur).out), nm);
  }
}
console.log(`simplified chains: ${chains}, bernama: ${namesByPair.size}`);

// ---- 4. merge ke roads.geojson ----
const geo = JSON.parse(readFileSync(PATH, 'utf8'));
let hit = 0;
let miss = 0;
let hadName = 0;
for (const f of geo.features) {
  const c = f.geometry.coordinates; // GeoJSON [lon, lat]
  const a = [c[0][1], c[0][0]];
  const b = [c[1][1], c[1][0]];
  const nm = namesByPair.get(pairKey(a, b));
  if (nm !== undefined) {
    f.properties.n = nm;
    hit++;
    if (f.properties.n) hadName++;
  } else {
    miss++;
  }
}
console.log(`match: ${hit}/${geo.features.length} (${((hit / geo.features.length) * 100).toFixed(1)}%) · tanpa nama: ${miss}`);
if (hit < geo.features.length * 0.5) {
  console.error('Match rate rendah — JANGAN disimpan, periksa query/presisi.');
  process.exit(1);
}
writeFileSync(PATH, JSON.stringify(geo));
console.log('OK — roads.geojson diperbarui:', PATH);
