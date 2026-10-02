// Verifikasi frontend produksi Vercel (Fase 6 go-live).
// Browser headless → https://lastmile-lab.ricothen.com (Vercel) → api./ws. publik.
//   MODE=live    : LIVE via domain frontend produksi + KPI/replay via api. publik
//   MODE=offline : blokir api./ws. saja (backend tak terjangkau) → banner REPLAY MODE
import { chromium } from 'playwright-core';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

const BASE = process.env.APP_URL ?? 'https://lastmile-lab.ricothen.com';
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
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    page.on('pageerror', (e) => consoleErrors.push(String(e)));
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(
      () => document.body.innerText.includes('REPLAY MODE'),
      null, { timeout: 30000 },
    );
    log('vercel_fallback_backend_mati', 'REPLAY MODE tampil (api./ws. diblokir dari browser)');
    log('demo_offline', (await page.getByRole('button', { name: /▶ DEMO/i }).innerText()).includes('OFFLINE') ? 'tombol mati' : 'MASIH HIDUP?');
    await page.screenshot({ path: `${OUT}/phase06-vercel-replay.png` });
    const appErrors = consoleErrors.filter(e => !/ERR_CONNECTION_REFUSED|WebSocket|Failed to fetch|net::/i.test(e));
    log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
    const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH|\?\?$/.test(String(v)));
    console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'VERCEL OFFLINE CHECKS PASSED');
    process.exit(fails.length || appErrors.length ? 1 : 0);
  }

  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
  await page.waitForFunction(
    () => document.body.innerText.includes('LIVE LINK'),
    null, { timeout: 45000 },
  );
  log('vercel_live', 'LIVE LINK via https://lastmile-lab.ricothen.com (challenge platform lolos di browser asli)');
  const kpi = await page.evaluate(async (url) => {
    const r = await fetch(`${url}/api/kpi`, { cache: 'no-store' });
    return r.status;
  }, API);
  log('kpi_via_api_publik', kpi === 200 ? '200' : `GAGAL ${kpi}`);
  await page.screenshot({ path: `${OUT}/phase06-vercel-live.png` });

  // fallback: blokir HANYA api./ws. (frontend tetap hidup — jalur produksi nyata)
  await page.route(/(api|ws)\.lastmile-lab\.ricothen\.com/, (route) => route.abort());
  await page.routeWebSocket(/ws\.lastmile-lab\.ricothen\.com/, (ws) => { ws.close(); });
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForFunction(
    () => document.body.innerText.includes('REPLAY MODE'),
    null, { timeout: 30000 },
  );
  log('fallback_tanpa_backend', 'REPLAY MODE tampil setelah api./ws. diblokir');
  await page.screenshot({ path: `${OUT}/phase06-vercel-replay.png` });

  const appErrors = consoleErrors.filter(e => !/ERR_CONNECTION_REFUSED|WebSocket|Failed to fetch|net::/i.test(e));
  log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
  const fails = results.filter(([k, v]) => /GAGAL|HILANG|MASIH|\?\?$/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL VERCEL LIVE CHECKS PASSED');
  process.exit(fails.length || appErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
