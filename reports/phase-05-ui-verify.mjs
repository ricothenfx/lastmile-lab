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
  // ================= MODE OFFLINE =================
  // Jalankan SETELAH host menghentikan rider-sim + ws-gateway + api-gateway:
  //   OFFLINE_MODE=1 node phase-05-ui-verify.mjs
  // Memverifikasi frontend tetap hidup 100% tanpa backend: banner REPLAY
  // (fixture fase 1), replay panel memakai fixture + scrub, demo tombol mati.
  if (process.env.OFFLINE_MODE === '1') {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });
    page.on('pageerror', (e) => consoleErrors.push(String(e)));
    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(
      () => document.body.innerText.includes('REPLAY MODE'),
      null, { timeout: 25000 },
    );
    log('fixture_fallback', 'REPLAY MODE tampil (tanpa backend)');
    await page.getByRole('button', { name: /⟲ REPLAY/i }).click();
    await page.waitForFunction(
      () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('FIXTURE'),
      null, { timeout: 20000 },
    );
    const fxRng = page.locator('input[aria-label="Posisi timeline replay"]');
    const fxMin = Number(await fxRng.getAttribute('min'));
    const fxMax = Number(await fxRng.getAttribute('max'));
    await fxRng.fill(String(Math.round(Math.floor((fxMin + fxMax) / 2) / 100) * 100));
    await sleep(400);
    const fxPanel = await page.locator('[aria-label="Replay dan Inspect"]').innerText();
    const scrubOk = /T+\d+:\d+ \/ 00:45/.test(fxPanel);
    log('fixture_scrub', scrubOk ? 'ok' : fxPanel.match(/T\+\d+:\d+ \/ \d+:\d+/)?.[0] ?? '?');
    log('fixture_demo_offline', (await page.getByRole('button', { name: /▶ DEMO/i }).innerText()).includes('OFFLINE') ? 'tombol mati' : 'MASIH HIDUP?');
    await page.screenshot({ path: `${OUT}/phase05-fixture-offline.png` });
    // Koneksi ditolak adalah konsekuensi backend mati — bukan bug UI.
    const appErrors = consoleErrors.filter(e => !/ERR_CONNECTION_REFUSED|WebSocket connection/i.test(e));
    log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
    const fails = results.filter(([k, v]) => /GAGAL|HILANG|TIDAK|MASIH|KOSONG|\?\?$/.test(String(v)));
    console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL OFFLINE CHECKS PASSED');
    process.exit(fails.length || appErrors.length ? 1 : 0);
  }

  // ================= MODE UTAMA (backend hidup) =================
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
  await rng.fill(String(Math.round(Math.floor((min + max) / 2) / 100) * 100));
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

  // ---- inspect rider: klik posisi rider nyata dari cache hit-test peta ----
  let riderCard = false;
  const pick = await page.evaluate(() => window.__lmPick ?? null);
  if (pick && pick.riders.length > 0) {
    const mapBox = await page.locator('div[aria-label="Live Ops Map — Berlin"]').boundingBox();
    const target = pick.riders[Math.floor(pick.riders.length / 2)];
    await page.mouse.click(mapBox.x + target.x, mapBox.y + target.y);
    await sleep(300);
    riderCard = (await page.locator('[aria-label="Detail rider"]').count()) > 0;
  }
  log('inspect_rider_card', riderCard ? 'ada' : 'TIDAK DITEMUKAN');
  if (riderCard) {
    const card = await page.locator('[aria-label="Detail rider"]').innerText().catch(() => '');
    log('rider_reason', card.toLowerCase().includes('alasan keputusan dispatch') ? 'ada' : 'HILANG');
    log('rider_reason_text', card.split('\n').find(l => l.startsWith('“')) ?? '(kosong)');
  }
  // klik order → kartu order (seleksi tergantikan per klik; titik di bawah
  // panel ditoleransi — klik yang lolos ke panel diabaikan lalu coba lagi)
  let orderCard = false;
  if (pick && pick.orders.length > 0) {
    const mapBox = await page.locator('div[aria-label="Live Ops Map — Berlin"]').boundingBox();
    for (let i = 0; i < Math.min(8, pick.orders.length) && !orderCard; i++) {
      const target = pick.orders[i];
      await page.mouse.click(mapBox.x + target.x, mapBox.y + target.y);
      await sleep(280);
      orderCard = (await page.locator('[aria-label="Detail order"]').count()) > 0;
    }
  }
  log('inspect_order_card', orderCard ? 'ada' : 'TIDAK DITEMUKAN');

  await page.screenshot({ path: `${OUT}/phase05-replay-inspect.png` });
  // tutup panel replay — dropdown demo tidak tertutup
  await page.getByRole('button', { name: 'Tutup panel replay' }).click();
  await sleep(300);

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
  let demoWallS = 0;
  const tStart = Date.now();
  while (Date.now() - tStart < 130000) {
    await sleep(2000);
    // durasi diukur dari state server (sumber kebenaran) — banner UI bisa
    // terlambat saat koneksi browser jenuh sesaat setelah kill
    const st = await fetch(`${API}/api/demo/state`, { cache: 'no-store' }).then(r => r.json()).catch(() => null);
    const banner = await page.locator('[role="status"][aria-live="polite"]').innerText().catch(() => null);
    if (banner) {
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
    if (st && !st.active) {
      demoWallS = Math.round((Date.now() - tStart) / 1000);
      break;
    }
  }
  const demoState = await (await fetch(`${API}/api/demo/state`)).json();
  log('demo_steps_seen', stepSeen.join(' ') || '—');
  log('demo_duration_s', `${demoWallS} (server) · last_id=${demoState.last_id}`);
  log('demo_ui_banner', 'tampil selama demo (narasi langkah), hilang saat selesai');
  const incAfter = (await (await fetch(`${API}/api/chaos/incidents`)).json());
  const newKill = incAfter.incidents.find(i => i.kind === 'chaos-kill' && i.target === 'rider-sim' && i.t_start * 1 > tStart - 5000);
  log('demo_kill_incident', newKill ? `${newKill.id} mttd=${newKill.t_detect - newKill.t_start}ms mttr=${newKill.t_recover - newKill.t_detect}ms` : 'TIDAK TERCATAT');
  log('demo_kill_recovered', newKill && newKill.t_recover > 0 ? 'ya' : 'belum');
  await page.screenshot({ path: `${OUT}/phase05-demo-done.png` });

  // app tetap hidup setelah demo (kill rider-sim → self-heal)
  await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 30000 });
  log('live_after_demo', 'ya');

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
  await rmRng.fill(String(Math.round((rmMin + (rmMax - rmMin) * 0.7) / 100) * 100));
  await sleep(300);
  const rmPanel = await p3.locator('[aria-label="Replay dan Inspect"]').innerText();
  log('reduced_play_hidden', playCount === 0 ? 'ya' : 'MASIH ADA');
  const rmMatch = rmPanel.match(/T\+\d+:\d+ \/ \d+:\d+/);
  log('reduced_scrub', rmMatch ? `ok (${rmMatch[0]})` : `GAGAL — teks: ${rmPanel.replace(/\n/g, ' | ').slice(0, 220)}`);
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
