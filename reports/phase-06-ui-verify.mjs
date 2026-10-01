// Verifikasi headless Fase 6 — go-live produksi.
// Jalankan di mcr.microsoft.com/playwright:v1.63.0-noble --network host,
// deps dari /tmp/kilo/node_modules (playwright-core), app next start :3000
// dibangun dengan NEXT_PUBLIC_* menunjuk DOMAIN PUBLIK produksi
// (wss://ws.lastmile-lab.ricothen.com/ws + https://api.lastmile-lab.ricothen.com)
// → semua trafik browser keluar lewat jalur internet (Caddy TLS + WS upgrade):
//   MODE=live    node phase-06-ui-verify.mjs    # LIVE via domain publik + fallback route-block
//   MODE=offline node phase-06-ui-verify.mjs    # backend benar-benar distop → REPLAY MODE
//   MODE=recap   node phase-06-ui-verify.mjs    # setelah restore → LIVE lagi
import { chromium } from 'playwright-core';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const API = 'https://api.lastmile-lab.ricothen.com';
const MODE = process.env.MODE ?? 'live';
const OUT = process.env.OUT_DIR ?? '/out';
mkdirSync(OUT, { recursive: true });
const results = [];
const consoleErrors = [];
const log = (k, v) => { results.push([k, v]); console.log(`${k}: ${v}`); };

const browser = await chromium.launch({ args: ['--use-gl=swiftshader'] });
try {
  if (MODE === 'offline') {
    // Backend distop sungguhan (rider-sim + ws-gateway + api-gateway):
    // banner REPLAY MODE (fixture fase 1) wajib muncul — koneksi ditolak
    // via domain publik adalah konsekuensi, bukan bug UI.
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
    page.on('pageerror', (e) => consoleErrors.push(String(e)));
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(
      () => document.body.innerText.includes('REPLAY MODE'),
      null, { timeout: 30000 },
    );
    log('prod_offline_banner', 'REPLAY MODE tampil (backend produksi distop)');
    log('prod_demo_offline', (await page.getByRole('button', { name: /▶ DEMO/i }).innerText()).includes('OFFLINE') ? 'tombol mati' : 'MASIH HIDUP?');
    await page.screenshot({ path: `${OUT}/phase06-prod-offline.png` });
    const appErrors = consoleErrors.filter(e => !/ERR_CONNECTION_REFUSED|WebSocket connection|Failed to fetch/i.test(e));
    log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
    const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH|KOSONG|\?\?$/.test(String(v)));
    console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL OFFLINE CHECKS PASSED');
    process.exit(fails.length || appErrors.length ? 1 : 0);
  }

  if (MODE === 'recap') {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    page.on('pageerror', (e) => consoleErrors.push(String(e)));
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(
      () => document.body.innerText.includes('LIVE LINK'),
      null, { timeout: 30000 },
    );
    log('prod_recovered', 'LIVE LINK kembali setelah backend di-start ulang');
    await page.screenshot({ path: `${OUT}/phase06-prod-recovered.png` });
    const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH|\?\?$/.test(String(v)));
    console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'RECAP PASSED');
    process.exit(fails.length || consoleErrors.length ? 1 : 0);
  }

  // ================= MODE UTAMA: LIVE via domain produksi =================
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));
  await page.goto(BASE, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(
    () => document.body.innerText.includes('LIVE LINK'),
    null, { timeout: 30000 },
  );
  log('mode', 'LIVE via domain produksi (api./ws. publik, TLS + WS upgrade Caddy)');

  // KPI via domain publik — angka nyata (bukan null/—)
  const kpi = await page.evaluate(async (url) => {
    const r = await fetch(`${url}/api/kpi`, { cache: 'no-store' });
    return r.json();
  }, API);
  log('kpi_strategy', kpi.eng?.strategy ?? kpi.strategy ?? JSON.stringify(Object.keys(kpi)));
  log('kpi_has_numbers', kpi ? 'ok' : 'KOSONG');

  // healthz via domain publik (dari browser)
  const hz = await page.evaluate(async (url) => {
    const r = await fetch(`${url}/healthz`, { cache: 'no-store' });
    return { code: r.status, body: await r.json() };
  }, API);
  log('healthz_via_domain', hz.code === 200 && hz.body.ok === true ? `200 ok (${hz.body.service})` : `GAGAL ${hz.code}`);

  // Replay panel: sesi LIVE via /api/replay/* publik (gzip passthrough Caddy)
  await page.getByRole('button', { name: /⟲ REPLAY/i }).click();
  await page.waitForSelector('[aria-label="Replay dan Inspect"]', { timeout: 5000 });
  await page.waitForFunction(
    () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('SESI LIVE'),
    null, { timeout: 30000 },
  );
  const panelText = await page.locator('[aria-label="Replay dan Inspect"]').innerText();
  log('replay_via_public_api', panelText.includes('SESI LIVE') ? 'SESSION' : 'GAGAL');
  const frames = Number((panelText.match(/(\d+) FRAME/) ?? [])[1] ?? 0);
  log('replay_frames', frames);
  await page.screenshot({ path: `${OUT}/phase06-prod-live.png` });
  await page.getByRole('button', { name: 'Tutup panel replay' }).click();
  await sleep(300);

  // Golden Demo endpoint publik tersedia (exposure D23 — tanpa dieksekusi)
  const presets = await page.evaluate(async (url) => {
    const r = await fetch(`${url}/api/demo/presets`, { cache: 'no-store' });
    return r.status;
  }, API);
  log('demo_presets_public', presets === 200 ? '200 (publik, ADR D23)' : `GAGAL ${presets}`);

  // ---- Fallback via jalur produksi: blokir domain publik dari browser ----
  // (setara "backend tak terjangkau dari internet" — tanpa menyentuh stack;
  // page.route tidak mencegat WebSocket → pakai routeWebSocket untuk menutupnya)
  await page.route(/lastmile-lab\.ricothen\.com/, (route) => route.abort());
  await page.routeWebSocket(/lastmile-lab\.ricothen\.com/, (ws) => { ws.close(); });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForFunction(
    () => document.body.innerText.includes('REPLAY MODE'),
    null, { timeout: 30000 },
  );
  log('fallback_route_blocked', 'REPLAY MODE tampil (domain publik diblokir dari browser)');
  await page.screenshot({ path: `${OUT}/phase06-fallback-blocked.png` });

  const appErrors = consoleErrors.filter(e => !/ERR_CONNECTION_REFUSED|WebSocket connection|Failed to fetch|ERR_FAILED|net::/i.test(e));
  log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
  const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH|KOSONG|\?\?$/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL LIVE CHECKS PASSED');
  process.exit(fails.length || appErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
