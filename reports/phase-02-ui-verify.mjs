// Verifikasi headless Surge Console (Fase 2) — jalankan di container
// mcr.microsoft.com/playwright dengan --network host:
//   node verify-surge.mjs
// Cakupan: mode LIVE, fps rAF, reaksi kontrol < 1 s (echo BACKEND via baris
// EFFECT yang hanya berisi nilai snapshot), reduced-motion, screenshot.
import { chromium } from 'playwright';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const results = [];
const consoleErrors = [];
const log = (k, v) => { results.push([k, v]); console.log(`${k}: ${v}`); };

const browser = await chromium.launch({ args: ['--use-gl=swiftshader'] });
try {
  // reset state backend — run sebelumnya bisa meninggalkan surge/rain aktif
  await fetch('http://127.0.0.1:3013/control/surge', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"factor":1}' });
  await fetch('http://127.0.0.1:3013/control/weather', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"factor":1}' });
  await new Promise((r) => setTimeout(r, 1200));

  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  page.on('pageerror', (e) => consoleErrors.push(String(e)));

  await page.goto(BASE, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(
    () => document.body.innerText.includes('LIVE LINK'),
    null, { timeout: 20000 },
  );
  log('mode', 'LIVE');

  // fps rAF 4 detik (canvas loop + render React) — baseline surge ×1
  const fps = await page.evaluate(() => new Promise((res) => {
    let n = 0;
    const t0 = performance.now();
    const tick = () => {
      n++;
      if (performance.now() - t0 < 4000) requestAnimationFrame(tick);
      else res(n / ((performance.now() - t0) / 1000));
    };
    requestAnimationFrame(tick);
  }));
  log('fps_rAF_avg(4s)', fps.toFixed(1));

  const effect = () => page.locator('aside[aria-label="Surge Console"] p').last().innerText();

  // reaksi FLASH SALE ×8 → ECHO backend (EFFECT ×8.0 DEMAND dari snapshot)
  const prevEffect = await effect();
  const t0 = Date.now();
  await page.getByRole('button', { name: 'FLASH SALE' }).click();
  await page.waitForFunction(
    (prev) => {
      const el = document.querySelector('aside[aria-label="Surge Console"] p:last-of-type');
      return el && el.textContent !== prev && el.textContent.includes('×8.0 DEMAND');
    }, prevEffect, { timeout: 5000 },
  );
  log('flash_sale_echo_ms', Date.now() - t0);

  // reaksi RAIN → ECHO backend (EFFECT ×0.60 SPEED dari snapshot)
  const prevEffect2 = await effect();
  const t1 = Date.now();
  await page.getByRole('button', { name: 'RAIN' }).click();
  await page.waitForFunction(
    (prev) => {
      const el = document.querySelector('aside[aria-label="Surge Console"] p:last-of-type');
      return el && el.textContent !== prev && el.textContent.includes('×0.60 SPEED');
    }, prevEffect2, { timeout: 5000 },
  );
  log('rain_echo_ms', Date.now() - t1);

  // fps saat surge ×8 + hujan (beban terberat)
  await page.waitForTimeout(3000);
  const fpsHeavy = await page.evaluate(() => new Promise((res) => {
    let n = 0;
    const t0 = performance.now();
    const tick = () => {
      n++;
      if (performance.now() - t0 < 4000) requestAnimationFrame(tick);
      else res(n / ((performance.now() - t0) / 1000));
    };
    requestAnimationFrame(tick);
  }));
  log('fps_rAF_surge8_rain', fpsHeavy.toFixed(1));

  await page.screenshot({ path: '/tmp/kilo/surge-live.png' });

  // slider manual ×3.5: readout instan + echo backend
  await page.getByRole('button', { name: 'FLASH SALE' }).click(); // off → kembali nilai tersimpan
  await page.waitForTimeout(800);
  const t2 = Date.now();
  await page.locator('input[aria-label="Faktor surge"]').fill('3.5');
  await page.locator('input[aria-label="Faktor surge"]').dispatchEvent('pointerup');
  await page.waitForFunction(() => {
    const el = document.querySelector('aside[aria-label="Surge Console"] p:last-of-type');
    return el && el.textContent.includes('×3.5 DEMAND');
  }, null, { timeout: 5000 });
  log('slider_echo_ms', Date.now() - t2);

  // pulihkan: surge ×1, weather cerah
  await page.getByRole('button', { name: 'RAIN' }).click();
  await page.waitForTimeout(600);
  await page.locator('input[aria-label="Faktor surge"]').fill('1');
  await page.locator('input[aria-label="Faktor surge"]').dispatchEvent('pointerup');
  await page.waitForTimeout(600);

  // reduced motion: halaman tetap hidup tanpa error
  const ctx2 = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1440, height: 900 } });
  const p2 = await ctx2.newPage();
  p2.on('pageerror', (e) => consoleErrors.push('RM: ' + String(e)));
  await p2.goto(BASE, { waitUntil: 'domcontentloaded' });
  await p2.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 20000 });
  log('reduced_motion', 'halaman LIVE tanpa error');
  await ctx2.close();

  log('console_errors', consoleErrors.length === 0 ? '0' : JSON.stringify(consoleErrors));
  await page.screenshot({ path: '/tmp/kilo/surge-final.png' });
} finally {
  await browser.close();
}
