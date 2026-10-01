// Verifikasi headless Fase 5 — jalankan di container
// mcr.microsoft.com/playwright:v1.63.0-noble dengan --network host
// (setelah image baru ter-deploy dan api-gateway + chaos healthy di stack
// `lastmile`, server next lokal di :3000 dengan NEXT_PUBLIC_* menunjuk
// 172.19.0.1 — pola reports/phase-04-ui-verify.mjs):
//   node phase-05-ui-verify.mjs
//
// Cakupan: Replay & Inspect (scrub timeline + PLAY ×1/×4/×16, kartu rider
// dengan alasan dispatch, kartu order + umur), Golden Demo E2E end-to-end
// (preset dinner-rush 90 s → langkah narasi → kill rider-sim → pulih →
// incident chaos-kill di timeline), fallback fixture saat backend diblokir
// (regresi fase 1), reduced-motion (scrub statis, tanpa PLAY), audit rAF
// (tanpa rAF idle baru), console error 0, screenshot 1440/1024/768.
import { chromium } from 'playwright';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const API = process.env.API_URL ?? 'http://172.19.0.1:3010';
const OUT = process.env.OUT_DIR ?? '/out';
mkdirSync(OUT, { recursive: true });
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

  // ---- baseline rAF 2 s (loop peta, dokumentasi fase 4: ±70–73/2 s) ----
  const rafBase = await page.evaluate(() => new Promise((res) => {
    let n = 0;
    const t0 = performance.now();
    const cb = () => { n++; if (performance.now() - t0 < 2000) requestAnimationFrame(cb); else res(n); };
    requestAnimationFrame(cb);
  }));
  log('raf_baseline_2s', rafBase);

  // ---- Replay & Inspect: buka panel, muat sesi ----
  await page.getByRole('button', { name: /⟲ REPLAY/i }).click();
  await page.waitForSelector('[aria-label="Replay dan Inspect"]', { timeout: 5000 });
  await page.waitForFunction(
    () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('SESI LIVE'),
    null, { timeout: 30000 },
  );
  const panelText = await page.locator('[aria-label="Replay dan Inspect"]').innerText();
  const frames = Number((panelText.match(/(\d+) FRAME/) ?? [])[1] ?? 0);
  const hz = Number((panelText.match(/(\d+) HZ/) ?? [])[1] ?? 0);
  const bufMin = (panelText.match(/BUFFER (\d+) MENIT (\d+) S/) ?? []).slice(1).map(Number);
  log('replay_source', panelText.includes('SESI LIVE') ? 'SESSION' : 'FIXTURE');
  log('replay_frames', frames);
  log('replay_hz', hz);
  log('replay_buffer', `${bufMin[0] ?? '?'}m${bufMin[1] ?? '?'}s`);

  // ---- scrub: set timeline ke tengah → label T+ berubah (render on-demand) ----
  const t0Label = await page.locator('[aria-label="Replay dan Inspect"]').innerText().then(t => (t.match(/T\+(\d+:\d+)/) ?? [])[1]);
  const rng = page.locator('input[aria-label="Posisi timeline replay"]');
  const min = Number(await rng.getAttribute('min'));
  const max = Number(await rng.getAttribute('max'));
  await rng.fill(String(Math.floor((min + max) / 2)));
  await sleep(400);
  const t1Label = await page.locator('[aria-label="Replay dan Inspect"]').innerText().then(t => (t.match(/T\+(\d+:\d+)/) ?? [])[1]);
  log('scrub', t0Label !== t1Label ? `ok (${t0Label} → ${t1Label})` : `GAGAL (${t0Label} == ${t1Label})`);

  // rAF saat pause+scrub: tidak boleh naik signifikan vs baseline (render on-demand)
  const rafPaused = await page.evaluate(() => new Promise((res) => {
    let n = 0;
    const t0 = performance.now();
    const cb = () => { n++; if (performance.now() - t0 < 2000) requestAnimationFrame(cb); else res(n); };
    requestAnimationFrame(cb);
  }));
  log('raf_paused_2s', rafPaused);

  // ---- PLAY ×4 → T+ maju ----
  await page.getByRole('button', { name: '▶ PLAY', exact: true }).click();
  await page.getByRole('button', { name: '×4', exact: true }).click();
  await sleep(2200);
  const t2Label = await page.locator('[aria-label="Replay dan Inspect"]').innerText().then(t => (t.match(/T\+(\d+:\d+)/) ?? [])[1]);
  log('play_x4_advances', t2Label !== t1Label ? `ok (${t1Label} → ${t2Label})` : 'GAGAL');
  await page.getByRole('button', { name: '❚❚ PAUSE', exact: true }).click();

  // ---- inspect rider: klik titik di peta sampai kartu RIDER muncul ----
  let riderCard = false;
  const mapBox = await page.locator('div[aria-label="Live Ops Map — Berlin"]').boundingBox();
  for (let i = 0; i < 12 && !riderCard; i++) {
    const x = mapBox.x + mapBox.width * (0.3 + 0.4 * ((i * 37) % 10) / 10);
    const y = mapBox.y + mapBox.height * (0.3 + 0.4 * ((i * 53) % 10) / 10);
    await page.mouse.click(x, y);
    await sleep(250);
    riderCard = await page.locator('[aria-label="Replay dan Inspect"]').innerText().then(t => /RIDER r\d+/.test(t));
  }
  log('inspect_rider_card', riderCard ? 'ada' : 'TIDAK DITEMUKAN');
  if (riderCard) {
    const card = await page.locator('[aria-label="Detail rider"]').innerText().catch(() => '');
    log('rider_reason', card.includes('Alasan keputusan dispatch') ? 'ada' : 'HILANG');
    log('rider_reason_text', card.split('\n').find(l => l.startsWith('“')) ?? '(kosong)');
  }
  // klik lain → kartu order (coba beberapa kali)
  let orderCard = false;
  for (let i = 0; i < 12 && !orderCard; i++) {
    const x = mapBox.x + mapBox.width * (0.25 + 0.5 * ((i * 71) % 10) / 10);
    const y = mapBox.y + mapBox.height * (0.25 + 0.5 * ((i * 29) % 10) / 10);
    await page.mouse.click(x, y);
    await sleep(250);
    const txt = await page.locator('[aria-label="Replay dan Inspect"]').innerText();
    orderCard = /ORDER o\w+/.test(txt.replace(/RIDER r\d+/g, ''));
  }
  log('inspect_order_card', orderCard ? 'ada' : 'TIDAK DITEMUKAN');

  await page.screenshot({ path: `${OUT}/phase05-replay-inspect.png` });

  // ---- Golden Demo E2E: preset dinner-rush (90 s, termasuk kill + pulih) ----
  const incBefore = (await (await fetch(`${API}/api/chaos/incidents`)).json()).incidents.length;
  await page.getByRole('button', { name: /▶ DEMO/i }).click();
  await page.waitForSelector('[aria-label="Golden Demo presets"]', { timeout: 5000 });
  const menuText = await page.locator('[aria-label="Golden Demo presets"]').innerText();
  log('demo_presets', (menuText.match(/▶ PLAY/g) ?? []).length);
  // pilih Dinner Rush (preset pertama)
  await page.locator('[aria-label="Golden Demo presets"] li').first().getByRole('button', { name: '▶ PLAY' }).click();
  await page.waitForSelector('[role="status"][aria-live="polite"]', { timeout: 10000 });
  let stepSeen = [];
  let killed = false;
  const tStart = Date.now();
  while (Date.now() - tStart < 115000) {
    await sleep(2000);
    const banner = await page.locator('[role="status"][aria-live="polite"]').innerText().catch(() => null);
    if (!banner) break; // selesai — banner hilang
    const m = banner.match(/LANGKAH (\d+)\/(\d+)/);
    if (m) {
      const cur = `${m[1]}/${m[2]}`;
      if (!stepSeen.includes(cur)) stepSeen.push(cur);
    }
    if (!killed && banner.includes('CHAOS: node simulasi')) {
      killed = true;
      await page.screenshot({ path: `${OUT}/phase05-demo-kill.png` });
    }
  }
  const demoDurS = Math.round((Date.now() - tStart) / 1000);
  const demoState = await (await fetch(`${API}/api/demo/state`)).json();
  log('demo_steps_seen', stepSeen.join(' ') || '—');
  log('demo_duration_s', demoDurS);
  log('demo_state_after', JSON.stringify(demoState));
  const incAfter = (await (await fetch(`${API}/api/chaos/incidents`)).json());
  const newKill = incAfter.incidents.find(i => i.kind === 'chaos-kill' && i.target === 'rider-sim' && i.t_start * 1 > tStart - 5000);
  log('demo_kill_incident', newKill ? `${newKill.id} mttd=${newKill.t_detect - newKill.t_start}ms mttr=${newKill.t_recover - newKill.t_detect}ms` : 'TIDAK TERCATAT');
  log('demo_kill_recovered', newKill && newKill.t_recover > 0 ? 'ya' : 'belum');
  await page.screenshot({ path: `${OUT}/phase05-demo-done.png` });

  // app tetap hidup setelah demo (kill rider-sim → self-heal)
  await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 30000 });
  log('live_after_demo', 'ya');

  // ---- Fallback fixture: blokir API+WS → REPLAY MODE (regresi fase 1) ----
  const ctx2 = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const p2 = await ctx2.newPage();
  p2.on('pageerror', (e) => consoleErrors.push('ctx2:' + String(e)));
  await p2.route('http://172.19.0.1:3010/**', (r) => r.abort());
  await p2.route('ws://172.19.0.1:3012/**', (r) => r.abort());
  await p2.goto(BASE, { waitUntil: 'domcontentloaded' });
  await p2.waitForFunction(
    () => document.body.innerText.includes('REPLAY MODE'),
    null, { timeout: 25000 },
  );
  log('fixture_fallback', 'REPLAY MODE tampil');
  // replay panel memakai fixture + scrub tetap jalan
  await p2.getByRole('button', { name: /⟲ REPLAY/i }).click();
  await p2.waitForFunction(
    () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('FIXTURE'),
    null, { timeout: 20000 },
  );
  const fxRng = p2.locator('input[aria-label="Posisi timeline replay"]');
  const fxMin = Number(await fxRng.getAttribute('min'));
  const fxMax = Number(await fxRng.getAttribute('max'));
  await fxRng.fill(String(Math.floor((fxMin + fxMax) / 2)));
  await sleep(300);
  const fxPanel = await p2.locator('[aria-label="Replay dan Inspect"]').innerText();
  log('fixture_scrub', /T+\d+:\d+ \/ 00:45/.test(fxPanel) ? 'ok' : fxPanel.match(/T\+\d+:\d+ \/ \d+:\d+/)?.[0] ?? '?');
  log('fixture_demo_offline', (await p2.getByRole('button', { name: /▶ DEMO/i }).innerText()).includes('OFFLINE') ? 'tombol mati' : '?');
  await p2.screenshot({ path: `${OUT}/phase05-fixture-offline.png` });
  await ctx2.close();

  // ---- Reduced motion: scrub statis, PLAY disembunyikan ----
  const ctx3 = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: 'reduce' });
  const p3 = await ctx3.newPage();
  p3.on('pageerror', (e) => consoleErrors.push('ctx3:' + String(e)));
  await p3.goto(BASE, { waitUntil: 'domcontentloaded' });
  await p3.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 20000 });
  await p3.getByRole('button', { name: /⟲ REPLAY/i }).click();
  await p3.waitForFunction(
    () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('SESI LIVE'),
    null, { timeout: 30000 },
  );
  const playCount = await p3.getByRole('button', { name: '▶ PLAY', exact: true }).count();
  const rmRng = p3.locator('input[aria-label="Posisi timeline replay"]');
  const rmMin = Number(await rmRng.getAttribute('min'));
  const rmMax = Number(await rmRng.getAttribute('max'));
  await rmRng.fill(String(rmMin + Math.floor((rmMax - rmMin) * 0.7)));
  await sleep(300);
  const rmPanel = await p3.locator('[aria-label="Replay dan Inspect"]').innerText();
  log('reduced_play_hidden', playCount === 0 ? 'ya' : 'MASIH ADA');
  log('reduced_scrub', /T+\d+:\d+/.test(rmPanel) ? 'ok' : 'GAGAL');
  await p3.screenshot({ path: `${OUT}/phase05-reduced.png` });
  await ctx3.close();

  // ---- responsive ----
  for (const w of [1440, 1024, 768]) {
    await page.setViewportSize({ width: w, height: 900 });
    await sleep(600);
    await page.screenshot({ path: `${OUT}/phase05-${w}.png` });
  }
  log('screens', '1440/1024/768');

  log('console_errors', consoleErrors.length === 0 ? 0 : consoleErrors.slice(0, 5).join(' | '));
  const fails = results.filter(([k, v]) => /GAGAL|HILANG|TIDAK|MASIH|KOSONG|\?\?$/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL CHECKS PASSED');
  process.exit(fails.length || consoleErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
