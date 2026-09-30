// Verifikasi headless Strategy Lab (Fase 3) — jalankan di container
// mcr.microsoft.com/playwright dengan --network host (setelah image baru
// ter-deploy dan strategy-lab healthy di stack `lastmile`):
//   node phase-03-ui-verify.mjs
// Cakupan: panel terbuka, duel end-to-end dari UI (RUN → polling → hasil),
// tabel delta + histogram + peta kembar ter-render (canvas non-blank),
// playback berhenti sendiri (tanpa rAF saat idle), reduced-motion statis,
// console error 0, screenshot.
import { chromium } from 'playwright';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const LAB = process.env.LAB_URL ?? 'http://127.0.0.1:3010/api/lab';
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

  // buka panel Strategy Lab
  await page.getByRole('button', { name: /Strategy Lab/i }).click();
  await page.waitForSelector('select[aria-label="Strategi A"]', { timeout: 5000 });
  log('panel', 'open');

  // pilih duel fifo vs optimal, preset rush, 120s (cepat untuk verifikasi)
  await page.selectOption('select[aria-label="Strategi A"]', 'fifo');
  await page.selectOption('select[aria-label="Strategi B"]', 'optimal');
  await page.selectOption('select[aria-label="Preset skenario"]', 'rush');
  await page.selectOption('select[aria-label="Durasi duel"]', '120');

  const t0 = Date.now();
  await page.getByRole('button', { name: 'RUN', exact: true }).click();
  log('run_accepted_ms', Date.now() - t0);

  // polling sampai tabel delta (hasil) muncul di panel — marker unik:
  // tabel hasil, BUKAN ticker TopBar (label "Delivered" di-uppercase CSS).
  await page.waitForSelector('aside[aria-label="Strategy Lab"] table', { timeout: 60000 });
  log('duel_done_total_ms', Date.now() - t0);

  // hasil duel via API sama-sama done (konsistensi UI ↔ backend); tunggu
  // tak ada duel berjalan agar tidak race dengan polling UI.
  await page.waitForFunction(
    async (url) => {
      const list = (await (await fetch(url + '/results')).json()).results ?? [];
      return list.length > 0 && list.every((s) => s.status !== 'running');
    }, LAB, { timeout: 30000, polling: 1000 },
  );
  const listRes = await fetch(`${LAB}/results`);
  const list = (await listRes.json()).results ?? [];
  log('api_results_count', list.length);
  log('api_latest', `${list[0]?.strategy_a}/${list[0]?.strategy_b} ${list[0]?.status}`);

  // peta kembar: tunggu canvas results terpasang, lalu cek piksel non-kosong
  await page.waitForFunction(
    () => document.querySelectorAll('aside[aria-label="Strategy Lab"] canvas').length >= 3,
    null, { timeout: 15000 },
  );
  const canvases = await page.$$eval('aside[aria-label="Strategy Lab"] canvas', (cs) =>
    cs.map((c) => {
      const ctx = c.getContext('2d');
      const data = ctx.getImageData(0, 0, c.width, c.height).data;
      let painted = 0;
      for (let i = 3; i < data.length; i += 40) if (data[i] !== 0) painted++;
      return { w: c.width, h: c.height, painted };
    }),
  );
  log('canvas_count', canvases.length);
  log('canvas_painted', canvases.map((c) => `${c.w}x${c.h}:${c.painted > 0}`).join(','));
  if (canvases.length < 3) throw new Error('canvas kurang dari 3 (2 peta + 1 histogram)');

  // playback: PLAY → berjalan → berhenti sendiri di frame akhir (tanpa loop idle)
  const playBtn = page.getByRole('button', { name: '▶ PLAY' });
  if (await playBtn.count()) {
    await playBtn.click();
    await page.waitForTimeout(2500);
    const midLabel = await page.locator('aside[aria-label="Strategy Lab"] span', { hasText: /T\+\d+\/\d+S/ }).first().innerText();
    await page.waitForFunction(
      () => !document.body.innerText.includes('❚❚ PAUSE'),
      null, { timeout: 60000 },
    );
    log('playback', `berhenti sendiri (label tengah: ${midLabel})`);
  }

  // scrub manual + idle: tidak ada rAF berjalan saat tak ada playback
  await page.waitForTimeout(1500);
  const rafIdle = await page.evaluate(() => new Promise((res) => {
    let n = 0;
    const t0 = performance.now();
    const tick = () => {
      n++;
      if (performance.now() - t0 < 2000) requestAnimationFrame(tick);
      else res(n / ((performance.now() - t0) / 1000));
    };
    requestAnimationFrame(tick);
  }));
  log('raf_idle_hz_2s(=vsync host)', rafIdle.toFixed(1));

  await page.screenshot({ path: '/tmp/kilo/phase03-lab-result.png' });

  // export JSON tersedia (blob download — cukup cek tombolnya ada)
  log('export_button', (await page.getByRole('button', { name: /EXPORT JSON/ }).count()) === 1 ? 'ada' : 'HILANG');

  // reduced motion: muat duel tersimpan dari riwayat → hasil tetap tampil,
  // tombol PLAY disembunyikan (frame akhir statis, tanpa playback)
  const ctx2 = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1440, height: 900 } });
  const p2 = await ctx2.newPage();
  p2.on('pageerror', (e) => consoleErrors.push('RM: ' + String(e)));
  await p2.goto(BASE, { waitUntil: 'domcontentloaded' });
  await p2.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 20000 });
  await p2.getByRole('button', { name: /Strategy Lab/i }).click();
  const histSel = p2.locator('select[aria-label="Muat duel tersimpan"]');
  await histSel.waitFor({ state: 'visible', timeout: 10000 });
  const firstDone = await histSel.locator('option').nth(1).getAttribute('value');
  await histSel.selectOption(firstDone);
  await p2.waitForSelector('aside[aria-label="Strategy Lab"] table', { timeout: 15000 });
  const rmHasPlay = await p2.getByRole('button', { name: /PLAY/ }).count();
  const rmCanvases = await p2.$$eval('aside[aria-label="Strategy Lab"] canvas', (cs) => cs.length);
  log('reduced_motion', `hasil statis dimuat (canvas=${rmCanvases}), PLAY ${rmHasPlay === 0 ? 'TERSEMBUNYI' : 'MUNCUL'}`);
  await p2.screenshot({ path: '/out/phase03-lab-reduced.png' });
  await ctx2.close();

  log('console_errors', consoleErrors.length === 0 ? '0' : JSON.stringify(consoleErrors));
  await page.screenshot({ path: '/tmp/kilo/phase03-lab-final.png' });
} finally {
  await browser.close();
}
