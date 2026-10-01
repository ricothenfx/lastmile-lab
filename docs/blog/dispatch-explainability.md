# Making a dispatch engine explainable: replay, inspect, and a golden demo that never lies

> lastmile-lab is a simulation & benchmark platform for last-mile delivery operations:
> 100 riders on a real Berlin road network (OpenStreetMap), four dispatch strategies, a
> Kafka-shaped order pipeline, chaos experiments, and a live control-room UI. This article
> is about the part that took the most discipline: making the engine *explainable* —
> deterministic enough to record, honest enough to demo, and cheap enough to run in 95 MiB
> of RAM on a small VPS.
>
> *Draft final, 2026-10-01 · all numbers reproducible from the repo (`reports/`).*

## The problem: dashboards that can't answer "why?"

Every delivery platform has a map with moving dots. Very few can answer the question that
actually matters at 18:47 on a Friday: **why is rider 40 going to restaurant B instead of
restaurant A?** The usual answer is a shrug — the assignment came out of a model, the
moment is gone, and the incident review happens on screenshots.

When I started lastmile-lab as a portfolio piece for delivery/quick-commerce engineering
roles, I set one rule that shaped everything else: **every number on the dashboard must be
sourced from a real metric in the system, and every decision must carry its reason.**
A dispatch platform you can't interrogate is just an animated screensaver.

## Determinism is the whole trick

The core simulation (`internal/sim`) is a virtual-clock, seeded, deterministic engine:
orders arrive on a Poisson schedule derived from the seed, riders move on the road graph,
and assignment goes through a single interface:

```go
type Strategy interface {
    Name() string
    Assign(orders []OrderView, riders []RiderView) []Assignment // each carries a Reason
}
```

Four implementations ship: `fifo` (oldest order → nearest idle rider), `batching`
(2-second stateless window, then cluster-by-pickup), `zone` (grid + ring locality bias),
and `optimal` (min-cost bipartite matching via Jonker-Volgenant, pure Go, no OR-Tools/CGO).

Determinism pays for itself three times:

1. **Fair A/B duels.** One order generator feeds *two identical engines* on the same tick
   (`internal/duel`). Same input bytes, different strategy. In a 600-second virtual rush
   hour, `optimal` delivered **255 orders vs FIFO's 129 (+98%)** at **0,93 km cost per
   order vs 2,04 km (−54%)**. In steady state (70% capacity) all four strategies are
   statistically identical — which is itself a lesson: strategy only pays under pressure.
2. **Replayable time.** If the run is a pure function of (seed, strategy, ticks), the past
   is not gone. It can be recorded cheaply — which brings us to the interesting part.
3. **A demo you can trust.** A "golden path" through the system either always happens the
   same way, or it's not a demo, it's a gamble.

Decision latency matters too, because dispatch is an online problem: measured on a single
CPU core with real Berlin coordinates, p99 per call is **2,6 ms (batching) to 13,1 ms
(optimal)** at 100 orders × 100 riders — two orders of magnitude under the 50 ms budget.

## The replay engine: 15 minutes of city-wide time in 12 MiB

Phase 5 added a session recorder to the rider-sim service. The requirements were brutal
and simple: the engine must not be touched (determinism is load-bearing), and the memory
budget must not explode. The design that survived:

- A **separate goroutine** pulls a full snapshot at 5 Hz into a **fixed-capacity ring**:
  4 500 frames = 15 minutes. Old frames fall off; the cap is enforced by a byte fence.
- Each snapshot is **marshalled to JSON exactly once** and compressed with **gzip per
  frame** — ~2,6 KB/frame at 100 riders. The whole ring is **11,9 MiB compressed**, and
  the rider-sim process settles at a 46 MiB RSS inside a 160 MiB container limit.
- Dispatch decisions for the window are recorded too (deduplicated by sequence number,
  capped at 8 000), so the UI can show *why* at any point in the past.

The frontend scrubs this dump with binary search — **≤ 0,2 s accuracy** — and clicking a
rider shows their state, their order, and the recorded dispatch reason:

> `fifo: antrean tertua (umur 0s) → rider r40 idle terdekat (1011 m, haversine)`

That single line is the product. It turns "trust the black box" into "audit the decision."

### The bug that taught me how browsers eat gzip

The first version of the dump endpoint concatenated per-frame gzip members (perfectly
valid — curl decompressed it happily, Go too). In Chromium the stream **stopped after the
first member** and the frontend silently received truncated JSON. Browsers treat a
multi-member gzip stream as the end of body after the first member in fetch/XHR contexts.
The fix: when serving, stream-decompress the stored ring and **re-compress into exactly
one gzip member**. There is now a unit test asserting the served body contains precisely
one gzip member — because "works in curl" is not "works in a browser."

(Second lesson from the same verification round, for the web folks: maplibre-gl.css sets
`.maplibregl-map { position: relative }`, which can override a Tailwind `absolute`
utility depending on CSS chunk order and collapse the map container to zero height. An
inline `position: absolute` on the container ended a very confusing "the map is black"
bug.)

## The Golden Demo: 90 seconds, same seconds every time

Recording and inspecting prove the system explains itself after the fact. The Golden Demo
proves it *live* — to a recruiter, at 2 a.m., with no one watching.

It's a tiny orchestrator in the api-gateway with three ~90-second presets
(dinner-rush, blackout-drill, rain-commute). The discipline was in what it **doesn't** do:
no new engine code, no special mode, no LLM. It only pulls the levers that already exist —
surge and weather via sim-control, and a real service kill via the chaos injector. The
dinner-rush preset surges demand ×6, turns on rain, then **kills the rider-sim container
mid-narrative**. The UI banner narrates each step; the incident timeline records the kill;
restart policy (`unless-stopped`) brings the service back.

Because everything is deterministic, the narrative lands on the same seconds every run —
verified in headless UI runs (steps at t+0,3/12/36/54/64/74/86, finishing at exactly
**t+90,0 s**) — and the kill produced an incident with **MTTD 642 ms** and automatic
recovery. A demo that "never lies" is one where the dramatic part is not staged: the kill
is real, the detection is measured, the recovery is Docker's, not a stagehand's.

## Zero loss under a 10× flash sale, and honesty about chaos

Two more results worth their bytes:

- **Pipeline**: loadgen → ingestion (Redis idempotency + Postgres + Kafka sync-ack via
  Redpanda) → 3-partition topic → consumer (manual offset commits, three dedup layers) →
  engine injection. Under a 300 s run with two 10× spikes: **sent = acked = consumed =
  stored = injected = 4 849**, zero rejections, zero duplicates, zero lag.
- **Chaos**: killing rider-sim, ws-gateway, api-gateway, and (under full pipeline load)
  the dispatch-consumer — measured MTTD **0,81–1,30 s**, MTTR **1,98–3,02 s**, and the
  consumer kill still ended with **5 460 = 5 460** messages. The error budget for that
  13-minute experiment window *exceeded* the 99,9% availability SLO (99,69%) — reported
  as-is, because a benchmark platform that hides its own worst numbers is a marketing site.

Two Docker footguns are now permanent ADR material: `/containers/{id}/kill` does **not**
trigger restart policy (Docker counts it as a manual stop), and PID 1 is immune to SIGKILL
*from inside its own namespace* — so chaos kills via `SIGTERM to PID 1` through the exec
API, letting the Go runtime's signal handler exit cleanly and the restart policy do the
healing.

## What I'd tell my past self

1. **Determinism is a feature you buy upfront.** Every cool phase-5 capability (replay,
   duels, golden demo) is a downstream purchase of a phase-1 decision (seeded virtual
   clock). Retrofitting it is brutal.
2. **Budgets make design decisions for you.** A hard 2 GB stack budget on a shared VPS
   killed the default "just add Grafana" reflex and produced per-frame gzip + marshal-once
   instead. Constraints were the architect.
3. **Verify in the browser, not in curl.** Both phase-5 bugs were invisible to CLI tooling
   and obvious in headless Chromium.
4. **Report the ugly numbers.** Headless FPS on a software-GL VPS is load-contaminated;
   the honest claim is the 3,1 ms canvas draw EMA and 16,7 ms p50 frame, with the caveat
   attached. Recruiters can smell a curated benchmark.

Try it: **lastmile-lab.ricothen.com** — and if the backend happens to be down when you
click, you'll just watch the replay ring instead. That's not a bug; that's the point.
