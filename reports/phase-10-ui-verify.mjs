// Verifikasi fase 10 — Symbol language + declutter + dark/light theme
// (headless Playwright, pola fase 8). Target: build produksi lokal
// (APP_URL) dengan env domain publik yang sama dengan Vercel.
//
// Jalankan (pola sesi sebelumnya):
//   docker run --rm --network host \
//     -v "$PWD:/repo" -v /tmp/kilo/node_modules:/deps \
//     -w /repo -e NODE_PATH=/deps -e OUT_DIR=/out \
//     -v /tmp/kilo/out:/out mcr.microsoft.com/playwright:v1.63.0-noble \
//     node reports/phase-10-ui-verify.mjs
import { createRequire } from 'node:module';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

// playwright-core dipasang di /deps (container); di luar container resolusi normal.
let chromium;
try {
  ({ chromium } = createRequire(import.meta.url)('playwright-core'));
} catch {
  ({ chromium } = createRequire(import.meta.url)('/deps/playwright-core'));
}

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3100';
const API = process.env.API_URL ?? 'https://api.lastmile-lab.ricothen.com';
const OUT = process.env.OUT_DIR ?? '/out';
mkdirSync(OUT, { recursive: true });

const results = [];
const consoleErrors = [];
const log = (k, v) => {
  results.push([k, v]);
  console.log(`${k}: ${v}`);
};
const GAGAL = (k) => log(k, 'GAGAL');

const browser = await chromium.launch({ args: ['--use-gl=swiftshader'] });
try {
  // ---------- Konteks utama (motion penuh, tema dark deterministik) ----------
  const page = await browser.newPage({
    viewport: { width: 1440, height: 900 },
    colorScheme: 'dark',
  });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text());
  });

  await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 45000 });
  log('live_link', 'ok');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(400);
  if (await page.$('[aria-label="Close guide"]')) {
    await page.locator('[aria-label="Close guide"]').click();
  }

  // ---- 1. map hidup + entitas tergambar ----
  await page.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
  await page.waitForFunction(
    () => window.__lmPick && window.__lmPick.riders.length > 0,
    null,
    { timeout: 90000 },
  );
  log('map_entities', 'ok');

  // ---- 2. bahasa ikon: motor / resto / rumah tergambar per frame ----
  let icons = null;
  for (let i = 0; i < 20; i++) {
    icons = await page.evaluate(() => window.__lmIcons);
    if (icons && icons.house > 0 && icons.resto > 0) break;
    await sleep(600);
  }
  log(
    'icon_motor',
    icons && icons.motor === icons.riders && icons.riders > 0
      ? `ok (motor=${icons.motor} == riders=${icons.riders})`
      : `GAGAL: ${JSON.stringify(icons)}`,
  );
  log(
    'icon_resto_house',
    icons && icons.resto + icons.house === icons.orders && icons.orders > 0
      ? `ok (resto=${icons.resto} + house=${icons.house} == orders=${icons.orders})`
      : `GAGAL: ${JSON.stringify(icons)}`,
  );
  await page.screenshot({ path: `${OUT}/phase10-default-dark.png` });

  // ---- 3. nama jalan terlihat di zoom DEFAULT (tanpa zoom manual; catatan:
  // maxBounds meng-clamp zoom minimum viewport 1440x900 ke ~13.4) ----
  let labelFeat = [];
  for (let i = 0; i < 20 && labelFeat.length === 0; i++) {
    await sleep(600);
    labelFeat = await page.evaluate(() =>
      window.__lmMap
        .queryRenderedFeatures({ layers: ['roads-label-major', 'roads-label-minor'] })
        .map((f) => f.properties?.n)
        .filter(Boolean),
    );
  }
  const zoomNow = icons ? icons.zoom : null;
  log(
    'street_labels_default_zoom',
    labelFeat.length > 0
      ? `ok (${labelFeat.length} label di zoom kamera ${zoomNow}, contoh: "${labelFeat[0]}")`
      : 'HILANG',
  );
  await page.screenshot({ path: `${OUT}/phase10-street-labels-default.png` });

  // ---- 4. label kawasan di zoom rendah ----
  await page.evaluate(() => window.__lmMap.setZoom(11.4));
  let places = [];
  for (let i = 0; i < 20 && places.length === 0; i++) {
    await sleep(500);
    places = await page.evaluate(() =>
      window.__lmMap
        .queryRenderedFeatures({ layers: ['places-district', 'places-place'] })
        .map((f) => f.properties?.name)
        .filter(Boolean),
    );
  }
  log(
    'area_labels',
    places.length > 0 ? `ok (${places.length}, contoh: ${places.slice(0, 3).join(', ')})` : 'HILANG',
  );

  // ---- 5. layer restoran (POI) muncul saat zoom-in ----
  await page.evaluate(() => window.__lmMap.setZoom(14.2));
  let pois = -1;
  for (let i = 0; i < 20 && pois <= 0; i++) {
    await sleep(500);
    pois = await page.evaluate(
      () => window.__lmMap.queryRenderedFeatures({ layers: ['poi-resto'] }).length,
    );
  }
  log('poi_resto_layer', pois > 0 ? `ok (${pois} POI terlihat di z14.2)` : `GAGAL (${pois})`);
  await page.screenshot({ path: `${OUT}/phase10-icons-poi-z14.2.png` });

  // ---- 6. gating garis assignment: 0 di zoom default, >0 saat zoom-in ----
  await page.evaluate(() => window.__lmMap.setZoom(12.15));
  await sleep(1500);
  const linesLow = await page.evaluate(() => window.__lmIcons.lines);
  await page.evaluate(() => window.__lmMap.setZoom(14.9));
  await sleep(1500);
  const linesHigh = await page.evaluate(() => window.__lmIcons.lines);
  log(
    'assignment_lines_gated',
    linesLow === 0 && linesHigh > 0
      ? `ok (default: ${linesLow} garis → z14.9: ${linesHigh} garis)`
      : `GAGAL: low=${linesLow}, high=${linesHigh}`,
  );
  await page.evaluate(() => window.__lmMap.setZoom(12.15));
  await sleep(800);

  // ---- 7. tema: toggle → data-theme + CSS var + paint MapLibre berubah ----
  const darkBg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  await page.getByRole('button', { name: /toggle dark or light theme/i }).click();
  await sleep(400);
  const theme1 = await page.evaluate(() => ({
    attr: document.documentElement.dataset.theme,
    bg: getComputedStyle(document.body).backgroundColor,
    mapBg: window.__lmMap.getStyle().layers.find((l) => l.id === 'bg').paint['background-color'],
  }));
  log(
    'theme_toggle_light',
    theme1.attr === 'light' && theme1.bg !== darkBg && /248,\s*250,\s*252/.test(theme1.bg) && /#F8FAFC/i.test(theme1.mapBg)
      ? `ok (body ${darkBg} → ${theme1.bg}, map bg ${theme1.mapBg})`
      : `GAGAL: ${JSON.stringify({ darkBg, ...theme1 })}`,
  );
  await page.screenshot({ path: `${OUT}/phase10-light.png` });
  await page.evaluate(() => window.__lmMap.setZoom(13.8));
  await sleep(800);
  await page.screenshot({ path: `${OUT}/phase10-light-map-zoom.png` });
  // persist: reload → tema tetap light
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
  const persisted = await page.evaluate(() => document.documentElement.dataset.theme);
  log('theme_persisted_reload', persisted === 'light' ? 'ok' : `GAGAL: ${persisted}`);
  // kembali ke dark via tombol
  await page.getByRole('button', { name: /toggle dark or light theme/i }).click();
  await sleep(300);
  const theme2 = await page.evaluate(() => document.documentElement.dataset.theme);
  log('theme_toggle_back_dark', theme2 === 'dark' ? 'ok' : `GAGAL: ${theme2}`);

  // ---- 8. hover → tooltip (regresi fase 8) ----
  await page.waitForFunction(() => window.__lmPick && window.__lmPick.riders.length > 0, null, { timeout: 90000 });
  const pick = await page.evaluate(() => {
    const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
    const clear = (x, y) => {
      const el = document.elementFromPoint(box.x + x, box.y + y);
      return !!el && el.classList?.contains('maplibregl-canvas');
    };
    const rider = window.__lmPick.riders.find((r) => clear(r.x, r.y)) ?? window.__lmPick.riders[0];
    return { rider, box };
  });
  await page.mouse.move(pick.box.x + pick.rider.x, pick.box.y + pick.rider.y, { steps: 3 });
  await sleep(150);
  const tip = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="map-tooltip"]');
    return el ? { text: el.textContent, visible: el.offsetParent !== null } : null;
  });
  log(
    'hover_tooltip',
    tip && tip.visible && tip.text.includes(`RIDER r${pick.rider.id}`)
      ? `ok (RIDER r${pick.rider.id})`
      : 'GAGAL',
  );
  // hover → garis assignment entitas itu tampil walau zoom-out (cari rider
  // yang membawa order — rider idle tidak punya garis). Posisi diambil FRESH
  // tiap percobaan — rider bergerak antar iterasi.
  let linesHover = 0;
  for (let i = 0; i < 20 && linesHover < 1; i++) {
    const rs = await page.evaluate(() => {
      const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
      const clear = (x, y) => {
        const el = document.elementFromPoint(box.x + x, box.y + y);
        return !!el && el.classList?.contains('maplibregl-canvas');
      };
      const r =
        window.__lmPick.riders.find((r) => clear(r.x, r.y)) ?? window.__lmPick.riders[0];
      return { r, box };
    });
    if (!rs.r) break;
    await page.mouse.move(rs.box.x + rs.r.x, rs.box.y + rs.r.y, { steps: 2 });
    await sleep(200);
    linesHover = await page.evaluate(() => window.__lmIcons.lines);
  }
  log('hover_line_focus', linesHover >= 1 ? `ok (${linesHover} garis saat hover)` : `GAGAL: ${linesHover}`);
  await page.mouse.click(pick.box.x + pick.rider.x, pick.box.y + pick.rider.y);
  await page.waitForSelector('[data-testid="live-inspect"]', { timeout: 5000 });
  log('live_inspect_card', 'ok');
  await page.keyboard.press('Escape');

  // ---- 9. panduan menyebut motor/restaurant/theme ----
  await page.getByRole('button', { name: /open usage guide/i }).click();
  await sleep(300);
  const guide = await page.evaluate(() => document.body.innerText);
  const guideOk =
    /Motorcycles/i.test(guide) && /restaurant/i.test(guide) && /Dark \/ light/i.test(guide);
  log('guide_updated', guideOk ? 'ok' : 'GAGAL (teks panduan belum memuat simbol baru)');
  await page.keyboard.press('Escape');

  // ---- 10. audit rAF idle (tanpa loop baru) ----
  const rafIdle = await page.evaluate(
    () =>
      new Promise((res) => {
        let n = 0;
        const orig = window.requestAnimationFrame.bind(window);
        window.requestAnimationFrame = (cb) => {
          n += 1;
          return orig(cb);
        };
        setTimeout(() => {
          window.requestAnimationFrame = orig;
          res(n);
        }, 2000);
      }),
  );
  log('raf_idle_2s', rafIdle < 200 ? `ok (${rafIdle})` : `GAGAL: ${rafIdle} (loop baru?)`);

  // ---- 11. replay inspect tidak regresi ----
  await page.getByRole('button', { name: /REPLAY/i }).click();
  await page.waitForFunction(() => /replay & inspect/i.test(document.body.innerText), null, { timeout: 30000 });
  await page.waitForFunction(
    () => /LIVE SESSION|FIXTURE \(OFFLINE\)/.test(document.body.innerText),
    null,
    { timeout: 90000 },
  );
  await sleep(800);
  const rpick = await page.evaluate(() => {
    const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
    const pts = [
      ...window.__lmPick.riders.map((r) => [r.x, r.y]),
      ...window.__lmPick.orders.map((o) => [o.x, o.y]),
    ];
    const clear = (x, y, self) => {
      const el = document.elementFromPoint(box.x + x, box.y + y);
      if (!el || !el.classList.contains('maplibregl-canvas')) return false;
      const others = pts.filter(([px, py]) => px !== self[0] || py !== self[1]);
      return Math.min(...others.map(([px, py]) => Math.hypot(px - x, py - y)), 1e9) > 24;
    };
    const rider =
      window.__lmPick?.riders?.find((r) => clear(r.x, r.y, [r.x, r.y])) ??
      window.__lmPick?.riders?.[0] ??
      null;
    return { rider, box };
  });
  if (rpick.rider) {
    await page.mouse.click(rpick.box.x + rpick.rider.x, rpick.box.y + rpick.rider.y);
    await sleep(600);
    const cardShown = await page.evaluate(
      () =>
        /dispatch decision reason/i.test(document.body.innerText) ||
        !!document.querySelector('[aria-label="Detail order"]'),
    );
    log('replay_inspect_reason', cardShown ? 'ok (kartu inspect replay tampil)' : 'GAGAL');
  } else {
    log('replay_inspect_reason', 'SKIP (tidak ada rider di frame replay)');
  }
  await page.screenshot({ path: `${OUT}/phase10-replay.png` });
  await page.locator('[aria-label="Replay dan Inspect"] button', { hasText: '✕' }).click().catch(() => {});

  // ---- 13. responsive ----
  for (const [w2, h2] of [[1024, 768], [768, 900]]) {
    await page.setViewportSize({ width: w2, height: h2 });
    await sleep(600);
    await page.screenshot({ path: `${OUT}/phase10-responsive-${w2}.png` });
  }
  log('responsive_screens', 'ok (1024, 768)');

  // ---- 14. ikon rumah (dropoff in-transit): pulihkan surge/rain ke ×1 ----
  // Kondisi live sebelum uji: surge ×10 + rain → semua rider to_pickup,
  // nol order in_transit. Pulihkan ke baseline (pola fase 8: mutasi
  // dipulihkan) → in_transit muncul saat rider menyelesaikan pickup.
  await page.setViewportSize({ width: 1440, height: 900 });
  const restoreRes = await page.evaluate(async (api) => {
    const a = await fetch(`${api}/api/control/surge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ factor: 1 }),
    });
    const b = await fetch(`${api}/api/control/weather`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ factor: 1 }),
    });
    return `${a.status}/${b.status}`;
  }, API);
  log('surge_weather_restored', restoreRes === '200/200' ? 'ok (×1, cerah)' : `GAGAL: ${restoreRes}`);
  let houseIcons = null;
  for (let i = 0; i < 60; i++) {
    await sleep(2500);
    houseIcons = await page.evaluate(() => window.__lmIcons);
    if (houseIcons && houseIcons.house > 0) break;
  }
  log(
    'house_icon_live',
    houseIcons && houseIcons.house > 0
      ? `ok (house=${houseIcons.house} setelah surge ×1)`
      : `GAGAL: ${JSON.stringify(houseIcons)}`,
  );
  await page.screenshot({ path: `${OUT}/phase10-house-intransit.png` });

  const appErrors = consoleErrors.filter(
    (e) => !/ERR_CONNECTION_REFUSED|WebSocket|Failed to fetch|net::|406|Load failed/i.test(e),
  );
  log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
  await page.close();

  // ---------- Konteks reduced-motion ----------
  const ctx2 = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1440, height: 900 } });
  const p2 = await ctx2.newPage();
  p2.on('pageerror', (e) => consoleErrors.push(`RM: ${e}`));
  await p2.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await p2.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
  await p2.keyboard.press('Escape');
  await p2.waitForTimeout(400);
  if (await p2.$('[aria-label="Close guide"]')) {
    await p2.locator('[aria-label="Close guide"]').click();
  }
  await p2.waitForFunction(() => window.__lmPick && window.__lmPick.riders.length > 0, null, { timeout: 90000 });
  const rmIcons = await p2.evaluate(() => window.__lmIcons);
  log(
    'reduced_motion_icons',
    rmIcons && rmIcons.motor > 0 ? `ok (motor=${rmIcons.motor})` : `GAGAL: ${JSON.stringify(rmIcons)}`,
  );
  await p2.screenshot({ path: `${OUT}/phase10-reduced-motion.png` });
  await ctx2.close();

  const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map((f) => f.join('=')).join(', ')}` : 'ALL PHASE-10 CHECKS PASSED');
  process.exit(fails.length || appErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
