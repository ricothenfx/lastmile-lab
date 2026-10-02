// Verifikasi fase 8 — Map Craft & Map Interactivity (headless Playwright).
// Target default: produksi https://lastmile-lab.ricothen.com (APP_URL override
// untuk stack lokal). Semua mutasi (surge) DIPULIHKAN ke ×1 di akhir.
//
// Jalankan (pola sesi sebelumnya):
//   docker run --rm --network host \
//     -v "$PWD:/repo" -v /tmp/kilo/node_modules:/deps -w /repo/depsless \
//     -e NODE_PATH=/deps mcr.microsoft.com/playwright:v1.63.0-noble \
//     node reports/phase-08-ui-verify.mjs
import { chromium } from 'playwright-core';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

const BASE = process.env.APP_URL ?? 'https://lastmile-lab.ricothen.com';
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
const fontRequests = [];
try {
  // ---------- Konteks utama (motion penuh) ----------
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text());
  });
  page.on('response', (r) => {
    if (r.url().includes('/fonts/')) fontRequests.push(r.url());
  });

  await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 45000 });
  log('live_link', 'ok');
  // Panduan first-run bisa menutupi peta — tutup via tombol/ESC (perilaku baku).
  await page.keyboard.press('Escape');
  await page.waitForTimeout(400);
  if (await page.$('[aria-label="Close guide"]')) {
    await page.locator('[aria-label="Close guide"]').click();
  }

  // ---- 1. instansi map + style baru (style termuat = frame pertama tergambar) ----
  await page.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
  await page.waitForFunction(
    () => window.__lmPick && window.__lmPick.riders.length > 0,
    null,
    { timeout: 90000 },
  );
  const style = await page.evaluate(() => {
    const s = window.__lmMap.getStyle();
    return {
      glyphs: s.glyphs,
      layers: s.layers.map((l) => l.id),
    };
  });
  log('glyphs_selfhosted', style.glyphs === '/fonts/{fontstack}/{range}.pbf' ? 'ok (/fonts/)' : `GAGAL: ${style.glyphs}`);
  for (const layer of ['roads-glow', 'roads-label-major', 'roads-label-minor']) {
    log(`layer_${layer}`, style.layers.includes(layer) ? 'ok' : 'HILANG');
  }

  // ---- 2. label jalan muncul saat zoom ----
  await page.evaluate(() => window.__lmMap.setZoom(15));
  let labelFeat = [];
  for (let i = 0; i < 30 && labelFeat.length === 0; i++) {
    await sleep(600);
    labelFeat = await page.evaluate(() => {
      const feats = window.__lmMap.queryRenderedFeatures({ layers: ['roads-label-major', 'roads-label-minor'] });
      return feats.map((f) => f.properties?.n).filter(Boolean);
    });
  }
  log('street_labels_rendered', labelFeat.length > 0 ? `ok (${labelFeat.length} label, contoh: "${labelFeat[0]}")` : 'HILANG');
  await page.screenshot({ path: `${OUT}/phase08-labels-z15.png` });
  await page.evaluate(() => window.__lmMap.setZoom(12.15));

  // ---- 3. font hanya dari domain sendiri ----
  const foreignFonts = fontRequests.filter((u) => !u.startsWith(BASE));
  log('font_requests_local', fontRequests.length > 0 && foreignFonts.length === 0 ? `ok (${fontRequests.length} request, 0 asing)` : foreignFonts.length > 0 ? `GAGAL: ${foreignFonts[0]}` : 'belum ada request font');

  // ---- 4. pilih entitas dengan titik bersih (tidak tertutup panel) ----
  const pick = await page.evaluate(() => {
    const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
    const clear = (x, y) => {
      const el = document.elementFromPoint(box.x + x, box.y + y);
      return !!el && el.classList?.contains('maplibregl-canvas');
    };
    const rider =
      window.__lmPick.riders.find((r) => clear(r.x, r.y)) ?? window.__lmPick.riders[0];
    return { rider, box };
  });

  // ---- 5. hover → tooltip ----
  const mx = pick.box.x + pick.rider.x;
  const my = pick.box.y + pick.rider.y;
  await page.mouse.move(mx, my, { steps: 3 });
  await sleep(150);
  const tip = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="map-tooltip"]');
    return el ? { text: el.textContent, visible: el.offsetParent !== null } : null;
  });
  log('hover_tooltip', tip && tip.visible && tip.text.includes(`RIDER r${pick.rider.id}`) ? `ok (RIDER r${pick.rider.id})` : 'GAGAL');
  const hoverMs = await page.evaluate(() => window.__lmHoverMs);
  log('hover_hit_test_ms', typeof hoverMs === 'number' && hoverMs < 16 ? `ok (${hoverMs.toFixed(3)} ms < 16)` : `GAGAL: ${hoverMs}`);
  await page.screenshot({ path: `${OUT}/phase08-hover-tooltip.png` });

  // ---- 6. klik dot → kartu inspect live ----
  await page.mouse.click(mx, my);
  await page.waitForSelector('[data-testid="live-inspect"]', { timeout: 5000 });
  const card = await page.evaluate(() => document.querySelector('[data-testid="live-inspect"]').textContent);
  log('live_inspect_card', card.includes(`RIDER r${pick.rider.id}`) ? 'ok' : `GAGAL: ${card.slice(0, 60)}`);
  await page.screenshot({ path: `${OUT}/phase08-live-inspect.png` });
  // Escape menutup kartu live inspect (plus klik area kosong sebagai UX)
  await page.keyboard.press('Escape');
  await sleep(200);
  log('live_inspect_deselect', (await page.$('[data-testid="live-inspect"]')) === null ? 'ok' : 'MASIH TERBUKA');

  // ---- 7. zona panas: probe + reaksi surge (dipulihkan ×1) ----
  const heat0 = await page.evaluate(() => window.__lmHeat);
  log('heat_probe', heat0 && typeof heat0.cells === 'number' ? `ok (cells=${heat0.cells}, max=${heat0.max}, total=${heat0.total}, su=${heat0.su})` : 'HILANG');
  const surgeRes = await page.evaluate(async (api) => {
    const r = await fetch(`${api}/api/control/surge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ factor: 4 }),
    });
    return r.status;
  }, API);
  let heat1 = heat0;
  if (surgeRes === 200) {
    for (let i = 0; i < 20; i++) {
      await sleep(500);
      heat1 = await page.evaluate(() => window.__lmHeat);
      if (heat1 && heat1.su >= 3.5) break;
    }
    log('heat_surge_echo', heat1.su >= 3.5 ? `ok (su=${heat1.su})` : `GAGAL: su=${heat1.su}`);
    // intensitas nyata = jumlah alpha yang digambar (bukan geometri sel)
    const grew = (heat1.alphaSum ?? 0) > (heat0.alphaSum ?? 0) * 1.3;
    log(
      'heat_intensifies',
      grew
        ? `ok (alphaSum ${heat0.alphaSum}→${heat1.alphaSum} pada su ${heat0.su}→${heat1.su})`
        : `GAGAL: alphaSum ${heat0.alphaSum}→${heat1.alphaSum} pada su ${heat0.su}→${heat1.su}`,
    );
  } else {
    log('heat_surge_echo', `SKIP (HTTP ${surgeRes})`);
  }
  await page.screenshot({ path: `${OUT}/phase08-heat.png` });
  // pulihkan ×1
  const restoreRes = await page.evaluate(async (api) => {
    const r = await fetch(`${api}/api/control/surge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ factor: 1 }),
    });
    return r.status;
  }, API);
  log('surge_restored', restoreRes === 200 ? 'ok (×1)' : `GAGAL: HTTP ${restoreRes}`);

  // ---- 8. delivery burst dalam 40 detik ----
  let bursts = null;
  try {
    await page.waitForFunction(() => window.__lmBursts && window.__lmBursts.delivered > 0, null, { timeout: 40000 });
    bursts = await page.evaluate(() => window.__lmBursts);
  } catch {
    bursts = await page.evaluate(() => window.__lmBursts);
  }
  log('delivery_burst', bursts && bursts.delivered > 0 ? `ok (delivered=${bursts.delivered}, expired=${bursts.expired})` : `GAGAL: ${JSON.stringify(bursts)}`);

  // ---- 9. audit rAF idle (tanpa loop baru) ----
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

  // ---- 10. replay inspect tidak regresi ----
  await page.getByRole('button', { name: /REPLAY/i }).click();
  await page.waitForFunction(() => /replay & inspect/i.test(document.body.innerText), null, { timeout: 30000 });
  await page.waitForFunction(
    () => /LIVE SESSION|FIXTURE \(OFFLINE\)/.test(document.body.innerText),
    null,
    { timeout: 90000 },
  );
  await sleep(800); // tunggu pickData frame replay aktif
  const rpick = await page.evaluate(() => {
    const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
    const pts = [
      ...window.__lmPick.riders.map((r) => [r.x, r.y]),
      ...window.__lmPick.orders.map((o) => [o.x, o.y]),
    ];
    // titik rider kandidat harus: elemen target = canvas peta, tak tertutup
    // panel, dan berjarak > 24 px dari entitas LAIN (dirinya dikecualikan).
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
    log('replay_inspect_reason', cardShown ? 'ok (kartu inspect replay tampil)' : 'GAGAL (kartu inspect tidak tampil)');
  } else {
    log('replay_inspect_reason', 'SKIP (tidak ada rider di frame replay)');
  }
  await page.screenshot({ path: `${OUT}/phase08-replay-regression.png` });
  await page.locator('[aria-label="Replay dan Inspect"] button', { hasText: '✕' }).click().catch(() => {});

  // ---- 11. responsive ----
  for (const [w2, h2] of [[1024, 768], [768, 900]]) {
    await page.setViewportSize({ width: w2, height: h2 });
    await sleep(600);
    await page.screenshot({ path: `${OUT}/phase08-responsive-${w2}.png` });
  }
  log('responsive_screens', 'ok (1024, 768)');

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
  const rmHeat = await p2.evaluate(() => window.__lmHeat);
  log('reduced_motion_heat_flag', rmHeat && rmHeat.reduced === true ? 'ok' : 'GAGAL');
  const rmPick = await p2.evaluate(() => {
    const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
    const clear = (x, y) => {
      const el = document.elementFromPoint(box.x + x, box.y + y);
      return !!el && el.classList?.contains('maplibregl-canvas');
    };
    const rider = window.__lmPick.riders.find((r) => clear(r.x, r.y)) ?? window.__lmPick.riders[0];
    return { rider, box };
  });
  await p2.mouse.move(rmPick.box.x + rmPick.rider.x, rmPick.box.y + rmPick.rider.y, { steps: 3 });
  await sleep(150);
  const rmTip = await p2.evaluate(() => !!document.querySelector('[data-testid="map-tooltip"]'));
  log('reduced_motion_tooltip', rmTip ? 'ok (tooltip tetap bekerja)' : 'GAGAL');
  await p2.screenshot({ path: `${OUT}/phase08-reduced-motion.png` });
  await ctx2.close();

  const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map((f) => f.join('=')).join(', ')}` : 'ALL PHASE-08 CHECKS PASSED');
  process.exit(fails.length || appErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
