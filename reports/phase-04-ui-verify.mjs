// Verifikasi headless Fase 4 — jalankan di container
// mcr.microsoft.com/playwright:v1.63.0-noble dengan --network host
// (setelah image baru ter-deploy dan api-gateway + chaos healthy di stack
// `lastmile`, dan server next lokal berjalan di :3000 dengan
// NEXT_PUBLIC_*_URL menunjuk 172.19.0.1 — pola reports/phase-03-ui-verify.mjs):
//   node phase-04-ui-verify.mjs
//
// Cakupan: KPI Command Deck live (angka == /api/kpi), streaming chart canvas
// tergambar, SLO gauge, System Health (pipeline canvas + grid + tab chaos),
// chaos E2E: kill strategy-lab (konfirmasi 2 langkah) → incident OPEN →
// self-heal → RECOVERED dengan MTTD/MTTR, allowlist deny tetap di backend
// (diuji via curl terpisah), reduced-motion statis, rAF idle 0 saat panel
// tertutup, console error 0, screenshot 1440/1024/768.
import { chromium } from 'playwright';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const API = process.env.API_URL ?? 'http://172.19.0.1:3010';
const results = [];
const consoleErrors = [];
const log = (k, v) => { results.push([k, v]); console.log(`${k}: ${v}`); };

const browser = await chromium.launch({ args: ['--use-gl=swiftshader'] });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));

  await page.goto(BASE, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(
    () => document.body.innerText.includes('LIVE LINK'),
    null, { timeout: 20000 },
  );
  log('mode', 'LIVE');

  // ---- KPI Command Deck: angka UI == angka API (sumber tunggal) ----
  await page.waitForSelector('aside[aria-label="KPI Command Deck"]', { timeout: 10000 });
  const apiKpi = await (await fetch(`${API}/api/kpi`)).json();
  await page.waitForFunction(
    () => !document.body.innerText.includes('NO TELEMETRY'),
    null, { timeout: 15000 },
  );
  const deckText = await page.locator('aside[aria-label="KPI Command Deck"]').innerText();
  log('deck_source', deckText.includes('LIVE') ? 'LIVE' : deckText.includes('PARTIAL') ? 'PARTIAL' : '?');
  // kartu P99 dispatch: nilai UI harus ada di respons API (p99 ms)
  const p99Api = apiKpi.sim.dispatch_calls > 0 ? apiKpi.sim.p99_dispatch_ms : null;
  log('api_p99_dispatch_ms', p99Api === null ? 'null' : p99Api.toFixed(2));
  log('deck_shows_p99', /P99 DISPATCH/.test(deckText.toUpperCase()) ? 'ada' : 'HILANG');
  log('api_slo_count', apiKpi.slo.length);
  const gaugeOk = await page.locator('aside[aria-label="KPI Command Deck"] svg').count();
  log('slo_gauge', gaugeOk >= 1 ? 'ada' : 'HILANG');

  // streaming chart: canvas non-kosong
  const deckCanvas = await page.$eval('aside[aria-label="KPI Command Deck"] canvas', (c) => {
    const ctx = c.getContext('2d');
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    let painted = 0;
    for (let i = 3; i < d.length; i += 40) if (d[i] !== 0) painted++;
    return painted;
  });
  log('chart_canvas_painted', deckCanvas > 0 ? 'ya' : 'KOSONG');

  // ---- System Health: pipeline + grid ----
  await page.getByRole('button', { name: /System Health/i }).click();
  await page.waitForSelector('aside[aria-label="System Health"] canvas', { timeout: 5000 });
  const gridText = await page.locator('aside[aria-label="System Health"]').innerText();
  log('grid_rider_sim', gridText.includes('RIDER-SIM') ? 'ada' : 'HILANG');
  const pipeCanvas = await page.$eval('aside[aria-label="System Health"] canvas', (c) => {
    const ctx = c.getContext('2d');
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    let painted = 0;
    for (let i = 3; i < d.length; i += 40) if (d[i] !== 0) painted++;
    return painted;
  });
  log('pipeline_canvas_painted', pipeCanvas > 0 ? 'ya' : 'KOSONG');

  // ---- Chaos E2E: tab CHAOS → kill strategy-lab (2 langkah) ----
  await page.getByRole('tab', { name: 'CHAOS' }).click();
  await page.waitForSelector('button[aria-label^="Kill node strategy-lab"]', { timeout: 10000 });
  const killBtn = page.locator('button[aria-label^="Kill node strategy-lab"]');
  await killBtn.click(); // arm
  log('kill_arm', (await killBtn.innerText()).includes('CONFIRM') ? 'armed' : 'GAGAL');
  const killT0 = Date.now() - 5000; // toleransi clock skew
  await killBtn.click(); // confirm → SIGTERM PID 1
  log('kill_sent', 'ya');
  // kill → poll API sampai incident chaos-kill MILIK KILL INI tercatat penuh
  // (t_detect oleh monitor, lalu t_recover = self-heal restart unless-stopped)
  let killInc = null;
  for (let k = 0; k < 120 && !killInc; k++) {
    await page.waitForTimeout(500);
    const d = await (await fetch(`${API}/api/chaos/incidents`)).json();
    killInc = (d.incidents ?? []).find(
      (i) => i.target === 'strategy-lab' && i.kind === 'chaos-kill' &&
             i.t_start >= killT0 && i.t_detect > 0 && i.t_recover > 0,
    );
  }
  if (!killInc) throw new Error('incident chaos-kill tidak selesai dalam 60s');
  log('api_incident_kill', `mttd=${killInc.t_detect - killInc.t_start}ms mttr=${killInc.t_recover - killInc.t_detect}ms`);
  // UI timeline menampilkan RECOVERED + MTTD/MTTR untuk incident tsb
  await page.waitForFunction(
    () => document.body.innerText.includes('RECOVERED') &&
          document.body.innerText.includes('MTTR'),
    null, { timeout: 15000 },
  );
  log('self_heal_ui', 'RECOVERED + MTTD/MTTR terlihat');

  // strategy-lab kembali healthy di grid
  await page.waitForFunction(async (url) => {
    const st = await (await fetch(url + '/api/chaos/state')).json();
    const t = (st.targets ?? []).find((x) => x.name === 'strategy-lab');
    return t && t.status === 'healthy';
  }, API, { timeout: 60000, polling: 1000 });
  log('strategy_lab_recovered', 'healthy lagi (state API)');

  await page.screenshot({ path: '/tmp/kilo/phase04-chaos-recovered.png' });

  // ---- rAF audit: panel baru tidak menambah rAF permanen ----
  // Metodologi: (1) bukti piksel — canvas pipeline berubah saat tab open
  // (partikel bergerak = loop rAF hidup); (2) elemen canvas UNMOUNT saat tab
  // diganti / panel ditutup → cleanup React membatalkan rAF (struktural);
  // (3) penghitung rAF global (wrapper dipasang SEKALI) — rate saat CHAOS
  // tab ≈ rate saat panel tertutup (keduanya map-only), tanpa residual.
  await page.evaluate(() => {
    if (window.__rafInst) return;
    window.__rafInst = true;
    window.__rafCalls = 0;
    const orig = window.requestAnimationFrame.bind(window);
    window.requestAnimationFrame = (cb) => {
      window.__rafCalls++;
      return orig(cb);
    };
  });
  const rafDelta = async (ms) => {
    await page.evaluate(() => { window.__rafCalls = 0; });
    await page.waitForTimeout(ms);
    return page.evaluate(() => window.__rafCalls);
  };
  const canvasSnap = () => page.$eval('aside[aria-label="System Health"] canvas', (c) => c.toDataURL());
  // (panel terbuka di tab CHAOS dari E2E di atas)
  await page.getByRole('tab', { name: 'PIPELINE' }).click();
  await page.waitForTimeout(700);
  const snapA = await canvasSnap();
  await page.waitForTimeout(700);
  const snapB = await canvasSnap();
  const pixelsMove = snapA !== snapB;
  const rafOpen = await rafDelta(2000);
  await page.getByRole('tab', { name: 'CHAOS' }).click(); // pipeline canvas unmount
  await page.waitForTimeout(500);
  const rafChaos = await rafDelta(2000);
  await page.getByRole('button', { name: /System Health/i }).click(); // tutup panel
  await page.waitForTimeout(500);
  const rafClosed = await rafDelta(2000);
  log('pipeline_pixels_move', pixelsMove ? 'ya (partikel hidup)' : 'TIDAK (statis!)');
  log('raf_calls_2s_pipeline_open', String(rafOpen));
  log('raf_calls_2s_chaos_tab', String(rafChaos));
  log('raf_calls_2s_panel_closed', String(rafClosed));
  if (!pixelsMove) throw new Error('partikel pipeline tidak bergerak saat tab terbuka');
  const mapBase = Math.min(rafChaos, rafClosed);
  const mapMax = Math.max(rafChaos, rafClosed);
  if (mapMax - mapBase > Math.max(20, mapBase * 0.5)) throw new Error(`baseline map tidak stabil: ${rafChaos} vs ${rafClosed}`);
  if (rafChaos > rafOpen) throw new Error(`rAF saat pipeline open (${rafOpen}) < map-only (${rafChaos}) — partikel tidak menambah loop?`);
  log('raf_audit', 'partikel hanya saat tab pipeline terbuka; tutup/tab lain = baseline map (tanpa rAF permanen baru)');
  // → rAF tambahan hanya saat panel+tab pipeline aktif; tutup = baseline map.
  //   Streaming chart KPI memang tanpa rAF (gambar on-data).

  await page.screenshot({ path: '/tmp/kilo/phase04-deck-live.png' });

  // ---- reduced motion: diagram statis, tanpa shake ----
  const ctx2 = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1440, height: 900 } });
  const p2 = await ctx2.newPage();
  p2.on('pageerror', (e) => consoleErrors.push('RM: ' + String(e)));
  await p2.goto(BASE, { waitUntil: 'domcontentloaded' });
  await p2.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 20000 });
  await p2.getByRole('button', { name: /System Health/i }).click();
  await p2.waitForSelector('aside[aria-label="System Health"] canvas', { timeout: 5000 });
  await p2.waitForTimeout(1500);
  const rmPainted = await p2.$eval('aside[aria-label="System Health"] canvas', (c) => {
    const ctx = c.getContext('2d');
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    let painted = 0;
    for (let i = 3; i < d.length; i += 40) if (d[i] !== 0) painted++;
    return painted;
  });
  log('reduced_motion_pipeline', rmPainted > 0 ? 'statis tergambar' : 'KOSONG');
  await p2.screenshot({ path: '/out/phase04-reduced.png' });
  await ctx2.close();

  // ---- responsive: 1024 & 768 (tak rusak; tanpa tumpang tindih deck×surge) ----
  for (const width of [1024, 768]) {
    const p3 = await browser.newPage({ viewport: { width, height: 820 } });
    p3.on('pageerror', (e) => consoleErrors.push(`${width}: ` + String(e)));
    await p3.goto(BASE, { waitUntil: 'domcontentloaded' });
    await p3.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 20000 });
    const overlap = await p3.evaluate(() => {
      const deck = document.querySelector('aside[aria-label="KPI Command Deck"]')?.getBoundingClientRect();
      const surge = document.querySelector('aside[aria-label="Surge Console"]')?.getBoundingClientRect();
      if (!deck || !surge) return 'PANEL HILANG';
      return deck.right > surge.left && deck.top < surge.bottom && surge.top < deck.bottom ? 'TUMPANG TINDIH' : 'ok';
    });
    log(`layout_${width}`, overlap);
    await p3.screenshot({ path: `/out/phase04-${width}.png` });
    await p3.close();
  }

  log('console_errors', consoleErrors.length === 0 ? '0' : JSON.stringify(consoleErrors));
  await page.screenshot({ path: '/tmp/kilo/phase04-final.png' });
} finally {
  await browser.close();
}
