// Verifikasi headless Fase 7 — AI Ops Copilot (ADR D24).
// Jalankan di mcr.microsoft.com/playwright:v1.63.0-noble --network host;
// app `next start :3000` dibangun dengan NEXT_PUBLIC_API_URL menunjuk
// gateway uji (kode baru, copilot OFF) → semua layar diuji terhadap stack
// demo yang hidup; jalur enabled diuji dengan ROUTE STUB (tanpa API key).
//
//   MODE=hidden node phase-07-ui-verify.mjs   # default — fitur 100% tersembunyi + regresi 6 layar + Golden Demo + replay
//   MODE=stub   node phase-07-ui-verify.mjs   # capabilities di-stub enabled — panel muncul + interaksi + sitasi
//   DEMO=0      # lewati Golden Demo 90 s (untuk iterasi cepat)
import { chromium } from 'playwright-core';
import { setTimeout as sleep } from 'node:timers/promises';
import { mkdirSync } from 'node:fs';

const BASE = process.env.APP_URL ?? 'http://127.0.0.1:3000';
const API = process.env.API_URL ?? 'http://172.20.0.8:3010';
const MODE = process.env.MODE ?? 'hidden';
const RUN_DEMO = process.env.DEMO !== '0';
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

  if (MODE === 'stub') {
    // ============ JALUR ENABLED VIA ROUTE STUB (tanpa API key) ============
    await page.route(/\/api\/copilot\/capabilities/, (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '{"enabled":true}' }));
    await page.route(/\/api\/copilot\/advise/, (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        seed: 77, seconds: 120, base_rate_per_min: 34,
        baseline: { created: 68, delivered: 52, expired: 16, delivery_p50_ms: 205000, delivery_p95_ms: 470000, cost_per_order_km: 1.9, utilization_pct: 62, throughput_per_min: 26 },
        plans: [
          { plan: { name: 'Ganti ke optimal', rationale: 'p95 470000 ms di atas SLO (kpi.sim.delivery_p95_ms); rush overload.', actions: [{ kind: 'strategy', params: { name: 'optimal' } }] },
            baseline: { created: 68, delivered: 52, expired: 16, delivery_p50_ms: 205000, delivery_p95_ms: 470000, cost_per_order_km: 1.9, utilization_pct: 62, throughput_per_min: 26 },
            predicted: { created: 68, delivered: 66, expired: 2, delivery_p50_ms: 148000, delivery_p95_ms: 301000, cost_per_order_km: 0.94, utilization_pct: 71, throughput_per_min: 33 } },
          { plan: { name: 'Chaos drill ws-gateway', rationale: 'latih self-heal (incidents.summary).', actions: [{ kind: 'kill', params: { target: 'lastmile-ws-gateway' } }] },
            baseline: null, predicted: null,
            note: '1) aksi kill "lastmile-ws-gateway" tidak tersimulasi dry-run (incident runtime) — nilai eksekusinya lihat Incident Timeline' },
        ],
      }) }));
    await page.route(/\/api\/control\/surge/, (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' }));
    let askReject = false;
    await page.route(/\/api\/copilot\/ask/, (route) => {
      if (askReject) {
        return route.fulfill({ status: 422, contentType: 'application/json', body: '{"error":"answer_rejected: jawaban tanpa sitasi ditolak"}' });
      }
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
        text: 'p95 delivery 470000 ms — di atas SLO 360 s (kpi.sim.delivery_p95_ms); ada 1 incident terbuka (incidents.summary).',
        sources: [
          { id: 'kpi.sim.delivery_p95_ms', label: 'Delivery p95 (ring live)', value: '470000 ms' },
          { id: 'incidents.summary', label: 'Ringkasan incident chaos', value: 'total=2 mttd_avg=800ms mttr_avg=2000ms' },
        ],
      }) });
    });

    await page.goto(BASE, { waitUntil: 'domcontentloaded' });
    await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 30000 });
    log('stub_app_live', 'ok');

    // ---- Strategy Lab: tab ADVISOR muncul ----
    await page.getByRole('button', { name: /Strategy Lab/i }).click();
    await page.waitForSelector('[role="tab"][aria-selected="true"]', { timeout: 5000 });
    const labTabs = await page.getByRole('tab', { name: 'ADVISOR' }).count();
    log('stub_advisor_tab', labTabs === 1 ? 'muncul' : 'HILANG');
    await page.getByRole('tab', { name: 'ADVISOR' }).click();
    await page.getByTestId('copilot-advisor').waitFor({ timeout: 5000 });
    await page.getByRole('button', { name: 'GENERATE PLANS' }).click();
    await page.waitForSelector('text=DRY-RUN 120s · SEED 77', { timeout: 5000 });
    const cardText = await page.locator('[data-testid="copilot-advisor"]').innerText();
    log('stub_plans_render', cardText.includes('Ganti ke optimal') && cardText.includes('Chaos drill') ? '2 plan' : 'GAGAL');
    log('stub_dryrun_delta', cardText.includes('BASE') && cardText.includes('PLAN') && /\+2?7%/.test(cardText) ? 'tabel Δ' : 'GAGAL');
    log('stub_kill_note', cardText.includes('tidak tersimulasi') ? 'jujur (tanpa angka palsu)' : 'GAGAL');
    // Execute 2 langkah (stub surge OK) — langkah 2 memakai aria-label arm
    await page.getByRole('button', { name: /Execute plan Ganti ke optimal/ }).click();
    const armed = await page.getByRole('button', { name: /klik lagi untuk konfirmasi/ }).count();
    await page.getByRole('button', { name: /klik lagi untuk konfirmasi/ }).click();
    await sleep(400);
    const afterExec = await page.locator('[data-testid="copilot-advisor"]').innerText();
    log('stub_execute_confirm', armed === 1 ? '2 langkah' : 'GAGAL');
    log('stub_execute_result', afterExec.includes('STRATEGY SKIP') || afterExec.includes('STRATEGY') ? 'dieksekusi via kontrol existing' : 'GAGAL');
    await page.screenshot({ path: `${OUT}/phase07-advisor-enabled.png` });

    // ---- System Health: tab COPILOT + Ask Ops + sitasi ----
    await page.getByRole('button', { name: /System Health/i }).click();
    await page.getByRole('tab', { name: 'COPILOT' }).click();
    await page.getByTestId('copilot-ask').waitFor({ timeout: 5000 });
    await page.getByLabel('Pertanyaan ops').fill('kenapa p95 naik?');
    await page.getByRole('button', { name: 'ASK', exact: true }).click();
    await page.waitForSelector('text=kpi.sim.delivery_p95_ms', { timeout: 5000 });
    const askText = await page.locator('[data-testid="copilot-ask"]').innerText();
    log('stub_ask_cited', askText.includes('SOURCES') && askText.includes('470000') ? 'jawaban + 2 sitasi' : 'GAGAL');
    // jawaban tanpa sitasi → ditolak
    askReject = true;
    await page.getByRole('button', { name: 'ASK', exact: true }).click();
    await page.waitForSelector('text=ANSWER_REJECTED', { timeout: 5000 });
    log('stub_ask_reject', 'tanpa sitasi ditolak (422)');
    await page.screenshot({ path: `${OUT}/phase07-ask-enabled.png` });

    // 422 = expected (uji penolakan jawaban tanpa sitasi sengaja memicunya)
    const appErrors = consoleErrors.filter(e => !/WebSocket connection|Failed to fetch|net::|status of 422/i.test(e));
    log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
    const fails = results.filter(([k, v]) => /GAGAL|HILANG/.test(String(v)));
    console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL STUB CHECKS PASSED');
    process.exit(fails.length || appErrors.length ? 1 : 0);
  }

  // ================= MODE HIDDEN (default): regresi tanpa key =================
  await page.goto(BASE, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => document.body.innerText.includes('LIVE LINK'), null, { timeout: 30000 });
  log('mode', 'LIVE (gateway uji, copilot OFF)');

  // 6 layar: Live Map (utama), KPI Deck, Surge Console, Strategy Lab, System Health, Replay.
  // 0) TIDAK ADA copilot di DOM sama sekali
  const copilotDom = await page.evaluate(() => ({
    nodes: document.querySelectorAll('[data-testid="copilot-advisor"], [data-testid="copilot-ask"]').length,
    text: /ADVISOR|COPILOT/i.test(document.body.innerText),
  }));
  log('hidden_no_copilot_dom', copilotDom.nodes === 0 && !copilotDom.text ? 'bersih (0 node)' : 'BOCOR');

  // 1) KPI Command Deck — angka nyata
  await page.waitForSelector('[aria-label="KPI Command Deck"]', { timeout: 10000 });
  log('screen_kpi_deck', 'tampil');
  const deckText = await page.locator('[aria-label="KPI Command Deck"]').innerText();
  log('kpi_numbers', /\d/.test(deckText) ? 'ada' : 'KOSONG');

  // 2) Surge Console
  const surgeVisible = await page.locator('[aria-label="Surge Console"]').count()
    && (await page.locator('[aria-label="Surge Console"]').innerText()).includes('SURGE');
  log('screen_surge', surgeVisible ? 'tampil' : 'HILANG');

  // 3) Strategy Lab — tanpa tab ADVISOR
  await page.getByRole('button', { name: /Strategy Lab/i }).click();
  await sleep(400);
  const labTabs = await page.getByRole('tablist', { name: 'Tab Strategy Lab' }).count();
  const duelForm = await page.getByLabel('Strategi A').count();
  log('screen_lab', duelForm === 1 ? 'form duel tampil' : 'HILANG');
  log('hidden_no_lab_tabs', labTabs === 0 ? 'DOM identik baseline' : 'BOCOR');
  await page.getByRole('button', { name: /Strategy Lab/i }).click(); // tutup
  await sleep(300);

  // 4) System Health — hanya 2 tab (PIPELINE/CHAOS), grid 2 kolom
  await page.getByRole('button', { name: /System Health/i }).click();
  await sleep(400);
  const healthTabs = await page.getByRole('tab').count();
  log('screen_health', healthTabs === 2 ? `tab ${healthTabs} (baseline)` : `BERUBAH (${healthTabs} tab)`);
  const copilotTab = await page.getByRole('tab', { name: 'COPILOT' }).count();
  log('hidden_no_copilot_tab', copilotTab === 0 ? 'tidak ada' : 'BOCOR');
  await page.screenshot({ path: `${OUT}/phase07-hidden-health.png` });
  await page.getByRole('button', { name: /System Health/i }).click();
  await sleep(300);

  // 5) Replay & Inspect
  await page.getByRole('button', { name: /⟲ REPLAY/i }).click();
  await page.waitForSelector('[aria-label="Replay dan Inspect"]', { timeout: 5000 });
  await page.waitForFunction(
    () => document.querySelector('[aria-label="Replay dan Inspect"]')?.innerText.includes('SESI'),
    null, { timeout: 30000 },
  );
  const replayText = await page.locator('[aria-label="Replay dan Inspect"]').innerText();
  log('screen_replay', replayText.includes('SESI') ? 'sesi tampil' : 'GAGAL');
  await page.getByRole('button', { name: 'Tutup panel replay' }).click();
  await sleep(300);

  // 6) Golden Demo — launcher + 3 preset (+ eksekusi dinner-rush 90 s bila DEMO=1)
  const presets = await page.evaluate(async (api) => {
    const r = await fetch(`${api}/api/demo/presets`, { cache: 'no-store' });
    return { code: r.status, body: await r.json() };
  }, API);
  const presetCount = presets.body?.presets?.length ?? 0;
  log('demo_presets', presets.code === 200 && presetCount === 3 ? `${presetCount} preset` : `GAGAL (${presets.code})`);
  if (RUN_DEMO) {
    await page.getByRole('button', { name: /▶ DEMO/i }).click();
    await page.waitForSelector('[aria-label="Golden Demo presets"]', { timeout: 5000 });
    const menuText = await page.locator('[aria-label="Golden Demo presets"]').innerText();
    log('demo_menu_play', (menuText.match(/▶ PLAY/g) ?? []).length);
    // Dinner Rush (preset pertama) — termasuk kill rider-sim + pulih (fase 5)
    await page.locator('[aria-label="Golden Demo presets"] li').first().getByRole('button', { name: '▶ PLAY' }).click();
    await page.waitForSelector('[role="status"][aria-live="polite"]', { timeout: 10000 });
    const tStart = Date.now();
    let steps = new Set();
    // durasi dari state server (sumber kebenaran) — pola fase 5
    while (Date.now() - tStart < 130000) {
      await sleep(2000);
      const st = await page.evaluate(async (api) =>
        fetch(`${api}/api/demo/state`, { cache: 'no-store' }).then((r) => r.json()).catch(() => null), API);
      const banner = await page.locator('[role="status"][aria-live="polite"]').innerText().catch(() => null);
      if (banner) {
        const m = banner.match(/LANGKAH (\d+)\/(\d+)/);
        if (m) steps.add(`${m[1]}/${m[2]}`);
      }
      if (st && !st.active) break;
    }
    log('golden_demo_e2e', `selesai ${(Math.round((Date.now() - tStart) / 100) / 10).toFixed(1)}s · langkah ${[...steps].join(' ') || '—'}`);
    await page.screenshot({ path: `${OUT}/phase07-hidden-after-demo.png` });
  }

  const appErrors = consoleErrors.filter(e => !/WebSocket connection|Failed to fetch|net::/i.test(e));
  log('console_errors', appErrors.length === 0 ? 0 : appErrors.slice(0, 5).join(' | '));
  const fails = results.filter(([k, v]) => /GAGAL|HILANG|BOCOR|BERUBAH|KOSONG|\?$/.test(String(v)));
  console.log(fails.length ? `FAIL: ${fails.map(f => f.join('=')).join(', ')}` : 'ALL HIDDEN CHECKS PASSED');
  process.exit(fails.length || appErrors.length ? 1 : 0);
} finally {
  await browser.close();
}
