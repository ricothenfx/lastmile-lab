/**
 * Interview Q&A content (fase 9) — questions a senior interviewer would ask
 * about this project, with answers grounded in the repo's own evidence.
 *
 * Rules: every answer cites an ADR (docs/BLUEPRINT.md §5) or a measured
 * number from reports/. Honest about trade-offs and known limits — the
 * interviewer always digs for weaknesses, and candor is the selling point.
 */

export interface QAItem {
  q: string;
  a: string;
  /** Bukti: ADR # / laporan terukur. */
  ev: string;
}

export interface QACategory {
  title: string;
  items: QAItem[];
}

export const INTERVIEW_CATEGORIES: QACategory[] = [
  {
    title: 'Product & architecture',
    items: [
      {
        q: 'Why build this at all — what does it actually demonstrate?',
        a: 'It is a working control room for a simulated last-mile delivery fleet (100 riders, Berlin road graph, live order flow). The point is not the simulation itself but the engineering around it: a deterministic dispatch core, an event-driven order pipeline, measurable strategy A/B tests, chaos experiments with real recovery numbers, and a 60 fps operations UI that never goes blank. Every number on screen comes from the running system — nothing is decorative.',
        ev: 'docs/BLUEPRINT.md §1–§3 · reports/phase-03-bench.md',
      },
      {
        q: 'Why Go for the backend, and not Kotlin, Java or Node.js?',
        a: 'Three reasons. First, RAM: the whole stack must fit a hard 2 GB budget on a shared 4-core/8 GB VPS, and Go services run at tens of MiB each. Second, operations: single static binaries make small Docker images and fast cold starts. Third, the target domain: dispatch engines and gateways are Go’s home turf (small hot loops, cheap concurrency, no GC surprises at p99).',
        ev: 'ADR D1 · docker stats in reports/phase-06-prod.md §5',
      },
      {
        q: 'Why a monorepo — apps/web, apps/services, tools, deploy together?',
        a: 'Cross-cutting changes land atomically: a dispatch strategy lives in pkg/dispatch, is consumed by two services and visualized by the frontend — one commit, one CI run, one review. The deploy/ directory and docs travel with the code so the runbook can never drift from reality. At this team size, monorepo overhead is near zero and the coordination win is constant.',
        ev: 'repo layout · AGENTS.md §5',
      },
      {
        q: 'Why Berlin with real OSM data — why not a fake grid?',
        a: 'Realism is cheap here and pays twice. tools/graphgen extracts a real inner-city Berlin graph from OpenStreetMap (9,459 nodes, 18,571 edges, 1,419 culinary POIs), so routing distances — and therefore strategy benchmarks — behave like a real city (haversine lies about route distance). The same extraction feeds the map, so the visual and the simulation can never disagree.',
        ev: 'ADR D12 · apps/services/rider-sim/data/berlin_graph.json',
      },
      {
        q: 'Is this a real delivery system?',
        a: 'It is a production-grade simulation, and I say that precisely: orders are synthetic (seeded Poisson), but every downstream concern is real — Kafka-compatible ingestion with idempotency, at-least-once consumption with dedupe, backpressure, chaos kills with measured MTTD/MTTR, SLOs with error budgets, and a deploy pipeline with health gates. What you would replace to go live is the order source, not the architecture.',
        ev: 'ADR D13/D15 · reports/phase-02-loadtest.md',
      },
    ],
  },
  {
    title: 'Kafka & the order pipeline',
    items: [
      {
        q: 'Why do you need Kafka-style messaging at all?',
        a: 'Because order ingestion and dispatch have independent failure and scaling profiles. A burst of orders must be accepted (durable, 201) even while the dispatcher is busy — the log absorbs the spike and lets the consumer catch up with full replayability. It also gives clean at-least-once semantics with offset commits, which is exactly the contract a dispatch consumer needs.',
        ev: 'ADR D15 · reports/phase-02-loadtest.md (spike ×10, zero loss)',
      },
      {
        q: 'Why Redpanda instead of Apache Kafka?',
        a: 'The API-compatible Kafka implementation, chosen for a hard infrastructure reason: Kafka’s JVM needs roughly 2–4× the RAM of Redpanda for this workload, and the whole lastmile stack is capped at 2 GB on a shared VPS. The engineering story — topics, partitions, consumer groups, offsets — is identical, so nothing is lost from the portfolio narrative.',
        ev: 'ADR D2 · deploy/compose.yaml limits',
      },
      {
        q: 'Why not RabbitMQ, Redis Streams, or a cloud queue?',
        a: 'RabbitMQ is great at routing but the delivery is not a replayable log — reprocessing history (which replay mode and debugging rely on) becomes awkward. Redis Streams could work at this scale but its consumer-group story is weaker and it already has another job here (idempotency keys). A managed cloud queue violates the no-external-dependency principle — the demo must never die from a quota or a bill.',
        ev: 'ADR D2/D12 · docs/BLUEPRINT.md §3',
      },
      {
        q: 'At-least-once means duplicates. How do you survive them?',
        a: 'Three independent layers: the consumer keeps an LRU seen-set (100k), the engine keeps its own seen-set and re-validates every injected assignment, and Postgres has a unique index per (order, event type) so replays are no-ops at the storage layer. Load tests measured zero duplicates end-to-end: sent = consumed = stored = injected = 4,849.',
        ev: 'ADR D15 · reports/phase-02-loadtest.md',
      },
      {
        q: 'Why inject assignments into the simulator over HTTP instead of a second Kafka consumer inside it?',
        a: 'So the engine stays broker-free and the demo never depends on the broker being up: ORDER_SOURCE=internal keeps it fully autonomous, while the pipeline mode is opt-in. The consumer owns the ugly part (offsets, dedupe) and injects through one validated endpoint; the engine re-validates and falls back to internal FIFO if anything smells wrong. Failure domains stay separated.',
        ev: 'ADR D15 · apps/services dispatch-consumer',
      },
    ],
  },
  {
    title: 'Dispatch & algorithms',
    items: [
      {
        q: 'Why must the dispatch core be deterministic?',
        a: 'Because every headline feature stands on it: the A/B Strategy Lab pipes ONE seeded order generator into two identical engines so the delta between strategies is pure signal, zero noise; replay scrubbing shows the exact decision at any timestamp; and tests assert byte-identical outcomes per seed. Determinism is what turns a demo into a measuring instrument.',
        ev: 'ADR D13/D18 · internal/sim determinism tests',
      },
      {
        q: 'Walk me through the four dispatch strategies. Why four?',
        a: 'They form a measured progression: FIFO (oldest order first — the fairness baseline), Batching (2 s window, cluster by pickup grid — throughput play with a TTL cost), Zone (grid + neighbor rings — locality play), and Optimal (bipartite min-cost via Hungarian/Jonker-Volgenant — the ceiling). Four strategies let the benchmark answer “under which load does sophistication pay?” with numbers instead of opinions.',
        ev: 'ADR D17 · reports/phase-03-bench.md §2–§3',
      },
      {
        q: 'You implemented the Hungarian algorithm by hand instead of using OR-Tools. Defend that.',
        a: 'OR-Tools is a C++ dependency behind CGO: image bloat, slower builds, and a version-risk footprint — for a problem that is O(n²m) and, at our scale (n ≤ 500), milliseconds. The pure-Go Jonker-Volgenant implementation is deterministic, dependency-free, and verified against brute-force permutation on small inputs. Measured p99 at 100×100: 13.1 ms on a single pinned CPU under host load 9+ — well inside the 50 ms target.',
        ev: 'ADR D17 · reports/phase-03-bench.md §3',
      },
      {
        q: 'What do the strategy benchmarks actually show?',
        a: 'Under rush (45 orders/min, 600 s virtual): Optimal delivered 255 vs FIFO’s 129 (+98%) with cost per order down 54% (0.93 vs 2.04 km on-task). Batching trades tail latency for expiry (+48% expired). And the most interesting result: at steady 70% capacity all four strategies are statistically identical — sophistication only pays under pressure. That is the kind of conclusion interviewers should grill.',
        ev: 'reports/phase-03-bench.md · Strategy Lab duel (ADR D18)',
      },
      {
        q: 'When would you NOT use the optimal strategy?',
        a: 'When the fleet-to-order ratio is healthy or n explodes: the assignment problem is superlinear, and at 300×300 Optimal’s p99 hits 303 ms — past the 50 ms dispatch SLO. The safe envelope is documented (n ≤ ~150–200 per tick), and FIFO/Zone remain better engineering under most real conditions. Picking the fancy algorithm by default is how dispatch systems melt.',
        ev: 'reports/phase-03-bench.md §3 (300×300 boundary)',
      },
      {
        q: 'Why is there no ML/LLM in the dispatch loop?',
        a: 'By design. Dispatch decisions must be explainable (“fifo: oldest order (age 0s) → nearest idle rider r40, 1011 m”), reproducible per seed, and safe to replay — none of which a model guarantees. The LLM exists only in the advisory copilot (phase 7), where it proposes plans that a deterministic simulator dry-runs before a human executes anything.',
        ev: 'ADR D8/D24 · dispatch decision reason ring',
      },
    ],
  },
  {
    title: 'Frontend craft',
    items: [
      {
        q: 'Why Canvas 2D for the map overlay — not SVG, not WebGL?',
        a: 'The overlay redraws ~100 riders + ~50 orders at 60 fps with trails and pulses. SVG would mean thousands of DOM nodes and layout thrash; WebGL (deck.gl) buys GPU throughput this particle count doesn’t need while costing bundle and complexity. Plain 2D canvas with interpolation between 10 Hz snapshots draws in ~3.1 ms/frame — an order of magnitude inside the frame budget.',
        ev: 'ADR D3 · reports/phase-01 perf (draw 3.1 ms/frame)',
      },
      {
        q: 'How do you actually hold 60 fps?',
        a: 'Discipline, not hope: exactly one rAF loop for the map (all features — heat zones, bursts, trails — draw inside it), render-on-demand when replay is paused (guard by frame key, zero work while idle), React updates throttled to 2 Hz with memoized subtrees, and a per-feature rAF audit in the verification scripts so no panel sneaks in its own loop. Headless-measured p50 frame time was 16.7 ms — vsync-locked.',
        ev: 'reports/phase-01/04/05 verify scripts · phase-08 rAF audit (28/2 s idle)',
      },
      {
        q: 'Why MapLibre with self-hosted GeoJSON — no Google Maps, no Mapbox?',
        a: 'Because a portfolio demo must never die from an API key, a quota, or a billing accident. The style, roads, water, and even the label glyphs (SDF PBFs generated from Inter) are committed to the repo. Bonus: the palette is 100% design tokens, so the map is visually native to the control room instead of a pasted-in third-party widget.',
        ev: 'ADR D3/D12 · apps/web/public/berlin + fonts/Inter',
      },
      {
        q: 'The backend is down. What does a visitor see?',
        a: 'The same product. If the WebSocket doesn’t come up within 3 seconds — or the feed stalls — the app switches to a recorded fixture loop with a REPLAY banner, and every panel keeps working (KPI deck shows “—” for unmeasured values instead of lying). Verified two ways: blocking api/ws domains in the browser, and actually stopping the backend containers. No dead pages, ever.',
        ev: 'ADR D9/D14 · reports/phase-05/06 offline checks',
      },
      {
        q: 'Tell me about a production bug you found and fixed.',
        a: 'My favorite: after weeks of screenshots, the live map showed zero dots — yet the overlay canvas buffer contained tens of thousands of entity pixels, WebSocket healthy, projections correct. Root cause: MapLibre appends its WebGL canvas to the container AFTER the overlay canvas, both position:absolute without z-index — the map was covering everything, since phase 1. The fix was one line (explicit z-index); the real fix is the pixel-probe assertion now in the verification suite. Lesson: render-order bugs are invisible to logic checks — assert pixels.',
        ev: 'docs/PROGRESS.md session 13 · phase-08 pixel probe',
      },
      {
        q: 'How do you handle accessibility and motion sensitivity?',
        a: 'prefers-reduced-motion is honored end-to-end: radar pulses, trails, dash flow, interpolation, heat breathing and bursts all switch off (verified headless), leaving a fully readable static map. Text keeps WCAG AA contrast pairs (labels 7.4:1 and 12.7:1 on the map background), controls carry aria-labels + tooltips, and the in-app guide auto-opens once for first-time visitors.',
        ev: 'docs/DESIGN.md §5/§7 · phase-08 reduced-motion checks',
      },
    ],
  },
  {
    title: 'Reliability & SRE',
    items: [
      {
        q: 'How do you prove reliability rather than claim it?',
        a: 'Chaos experiments with instruments: an injector kills services under live traffic, health monitors detect failures (1 Hz, flap-suppressed), and incidents record t_start/t_detect/t_recover. Four kills measured MTTD 0.81–1.30 s and MTTR 1.98–3.02 s; the consumer kill under a ×10 spike showed zero message loss (5,460 acked = 5,460 stored). The error budget for that window was honestly reported as exceeded (99.69% vs 99.9% SLO) — four kills in 13 minutes will do that.',
        ev: 'ADR D19 · reports/phase-04-chaos.md',
      },
      {
        q: 'Why kill containers with SIGTERM to PID 1 instead of the Docker kill API?',
        a: 'Two empirically verified Docker facts forced the design: (1) the /containers/{id}/kill API does NOT trigger the restart policy — the daemon treats it as a manual stop, so the node never came back; (2) PID 1 inside its own PID namespace is immune to SIGKILL from within, but SIGTERM reaches the Go runtime’s handler → clean exit → unless-stopped restarts it. So the injector execs a fixed `kill -TERM 1`, recovery is never manual, and what we measure is exactly the self-healing we claim.',
        ev: 'ADR D19 rev.3 (verified against Docker 29.8.1) · chaos injector design',
      },
      {
        q: 'What are the SLOs, and what happens when they break?',
        a: 'Explicit: p95 delivery < 360 s, zero message loss, grid healthy, availability ≥ 99.9% (0.1% budget). The KPI deck computes them from the engine’s metric ring and pipeline counters; a violating card visibly shakes (off under reduced-motion), and the System Health panel shows the incident timeline with MTTD/MTTR. Unmeasured values render as “—” — the dashboard refuses to invent numbers.',
        ev: 'ADR D20 · reports/phase-04-chaos.md §error budget',
      },
      {
        q: 'Convince me there is no message loss under surge.',
        a: 'Counted end-to-end, twice. Spike test: Poisson 300 orders/min with two ×10 bursts (50/s) — sent(acked) 4,261 = consumed = stored = injected, 0 HTTP 429, consumer lag 0, DB window delta exactly matching. A second run reproduced it at 4,849. Idempotency keys make producer retries safe; the three-layer dedupe makes consumer replays safe; and the DB delta check closes the loop between Kafka and storage.',
        ev: 'reports/phase-02-loadtest.md · reports/phase-04-chaos.md E4',
      },
    ],
  },
  {
    title: 'Ops & cost',
    items: [
      {
        q: 'Why no Prometheus or Grafana — isn’t that table stakes?',
        a: 'They cost ~300–400 MiB of RAM, which pushes the stack past the hard 2 GB cap on a shared VPS — a real outage risk for other tenants. The JSON metrics endpoints plus the KPI deck and health grid deliver the actual observability value (real numbers, SLO math, incident timeline) at ~0 MiB. It’s a documented trade-off with a revisit condition, not an oversight.',
        ev: 'ADR D16 · reports/phase-06-prod.md §5 (RAM budget)',
      },
      {
        q: 'Describe the deployment pipeline.',
        a: 'Push → GitHub Actions builds Go binaries into multi-stage images → GHCR → the VPS job pulls with a load guard (refuses when 15-min loadavg > 8 — it has actually rejected deploys on a busy host and succeeded on retry) → compose up -d → healthz gate. The frontend is a Vercel project with GitHub-integrated production deploys. The deploy SSH key is a dedicated ed25519 pair, revocable independently.',
        ev: 'ADR D7 · deploy/README.md §4 · reports/phase-06-prod.md',
      },
      {
        q: 'Why is the frontend on Vercel but the backend on a VPS?',
        a: 'Split by what each tier needs. The Next.js app is static-ish and global — Vercel gives no-sleep edges and preview deploys for free. The Go stack (sim, broker, Postgres) needs memory and long-running processes that don’t fit free tiers, so it lives on the VPS behind Caddy site blocks (api. / ws. subdomains) that I added to an existing Caddy without touching its stack.',
        ev: 'ADR D6 · deploy/README.md (Caddy runbook)',
      },
      {
        q: 'How do you keep one VPS from turning into chaos?',
        a: 'Hard isolation rules: every container gets explicit mem_limit + cpus + restart policy + /healthz; the whole stack is budgeted ≤ 2 GB (demo actually idles at ~80 MiB); ports live in a registered block; internal services are Docker-network-only, never published; log rotation is on; and the chaos injector’s Docker socket access is restricted to an explicit allowlist validated by unit tests.',
        ev: 'ADR D11/D19 · deploy/compose.yaml · reports/phase-06-prod.md §5',
      },
    ],
  },
  {
    title: 'Security',
    items: [
      {
        q: 'Your mutation endpoints are public without auth. Seriously?',
        a: 'Yes — deliberately, with eyes open. This is a portfolio showcase: the product is “a reviewer can press the buttons”. The blast radius is bounded mechanically, not by policy: chaos can only kill 7 stateless lastmile-* services (stateful infra returns 403, unit-tested), recovery is automatic, the lab runs one duel at a time pinned to 1 CPU, and there is zero user data — everything is seeded synthetic data. The documented upgrade path is a ~30-line bearer-token middleware on POST, intentionally deferred so trying the demo stays frictionless. Risky at a company; calibrated here.',
        ev: 'ADR D23 (+ D19 allowlist) · deploy/README.md §9',
      },
      {
        q: 'What’s your secret-handling story?',
        a: 'No secrets in git, enforced by structure: env files are gitignored (both repo root and deploy/), CI holds three deploy secrets (SSH key/host/user) with the private key never touching logs, the copilot API key lives only in the VPS env, and a dedicated deploy keypair is revocable without touching my personal keys. Verification runs even check that generated artifacts stay placeholder-only.',
        ev: 'AGENTS.md §4 · docs/PROGRESS.md sessions 10–11',
      },
    ],
  },
  {
    title: 'AI copilot & scale',
    items: [
      {
        q: 'Where does AI fit in this system?',
        a: 'As a strictly-bounded plugin (phase 7). The copilot service is OFF by default — without an API key the UI renders zero copilot nodes and the whole app is byte-identical to the baseline (proven by regression). With a key, the Advisor may PROPOSE up to three plans, but every plan is dry-run by the deterministic simulator in a same-seed duel before a human two-click-executes it via existing control endpoints. The LLM proposes; the simulator judges; the human executes.',
        ev: 'ADR D24 · reports/phase-07-copilot.md',
      },
      {
        q: 'How do you keep the LLM from hallucinating its way into production?',
        a: 'Four guards: strict JSON schema on output with invalid plans discarded (not repaired silently); actions are whitelisted and kill-actions return “cannot be simulated — no fake numbers” notes instead of invented ones; Ask Ops answers must cite internal metric/incident IDs or the API rejects them (422); and rate limits cap the blast radius per endpoint. A 15-case ground-truth evaluation measured where it’s strong (metrics) and weak (non-active incidents) — published, not hidden.',
        ev: 'ADR D24 · reports/phase-07-copilot.md §6 (15-case eval)',
      },
      {
        q: 'What breaks at 10,000 riders? Be honest.',
        a: 'Several things, all known and documented rather than hand-waved: Optimal dispatch is superlinear (p99 already 303 ms at 300×300 — needs zoning/sharding), the single VPS and RF=1 broker are single points of failure, Postgres write path would need partitioning, and the 2 GB budget assumes demo-scale fleets. The mitigations are standard (zone-sharded dispatch, 3-replica broker, managed PG, RAM budget review) — deliberately not built, because building them without the load that justifies them is how systems rot.',
        ev: 'reports/phase-03-bench.md §3 · docs/BLUEPRINT.md §10 (non-goals)',
      },
      {
        q: 'What would you build next, in priority order?',
        a: '1) Bearer auth on mutation endpoints (the D23 upgrade path, ~30 lines). 2) RF=3 broker + partition-by-zone so dispatch locality survives scaling. 3) Zone-sharded optimal dispatch to keep p99 under the SLO at larger n. 4) Prometheus/Grafana the moment the RAM budget allows. 5) Multi-city via graphgen parameterization — the pipeline is city-agnostic by construction.',
        ev: 'ADR D23/D16/D12 · docs/ROADMAP.md',
      },
    ],
  },
];

export const INTERVIEW_TOTAL = INTERVIEW_CATEGORIES.reduce(
  (n, c) => n + c.items.length,
  0,
);
