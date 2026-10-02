#!/usr/bin/env node
/**
 * verify-all (fase 9) — satu command untuk memeriksa seluruh fungsi &
 * frontend secara otonom (Playwright headless), dua mode:
 *
 *   --target=prod   (default) https://lastmile-lab.ricothen.com — subset
 *                   read-only + probe surge (DIPULIHKAN ×1). Tidak ada
 *                   kill/duel/demo.
 *   --target=local  APP_URL (default http://127.0.0.1:3000, build ber-env
 *                   prod) + API_URL (default http://127.0.0.1:3010) —
 *                   penuh: termasuk chaos kill (pulih sendiri), duel lab,
 *                   Golden Demo play→stop.
 *   --only=a,b      jalankan subset suite.
 *
 * Jalankan (pola repo):
 *   docker run --rm --network host -v /tmp/kilo/node_modules:/deps/node_modules \
 *     -v "$PWD:/deps/repo" -w /deps/repo mcr.microsoft.com/playwright:v1.63.0-noble \
 *     node scripts/verify-all.mjs --target=prod
 *
 * Bug historis yang menjadi check permanen: z-index overlay peta (sesi 13),
 * tinggi kontainer peta (fase 5), casing direktori glyph + filter 'has'
 * (fase 8), multi-member gzip replay (unit test backend, dirujuk).
 */
import { chromium } from 'playwright-core';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync, writeFileSync } from 'node:fs';

const args = process.argv.slice(2);
const arg = (k, d) => {
  const eq = args.find((a) => a.startsWith(`--${k}=`));
  if (eq) return eq.slice(k.length + 3);
  const i = args.indexOf(`--${k}`);
  return i >= 0 ? (args[i + 1]?.startsWith('--') ? true : args[i + 1]) : d;
};
const TARGET = arg('target', 'prod');
const ONLY = arg('only', null)?.split(',').filter(Boolean) ?? null;

const BASE = TARGET === 'local' ? process.env.APP_URL ?? 'http://127.0.0.1:3000' : 'https://lastmile-lab.ricothen.com';
// API selalu via domain publik (Caddy) — stack lastmile sengaja tidak
// dipublish ke host loopback (runbook §2), jadi 127.0.0.1:3010 tak terjangkau.
const API = process.env.API_URL ?? 'https://api.lastmile-lab.ricothen.com';
const OUT = process.env.OUT_DIR ?? `reports/verify-${new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)}`;
mkdirSync(OUT, { recursive: true });

const results = [];
const consoleErrors = [];
const shots = [];
let page;
let context;
let browser;

const log = (k, v, ok) => {
  results.push([k, v, ok]);
  console.log(`${ok === true ? '✓' : ok === false ? '✗' : '·'} ${k}: ${v}`);
};
const ok = (k, v) => log(k, v, true);
const fail = (k, v) => log(k, v, false);
const info = (k, v) => log(k, v, null);
const shot = async (name) => {
  const p = `${OUT}/${name}.png`;
  await page.screenshot({ path: p });
  shots.push(p);
};

const post = (path, body) =>
  page.evaluate(
    async ([api, path, body]) => {
      const r = await fetch(`${api}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      return { status: r.status, data: await r.json().catch(() => ({})) };
    },
    [API, path, body],
  );
const get = (path) =>
  page.evaluate(async ([api, path]) => {
    const r = await fetch(`${api}${path}`, { cache: 'no-store' });
    return { status: r.status, data: await r.json().catch(() => ({})) };
  }, [API, path]);

async function openApp(opts = {}) {
  context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    ...opts,
  });
  page = await context.newPage();
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e}`));
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text());
  });
  page.on('response', (r) => {
    if (r.status() === 404) consoleErrors.push(`404: ${r.url()}`);
  });
  await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 }).then(
    (r) => info('open.goto', `HTTP ${r?.status()} ${page.url()}`),
    (e) => fail('open.goto', String(e).split('\n')[0]),
  );
  // host bisa load 8+ — hydrate bisa lambat; tunggu marker render dulu,
  // lalu mode koneksi (LIVE/REPLAY) dan frame pertama peta.
  try {
    await page.waitForFunction(() => document.body.innerText.includes('PULSE'), null, { timeout: 120000 });
  } catch {
    const txt = await page.evaluate(() => document.body.innerText.slice(0, 300)).catch(() => 'EVAL FAIL');
    fail('open.hydrate', `innerText: ${txt.replace(/\n/g, ' | ')}`);
    throw new Error('app tidak ter-render (lihat open.hydrate)');
  }
  await page.waitForFunction(
    () => /LIVE LINK|CONNECTING|REPLAY/i.test(document.body.innerText),
    null,
    { timeout: 60000 },
  );
  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  const closeGuide = page.locator('[aria-label="Close guide"]');
  if (await closeGuide.count()) await closeGuide.click().catch(() => {});
  // tunggu peta hidup + frame pertama tergambar
  await page.waitForFunction(
    () => window.__lmPick && window.__lmPick.riders.length > 0,
    null,
    { timeout: 90000 },
  );
  return page;
}

const clearRider = () =>
  page.evaluate(() => {
    const box = document
      .querySelector('[aria-label="Live Ops Map — Berlin"]')
      ?.getBoundingClientRect();
    if (!box || !window.__lmPick?.riders?.length) return null;
    const pts = [
      ...window.__lmPick.riders.map((r) => [r.x, r.y]),
      ...window.__lmPick.orders.map((o) => [o.x, o.y]),
    ];
    const ptsExcept = (p) => pts.filter(([px, py]) => px !== p[0] || py !== p[1]);
    const clear = (x, y, self) => {
      const el = document.elementFromPoint(box.x + x, box.y + y);
      return (
        !!el &&
        el.classList.contains('maplibregl-canvas') &&
        Math.min(...ptsExcept(self).map(([px, py]) => Math.hypot(px - x, py - y)), 1e9) > 24
      );
    };
    const rider =
      window.__lmPick.riders.find((r) => clear(r.x, r.y, [r.x, r.y])) ??
      window.__lmPick.riders[0];
    return { rider, box };
  });

// ---- suites ----
const suites = {};
const suite = (name, tags, fn) => (suites[name] = { tags, fn });

suite(
  'smoke',
  ['prod', 'local'],
  async () => {
    const mode = await page.evaluate(() => document.body.innerText.match(/LIVE LINK|CONNECTING/i)?.[0] ?? '');
    if (/LIVE LINK/i.test(mode)) ok('smoke.mode', 'LIVE');
    else info('smoke.mode', `fallback terpakai (${mode || '?'}) — app tetap hidup`);
    const health = await get('/healthz');
    log('smoke.healthz', `HTTP ${health.status}`, health.status === 200 ? true : false);
    await shot('smoke');
  },
);

suite(
  'map',
  ['prod', 'local'],
  async () => {
    const style = await page.evaluate(() => {
      const s = window.__lmMap.getStyle();
      return { glyphs: s.glyphs, layers: s.layers.map((l) => l.id) };
    });
    log(
      'map.glyphs_selfhosted',
      style.glyphs,
      style.glyphs === '/fonts/{fontstack}/{range}.pbf' ? true : false,
    );
    for (const l of ['roads-glow', 'roads-label-major', 'roads-label-minor']) {
      log(`map.layer_${l}`, style.layers.includes(l) ? 'ok' : 'MISSING', style.layers.includes(l));
    }
    await page.evaluate(() => window.__lmMap.setZoom(15));
    let labels = [];
    for (let i = 0; i < 30 && labels.length === 0; i++) {
      await sleep(600);
      labels = await page.evaluate(
        () =>
          window.__lmMap
            .queryRenderedFeatures({ layers: ['roads-label-major', 'roads-label-minor'] })
            .map((f) => f.properties?.n)
            .filter(Boolean),
      );
    }
    log('map.street_labels', labels.length ? `${labels.length} label, contoh "${labels[0]}"` : 'NONE', labels.length > 0);
    await shot('map-labels-z15');
    await page.evaluate(() => window.__lmMap.setZoom(12.15));
  },
);

suite(
  'map-interact',
  ['prod', 'local'],
  async () => {
    const pick = await clearRider();
    if (!pick) return fail('map.hover', 'tidak ada titik rider bersih');
    await page.mouse.move(pick.box.x + pick.rider.x, pick.box.y + pick.rider.y, { steps: 3 });
    await sleep(200);
    const tip = await page.evaluate(() => {
      const el = document.querySelector('[data-testid="map-tooltip"]');
      return el ? el.textContent : null;
    });
    log('map.hover_tooltip', tip?.includes(`RIDER r${pick.rider.id}`) ? `ok (RIDER r${pick.rider.id})` : 'GAGAL', tip?.includes(`RIDER r${pick.rider.id}`) ? true : false);
    const ms = await page.evaluate(() => window.__lmHoverMs);
    log('map.hover_hit_test_ms', `${ms?.toFixed?.(3)} ms`, typeof ms === 'number' && ms < 16);
    await page.mouse.click(pick.box.x + pick.rider.x, pick.box.y + pick.rider.y);
    await page.waitForSelector('[data-testid="live-inspect"]', { timeout: 5000 });
    const card = await page.evaluate(() => document.querySelector('[data-testid="live-inspect"]')?.textContent ?? '');
    log('map.live_inspect', card.includes(`RIDER r${pick.rider.id}`) ? 'ok' : 'GAGAL', card.includes(`RIDER r${pick.rider.id}`));
    await shot('map-inspect');
    await page.keyboard.press('Escape');
    await sleep(200);
    const gone = (await page.$('[data-testid="live-inspect"]')) === null;
    log('map.inspect_escape_close', gone ? 'ok' : 'MASIH TERBUKA', gone);
  },
);

suite(
  'heat',
  ['prod', 'local'],
  async () => {
    const h0 = await page.evaluate(() => window.__lmHeat);
    log('heat.probe', h0 ? `cells=${h0.cells} max=${h0.max} total=${h0.total} su=${h0.su}` : 'MISSING', !!h0);
    const r = await post('/api/control/surge', { factor: 4 });
    if (r.status !== 200) return fail('heat.surge_echo', `POST HTTP ${r.status}`);
    let h1 = h0;
    for (let i = 0; i < 20; i++) {
      await sleep(500);
      h1 = await page.evaluate(() => window.__lmHeat);
      if (h1?.su >= 3.5) break;
    }
    log('heat.surge_echo', `su=${h1?.su}`, h1?.su >= 3.5);
    const grew = (h1?.alphaSum ?? 0) > (h0?.alphaSum ?? 0) * 1.3;
    log('heat.intensifies', `alphaSum ${h0?.alphaSum}→${h1?.alphaSum}`, grew);
    await shot('heat-surge4');
    const back = await post('/api/control/surge', { factor: 1 });
    log('heat.surge_restored', back.status === 200 ? 'ok (×1)' : `GAGAL HTTP ${back.status}`, back.status === 200);
  },
);

suite(
  'bursts',
  ['prod', 'local'],
  async () => {
    try {
      await page.waitForFunction(
        () => window.__lmBursts && window.__lmBursts.delivered > 0,
        null,
        { timeout: 40000 },
      );
    } catch {
      /* jatuh ke bawah */
    }
    const b = await page.evaluate(() => window.__lmBursts);
    log('bursts.counters', b ? `delivered=${b.delivered} expired=${b.expired}` : 'MISSING', !!b && b.delivered > 0);
  },
);

suite(
  'kpi',
  ['prod', 'local'],
  async () => {
    const kpi = await get('/api/kpi');
    log('kpi.endpoint', `HTTP ${kpi.status}`, kpi.status === 200);
    const hasNumbers = kpi.data?.orders_per_min != null || kpi.data?.sim != null;
    info('kpi.shape', Object.keys(kpi.data ?? {}).slice(0, 6).join(',') || 'empty');
    log('kpi.has_metrics', hasNumbers ? 'ok' : `shape tak dikenal: ${JSON.stringify(kpi.data).slice(0, 120)}`, hasNumbers);
  },
);

suite(
  'replay',
  ['prod', 'local'],
  async () => {
    await page.getByRole('button', { name: /REPLAY/i }).click();
    await page.waitForFunction(() => /replay & inspect/i.test(document.body.innerText), null, { timeout: 30000 });
    await page.waitForFunction(
      () => /live session|fixture \(offline\)/i.test(document.body.innerText),
      null,
      { timeout: 90000 },
    );
    const src = await page.evaluate(() =>
      /live session/i.test(document.body.innerText) ? 'LIVE SESSION' : 'FIXTURE (offline)',
    );
    info('replay.source', src);
    await sleep(1000);
    const pick = await clearRider();
    if (pick) {
      await page.mouse.click(pick.box.x + pick.rider.x, pick.box.y + pick.rider.y);
      await sleep(600);
      const shown = await page.evaluate(
        () =>
          /dispatch decision reason/i.test(document.body.innerText) ||
          !!document.querySelector('[aria-label="Detail order"]'),
      );
      log('replay.inspect_card', shown ? 'ok' : 'GAGAL', shown);
    } else {
      info('replay.inspect_card', 'SKIP — tidak ada titik bersih');
    }
    await shot('replay');
    await page.locator('[aria-label="Replay dan Inspect"] button', { hasText: '✕' }).click().catch(() => {});
  },
);

suite(
  'offline-fallback',
  ['prod', 'local'],
  async () => {
    // blokir api/ws (route interception) → app wajib jatuh ke REPLAY MODE
    await page.route(/api\.lastmile-lab\.ricothen\.com/, (route) => route.abort());
    await page.routeWebSocket(/ws\.lastmile-lab\.ricothen\.com/, (ws) => ws.close());
    if (TARGET === 'local') {
      await page.route(/127\.0\.0\.1:301[02]/, (route) => route.abort());
      await page.routeWebSocket(/127\.0\.0\.1:3012/, (ws) => ws.close());
    }
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForFunction(() => /REPLAY MODE/i.test(document.body.innerText), null, { timeout: 60000 });
    ok('offline.replay_banner', 'REPLAY MODE tampil tanpa backend');
    await shot('offline-replay');
    await page.unrouteAll({ behavior: 'ignoreErrors' });
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForFunction(() => /LIVE LINK|REPLAY/i.test(document.body.innerText), null, { timeout: 60000 });
    ok('offline.recovery', 'kembali hidup setelah unblock');
  },
);

suite(
  'interview',
  ['prod', 'local'],
  async () => {
    const resp = await page.goto(`${BASE}/interview`, { waitUntil: 'domcontentloaded', timeout: 45000 });
    info('interview.http', `HTTP ${resp?.status()} @ ${page.url()}`);
    const count = await page.evaluate(() => document.querySelectorAll('details').length);
    const cats = await page.evaluate(() => document.querySelectorAll('section h2').length);
    log('interview.qa_count', `${count} Q&A / ${cats} kategori`, count >= 25 && cats >= 8);
    const evCount = await page.evaluate(() =>
      Array.from(document.querySelectorAll('details p')).filter((p) => /^evidence:/i.test(p.textContent ?? '')).length,
    );
    log('interview.evidence_labels', `${evCount} sitasi`, evCount === count);
    await shot('interview');
  },
);

suite(
  'copilot',
  ['prod', 'local'],
  async () => {
    await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
    await page.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
    const caps = await get('/api/copilot/capabilities').catch(() => ({ status: 0, data: {} }));
    const enabled = caps.status === 200 && caps.data?.enabled === true;
    const nodes = await page.evaluate(
      () => document.body.innerText.match(/ADVISOR|COPILOT/gi)?.length ?? 0,
    );
    if (enabled) info('copilot.enabled', `panel tampil (${nodes} label di DOM) — key aktif`);
    else log('copilot.hidden_when_disabled', `caps=${caps.status}/${JSON.stringify(caps.data)} nodes=${nodes}`, !enabled ? nodes === 0 : true);
  },
);

suite(
  'a11y-motion',
  ['prod', 'local'],
  async () => {
    // ganti konteks aktif: tutup konteks biasa, reduced-motion jadi halaman utama
    await context.close().catch(() => {});
    context = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      reducedMotion: 'reduce',
    });
    page = await context.newPage();
    page.on('pageerror', (e) => consoleErrors.push(`RM pageerror: ${e}`));
    page.on('response', (r) => {
      if (r.status() === 404) consoleErrors.push(`RM 404: ${r.url()}`);
    });
    await page.goto(BASE, { waitUntil: 'domcontentloaded', timeout: 45000 });
    await page.waitForFunction(() => !!window.__lmMap, null, { timeout: 30000 });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    if (await page.locator('[aria-label="Close guide"]').count()) {
      await page.locator('[aria-label="Close guide"]').click().catch(() => {});
    }
    await page.waitForFunction(() => window.__lmPick && window.__lmPick.riders.length > 0, null, { timeout: 90000 });
    const heat = await page.evaluate(() => window.__lmHeat);
    log('a11y.reduced_heat_flag', heat ? `reduced=${heat.reduced}` : 'MISSING', heat?.reduced === true);
    const pick = await page.evaluate(() => {
      const box = document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect();
      const clear = (x, y) => document.elementFromPoint(box.x + x, box.y + y)?.classList.contains('maplibregl-canvas');
      return window.__lmPick.riders.find((r) => clear(r.x, r.y)) ?? window.__lmPick.riders[0];
    });
    const box2 = await page.evaluate(() => document.querySelector('[aria-label="Live Ops Map — Berlin"]').getBoundingClientRect());
    await page.mouse.move(box2.x + pick.x, box2.y + pick.y, { steps: 3 });
    await sleep(200);
    const tip = await page.evaluate(() => !!document.querySelector('[data-testid="map-tooltip"]'));
    log('a11y.reduced_tooltip', tip ? 'ok — interaksi tetap bekerja' : 'GAGAL', tip);
    await shot('reduced-motion');
  },
);

suite(
  'perf',
  ['prod', 'local'],
  async () => {
    const raf = await page.evaluate(
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
    log('perf.raf_idle_2s', `${raf}`, raf < 200);
  },
);

suite(
  'console',
  ['prod', 'local'],
  async () => {
    const app = consoleErrors.filter(
      (e) => !/ERR_CONNECTION_REFUSED|WebSocket|Failed to fetch|net::|Load failed/i.test(e),
    );
    log('console.errors', app.length ? app.slice(0, 4).join(' | ') : '0', app.length === 0);
  },
);

// ---- khusus local: mutasi penuh ----
suite(
  'chaos-local',
  ['local'],
  async () => {
    const gridBefore = await get('/api/kpi');
    const name = (gridBefore.data?.grid ? Object.values(gridBefore.data.grid) : []).find((g) => (g.name ?? '').includes('strategy-lab'))?.name;
    const kill = await post('/api/chaos/kill', { target: name ?? target });
    info('chaos.kill_accepted', `${name ?? target} → HTTP ${kill.status}`);
    let healed = false;
    const t0 = Date.now();
    for (let i = 0; i < 45 && !healed; i++) {
      await sleep(1000);
      const kpi = await get('/api/kpi');
      const node = Object.values(kpi.data?.grid ?? {}).find((g) => g.name === (name ?? 'strategy-lab'));
      healed = node?.status === 'up';
    }
    const mttrS = ((Date.now() - t0) / 1000).toFixed(1);
    // gerbang kebenaran: node pulih sendiri (restart policy), bukan kode HTTP
    log('chaos.self_heal', healed ? `pulih ${mttrS} s` : `belum 'up' dalam 45 s`, healed);
    await shot('chaos-healed');
  },
);

suite(
  'lab-local',
  ['local'],
  async () => {
    const run = await post('/api/lab/run', { strategy_a: 'fifo', strategy_b: 'optimal', preset: 'steady', seconds: 60 });
    // id pada body = duel diterima (proxy bisa saja 502 saat servis sibuk —
    // gerbang kebenaran = duel selesai di bawah)
    log('lab.run_accepted', run.data?.id ? `id=${run.data.id}` : `HTTP ${run.status} tanpa id`, !!run.data?.id || run.status === 200 || run.status === 409);
    if (run.data?.id) {
      let done = null;
      for (let i = 0; i < 60; i++) {
        await sleep(3000);
        const res = await get(`/api/lab/results/${run.data.id}`);
        if (res.data?.status && res.data.status !== 'running') {
          done = res.data;
          break;
        }
      }
      log('lab.duel_done', done ? `status=${done.status} delivered a/b=${done.delivered_a ?? '—'}/${done.delivered_b ?? '—'}` : 'timeout 180 s', done?.status === 'done');
    }
  },
);

suite(
  'demo-local',
  ['local'],
  async () => {
    const presets = await get('/api/demo/presets');
    const first = presets.data?.presets?.[0];
    log('demo.presets', presets.data?.presets ? `${presets.data.presets.length} preset` : 'MISSING', presets.status === 200 && !!first);
    if (!first) return;
    const play = await post('/api/demo/play', { id: first.id });
    log('demo.play', `HTTP ${play.status}`, play.status === 200);
    await sleep(12000);
    const st = await get('/api/demo/state');
    info('demo.state', `active=${st.data?.active} step=${st.data?.step ?? '—'}/${st.data?.total_steps ?? '—'} "${st.data?.label ?? ''}"`);
    const stop = await post('/api/demo/stop', {});
    log('demo.stop', `HTTP ${stop.status}`, stop.status === 200 || stop.status === 409);
  },
);

// ---- runner ----
const run = async () => {
  browser = await chromium.launch({ args: ['--use-gl=swiftshader'] });
  await openApp();
  const names = Object.keys(suites).filter(
    (n) => (!ONLY || ONLY.includes(n)) && suites[n].tags.includes(TARGET),
  );
  // lab sebelum chaos: duel butuh strategy-lab hidup; chaos terakhir agar
  // gangguan tidak merusak suite lain.
  const order = ['lab-local', 'chaos-local'];
  names.sort((a, b) => (order.indexOf(a) + 1 || 99) - (order.indexOf(b) + 1 || 99));
  for (const name of names) {
    console.log(`\n── suite: ${name} ──`);
    try {
      await suites[name].fn();
    } catch (e) {
      fail(`${name}.exception`, String(e).split('\n')[0]);
    }
  }
  await context.close().catch(() => {});
  await browser.close();

  const fails = results.filter(([, , okv]) => okv === false);
  const pass = results.filter(([, , okv]) => okv === true).length;
  console.log(`\n==== VERIFY-ALL (${TARGET}) ====`);
  console.log(`pass: ${pass} · fail: ${fails.length} · info: ${results.length - pass - fails.length}`);
  if (fails.length) console.log(`FAILED:\n${fails.map(([k, v]) => `  ✗ ${k}: ${v}`).join('\n')}`);
  writeFileSync(
    `${OUT}/summary.json`,
    JSON.stringify({ target: TARGET, base: BASE, pass, fails: fails.length, results }, null, 2),
  );
  console.log(`artifacts: ${OUT} (${shots.length} screenshot)`);
  process.exit(fails.length || consoleErrors.length ? 1 : 0);
};

run().catch((e) => {
  console.error('FATAL', e);
  process.exit(2);
});
