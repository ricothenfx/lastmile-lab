// dispatch-consumer — konsumsi topic `orders` (Fase 2).
//
//	Redpanda `orders` → dedupe by order id → Strategy FIFO (pkg/dispatch)
//	→ injeksi assignment ke rider-sim → tulis Postgres → commit offset manual.
//
// At-least-once: offset di-commit hanya setelah seluruh batch sukses
// (inject + DB). Replay aman: dedupe memori + engine dedupe + unique index DB.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// ---- dedupe ring by order id ----

type seenSet struct {
	mu   sync.Mutex
	m    map[string]struct{}
	ring []string
	idx  int
}

func newSeenSet(capacity int) *seenSet {
	return &seenSet{m: make(map[string]struct{}, capacity), ring: make([]string, 0, capacity)}
}

func (s *seenSet) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[id]
	return ok
}

func (s *seenSet) Add(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ring) < cap(s.ring) {
		s.ring = append(s.ring, id)
	} else {
		delete(s.m, s.ring[s.idx])
		s.ring[s.idx] = id
		s.idx = (s.idx + 1) % cap(s.ring)
	}
	s.m[id] = struct{}{}
}

// ---- kontrak dependency (dianyam di main.go) ----

type SimClient interface {
	IdleRiders(ctx context.Context) ([]dispatch.RiderView, error)
	Inject(ctx context.Context, o model.ExternalOrder) (result string, riderID int, err error)
}

type DB interface {
	UpsertOrder(ctx context.Context, msg model.OrderMsg) error
	EventIngested(ctx context.Context, orderID, key string) error
	SetAssigned(ctx context.Context, orderID string) error
	EventAssigned(ctx context.Context, orderID string, riderID int, distM float64, strategy string) error
}

// ---- perencanaan batch (murni — mudah dites) ----

type batchPlan struct {
	orders []model.ExternalOrder // dengan RiderID sesuai assignment FIFO (atau -1)
}

// planBatch memetakan pesan → injeksi: FIFO via Strategy bersama, oldest-first.
func planBatch(msgs []model.OrderMsg, riders []dispatch.RiderView, strat dispatch.Strategy) batchPlan {
	views := make([]dispatch.OrderView, 0, len(msgs))
	for _, m := range msgs {
		views = append(views, dispatch.OrderView{
			ID: m.ID, CreatedMs: m.CreatedMs,
			PickupLat: m.PickupLat, PickupLon: m.PickupLon,
		})
	}
	assign := make(map[string]dispatch.Assignment)
	for _, a := range strat.Assign(views, riders) {
		assign[a.OrderID] = a
	}
	plan := batchPlan{orders: make([]model.ExternalOrder, 0, len(msgs))}
	for _, m := range msgs {
		eo := model.ExternalOrder{
			ID:        m.ID,
			PickupLat: m.PickupLat, PickupLon: m.PickupLon,
			DropoffLat: m.DropoffLat, DropoffLon: m.DropoffLon,
			CreatedMs: m.CreatedMs,
			RiderID:   -1,
		}
		if a, ok := assign[m.ID]; ok {
			eo.RiderID = a.RiderID
			eo.DistM = a.DistM
		}
		plan.orders = append(plan.orders, eo)
	}
	return plan
}

// ---- metrics ----

type counters struct {
	mu          sync.Mutex
	consumed    uint64
	duplicates  uint64
	assigned    uint64
	queued      uint64
	engineDups  uint64
	parseErrors uint64
	simErrors   uint64
	dbErrors    uint64
	batches     uint64
	lastCommit  int64 // unix ms
}

func (c *counters) snap() map[string]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]uint64{
		"consumed":          c.consumed,
		"duplicates_local":  c.duplicates,
		"assigned":          c.assigned,
		"queued":            c.queued,
		"duplicates_engine": c.engineDups,
		"parse_errors":      c.parseErrors,
		"sim_errors":        c.simErrors,
		"db_errors":         c.dbErrors,
		"batches":           c.batches,
		"last_commit_ms":    uint64(c.lastCommit),
	}
}

// ---- runner ----

type Runner struct {
	cl       *kgo.Client
	sim      SimClient
	db       DB
	strat    dispatch.Strategy
	seen     *seenSet
	batchMax int
	cnt      counters
}

func NewRunner(cl *kgo.Client, sim SimClient, db DB, batchMax int, strat dispatch.Strategy) *Runner {
	if strat == nil {
		strat = dispatch.FIFO{}
	}
	return &Runner{cl: cl, sim: sim, db: db, strat: strat,
		seen: newSeenSet(100_000), batchMax: batchMax}
}

// Run memutar loop konsumsi sampai ctx selesai.
func (r *Runner) Run(ctx context.Context) {
	for {
		fs := r.cl.PollRecords(ctx, r.batchMax)
		fs.EachError(func(_ string, _ int32, err error) {
			log.Printf("fetch error: %v", err)
		})
		if fs.NumRecords() == 0 {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if err := r.process(ctx, fs); err != nil {
			log.Printf("batch gagal (tanpa commit, replay at-least-once): %v", err)
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (r *Runner) process(ctx context.Context, fs kgo.Fetches) error {
	var msgs []model.OrderMsg
	fs.EachRecord(func(rec *kgo.Record) {
		var m model.OrderMsg
		if err := json.Unmarshal(rec.Value, &m); err != nil || m.ID == "" {
			r.cnt.mu.Lock()
			r.cnt.parseErrors++
			r.cnt.mu.Unlock()
			return
		}
		if r.seen.Has(m.ID) {
			r.cnt.mu.Lock()
			r.cnt.duplicates++
			r.cnt.mu.Unlock()
			return
		}
		msgs = append(msgs, m)
	})
	if len(msgs) == 0 {
		return r.commit(ctx, fs) // semua parse-error/dup — offset tetap maju
	}

	riders, err := r.sim.IdleRiders(ctx)
	if err != nil {
		r.cnt.mu.Lock()
		r.cnt.simErrors++
		r.cnt.mu.Unlock()
		return err
	}

	plan := planBatch(msgs, riders, r.strat)
	type assignedRow struct {
		orderID string
		riderID int
		distM   float64
	}
	var assigned []assignedRow
	for _, eo := range plan.orders {
		result, riderID, err := r.sim.Inject(ctx, eo)
		if err != nil {
			r.cnt.mu.Lock()
			r.cnt.simErrors++
			r.cnt.mu.Unlock()
			return err
		}
		switch result {
		case "assigned":
			r.cnt.mu.Lock()
			r.cnt.assigned++
			r.cnt.mu.Unlock()
			assigned = append(assigned, assignedRow{eo.ID, riderID, eo.DistM})
		case "queued":
			r.cnt.mu.Lock()
			r.cnt.queued++
			r.cnt.mu.Unlock()
		case "duplicate":
			r.cnt.mu.Lock()
			r.cnt.engineDups++
			r.cnt.mu.Unlock()
		default:
			r.cnt.mu.Lock()
			r.cnt.parseErrors++
			r.cnt.mu.Unlock()
		}
	}

	// DB — idempoten (ON CONFLICT / unique index) untuk replay at-least-once.
	for _, m := range msgs {
		if err := r.db.UpsertOrder(ctx, m); err != nil {
			r.cnt.mu.Lock()
			r.cnt.dbErrors++
			r.cnt.mu.Unlock()
			return err
		}
	}
	for _, a := range assigned {
		if err := r.db.SetAssigned(ctx, a.orderID); err != nil {
			r.cnt.mu.Lock()
			r.cnt.dbErrors++
			r.cnt.mu.Unlock()
			return err
		}
		if err := r.db.EventAssigned(ctx, a.orderID, a.riderID, a.distM, r.strat.Name()); err != nil {
			r.cnt.mu.Lock()
			r.cnt.dbErrors++
			r.cnt.mu.Unlock()
			return err
		}
	}

	for _, m := range msgs {
		r.seen.Add(m.ID)
	}
	r.cnt.mu.Lock()
	r.cnt.consumed += uint64(len(msgs))
	r.cnt.batches++
	r.cnt.mu.Unlock()
	return r.commit(ctx, fs)
}

func (r *Runner) commit(ctx context.Context, fs kgo.Fetches) error {
	err := r.cl.CommitRecords(ctx, fs.Records()...)
	if err == nil {
		r.cnt.mu.Lock()
		r.cnt.lastCommit = time.Now().UnixMilli()
		r.cnt.mu.Unlock()
	}
	return err
}

// ---- helpers HTTP untuk injeksi & snapshot rider-sim ----

func postInject(ctx context.Context, hc *http.Client, baseURL string, o model.ExternalOrder) (string, int, error) {
	b, err := json.Marshal(o)
	if err != nil {
		return "", -1, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/internal/orders", strings.NewReader(string(b)))
	if err != nil {
		return "", -1, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return "", -1, errOrderSourceInternal
	}
	if resp.StatusCode != http.StatusOK {
		return "", -1, &httpError{status: resp.StatusCode, path: "/internal/orders"}
	}
	var out struct {
		Result string `json:"result"`
		Rider  int    `json:"rider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", -1, err
	}
	return out.Result, out.Rider, nil
}

type httpError struct {
	status int
	path   string
}

func (e *httpError) Error() string {
	return "http " + strconv.Itoa(e.status) + " on " + e.path
}

var errOrderSourceInternal = errorString("rider-sim dalam mode ORDER_SOURCE=internal (injeksi ditolak)")

type errorString string

func (e errorString) Error() string { return string(e) }
