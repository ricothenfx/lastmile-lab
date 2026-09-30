// Package sim holds the deterministic core simulation: riders moving on the
// road graph, Poisson order arrivals, a Strategy interface for dispatch
// (Fase 1: FIFO — Fase 3 menambah strategi lain tanpa refactor).
//
// The engine is wall-clock agnostic: Tick(dtMs) advances a virtual clock, so
// the live service drives it at 10 Hz while the fixture generator can run the
// same code as fast as the CPU allows, fully reproducible via Seed.
package sim

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

type Config struct {
	Seed            int64
	Riders          int
	OrderRatePerMin float64
	SpeedMPS        float64 // kecepatan dasar saat membawa tugas
	IdleSpeedMPS    float64 // cruising saat idle (biar peta terasa hidup)
	TickHz          int
	PickupDwellSec  float64 // ±30% jitter
	DropoffDwellSec float64
	OrderTTLSec     float64
	WeatherFactor   float64 // stub cuaca — Fase 2 disambungkan ke Surge Console
	MinTripM        float64 // jarak minimal pickup→dropoff
}

func DefaultConfig() Config {
	return Config{
		Seed:            42,
		Riders:          60,
		OrderRatePerMin: 20,
		SpeedMPS:        5.5,
		IdleSpeedMPS:    2.6,
		TickHz:          10,
		PickupDwellSec:  35,
		DropoffDwellSec: 8,
		OrderTTLSec:     90,
		WeatherFactor:   1.0,
		MinTripM:        400,
	}
}

// OrderView / RiderView melindungi Strategy dari state internal engine.
type OrderView struct {
	ID        string
	CreatedMs int64
	PickupLat float64
	PickupLon float64
}

type RiderView struct {
	ID     int
	Status model.RiderStatus
	Lat    float64
	Lon    float64
}

// Assignment adalah satu keputusan dispatch + alasannya (explainability).
type Assignment struct {
	OrderID string
	RiderID int
	DistM   float64
	Reason  string
}

// Strategy dipanggil tiap tick dengan antrean order tertua-dulu.
// Implementasi TIDAK BOLEH memutasi state engine.
type Strategy interface {
	Name() string
	Assign(orders []OrderView, riders []RiderView) []Assignment
}

type rider struct {
	id           int
	status       model.RiderStatus
	from, to     int
	progressM    float64
	route        []int // sisa node setelah `to`
	dwellUntilMs int64
	orderID      string
}

type order struct {
	id              string
	status          model.OrderStatus
	pickup, dropoff int
	createdMs       int64
	riderID         int
}

type Engine struct {
	mu       sync.RWMutex
	cfg      Config
	g        *graph.Graph
	rng      *rand.Rand
	strategy Strategy

	nowMs       int64
	seq         uint64
	riders      []*rider
	orders      []*order
	nextOrderN  int
	nextSpawnMs int64

	decisions []model.Decision // ring terbaru di depan (max 25)
	delivered int
	expired   int
	startWall time.Time
}

func New(g *graph.Graph, cfg Config, strategy Strategy) (*Engine, error) {
	if cfg.Riders <= 0 || cfg.TickHz <= 0 || cfg.OrderRatePerMin < 0 {
		return nil, fmt.Errorf("invalid config")
	}
	if strategy == nil {
		strategy = FIFO{}
	}
	e := &Engine{
		cfg:         cfg,
		g:           g,
		rng:         rand.New(rand.NewSource(cfg.Seed)),
		strategy:    strategy,
		nextSpawnMs: 0,
		startWall:   time.Now(),
	}
	if len(g.POIs()) < 2 {
		return nil, fmt.Errorf("graph needs >= 2 POIs")
	}
	for i := 0; i < cfg.Riders; i++ {
		e.riders = append(e.riders, e.spawnRider(i))
	}
	e.nextSpawnMs = 500 + int64(e.rng.Float64()*800) // settle dulu sebelum order pertama
	return e, nil
}

func (e *Engine) spawnRider(id int) *rider {
	n := e.rng.Intn(e.g.NodeCount())
	nb := e.g.Neighbors(n)
	to := n
	var prog float64
	if len(nb) > 0 {
		to = nb[e.rng.Intn(len(nb))]
		prog = e.rng.Float64() * e.g.EdgeLenM(n, to)
	}
	return &rider{id: id, status: model.RiderIdle, from: n, to: to, progressM: prog}
}

// Tick advances the simulation by dtMs of virtual time.
func (e *Engine) Tick(dtMs int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nowMs += dtMs
	e.spawnOrders()
	e.moveRiders(float64(dtMs) / 1000)
	e.expireOrders()
	e.dispatch()
	e.seq++
}

func (e *Engine) spawnOrders() {
	if e.cfg.OrderRatePerMin <= 0 {
		return
	}
	lambdaPerMs := e.cfg.OrderRatePerMin / 60000
	for e.nextSpawnMs <= e.nowMs {
		e.nextSpawnMs += int64(e.rng.ExpFloat64() / lambdaPerMs)
		e.spawnOrder()
	}
}

func (e *Engine) spawnOrder() {
	pickup := e.g.RandomPOI(e.rng)
	var dropoff int
	for i := 0; i < 25; i++ {
		cand := e.g.RandomPOI(e.rng)
		if cand == pickup {
			continue
		}
		d := graph.HaversineM(e.g.NodeLat(pickup), e.g.NodeLon(pickup), e.g.NodeLat(cand), e.g.NodeLon(cand))
		if d >= e.cfg.MinTripM {
			dropoff = cand
			break
		}
		if dropoff == 0 {
			dropoff = cand
		}
	}
	if dropoff == pickup {
		return
	}
	e.nextOrderN++
	e.orders = append(e.orders, &order{
		id:        fmt.Sprintf("o%06d", e.nextOrderN),
		status:    model.OrderWaiting,
		pickup:    pickup,
		dropoff:   dropoff,
		createdMs: e.nowMs,
		riderID:   -1,
	})
}

func (e *Engine) moveRiders(dtS float64) {
	for _, r := range e.riders {
		e.advanceRider(r, dtS)
	}
}

func (e *Engine) advanceRider(r *rider, dtS float64) {
	// dwell states
	if r.status == model.RiderPickup {
		if e.nowMs >= r.dwellUntilMs {
			o := e.activeOrder(r.orderID)
			if o == nil { // order hilang (guard): kembali idle
				r.status, r.orderID = model.RiderIdle, ""
				return
			}
			path := e.g.Dijkstra(r.to, o.dropoff)
			if path == nil {
				r.status, r.orderID = model.RiderIdle, ""
				return
			}
			r.route = path[1:]
			r.status = model.RiderDelivering
			o.status = model.OrderInTransit
		}
		return
	}
	if r.status == model.RiderDelivering && r.dwellUntilMs > 0 {
		if e.nowMs >= r.dwellUntilMs { // selesai antar
			e.delivered++
			e.removeOrder(r.orderID)
			r.status, r.orderID, r.dwellUntilMs = model.RiderIdle, "", 0
		}
		return
	}

	speed := e.cfg.IdleSpeedMPS
	if r.status == model.RiderToPickup || r.status == model.RiderDelivering {
		speed = e.cfg.SpeedMPS
	}
	remaining := speed * e.cfg.WeatherFactor * dtS

	for remaining > 0 {
		edgeLen := e.g.EdgeLenM(r.from, r.to)
		if math.IsNaN(edgeLen) || edgeLen <= 0 {
			// guard: edge rusak → diam di node tujuan, pilih ulang di iterasi berikut
			r.from = r.to
			r.progressM = 0
		}
		if r.from == r.to {
			// diam di node: pilih edge berikutnya
			nb := e.g.Neighbors(r.to)
			if len(nb) == 0 {
				return
			}
			next := nb[e.rng.Intn(len(nb))]
			// saat membawa rute, node berikutnya sudah ditentukan
			if len(r.route) > 0 {
				next = r.route[0]
			}
			r.from, r.to, r.progressM = r.to, next, 0
			edgeLen = e.g.EdgeLenM(r.from, r.to)
			if math.IsNaN(edgeLen) || edgeLen <= 0 {
				return
			}
		}
		left := edgeLen - r.progressM
		if remaining < left {
			r.progressM += remaining
			remaining = 0
			break
		}
		remaining -= left
		// tiba di r.to
		if len(r.route) > 0 {
			r.from, r.to, r.progressM = r.to, r.route[0], 0
			r.route = r.route[1:]
			continue
		}
		// rute habis — tiba di tujuan / pilih arah random
		r.from = r.to
		r.progressM = 0
		switch r.status {
		case model.RiderToPickup:
			r.status = model.RiderPickup
			r.dwellUntilMs = e.nowMs + int64(e.cfg.PickupDwellSec*1000*(0.7+0.6*e.rng.Float64()))
			return
		case model.RiderDelivering:
			r.dwellUntilMs = e.nowMs + int64(e.cfg.DropoffDwellSec*1000)
			return
		default: // idle wander
			nb := e.g.Neighbors(r.to)
			if len(nb) == 0 {
				return
			}
			r.from = r.to
			r.to = nb[e.rng.Intn(len(nb))]
			r.progressM = 0
		}
	}
}

func (e *Engine) expireOrders() {
	ttlMs := int64(e.cfg.OrderTTLSec * 1000)
	kept := e.orders[:0]
	for _, o := range e.orders {
		if o.status == model.OrderWaiting && e.nowMs-o.createdMs > ttlMs {
			e.expired++
			continue
		}
		kept = append(kept, o)
	}
	e.orders = kept
}

func (e *Engine) dispatch() {
	var ov []OrderView
	for _, o := range e.orders {
		if o.status != model.OrderWaiting {
			continue
		}
		ov = append(ov, OrderView{
			ID:        o.id,
			CreatedMs: o.createdMs,
			PickupLat: e.g.NodeLat(o.pickup),
			PickupLon: e.g.NodeLon(o.pickup),
		})
	}
	if len(ov) == 0 {
		return
	}
	sort.Slice(ov, func(i, j int) bool { return ov[i].CreatedMs < ov[j].CreatedMs })

	var rv []RiderView
	for _, r := range e.riders {
		if r.status != model.RiderIdle {
			continue
		}
		lat, lon := e.riderPos(r)
		rv = append(rv, RiderView{ID: r.id, Status: r.status, Lat: lat, Lon: lon})
	}
	if len(rv) == 0 {
		return
	}

	for _, a := range e.strategy.Assign(ov, rv) {
		o := e.activeOrder(a.OrderID)
		r := e.riderByID(a.RiderID)
		if o == nil || r == nil || o.status != model.OrderWaiting || r.status != model.RiderIdle {
			continue
		}
		path := e.g.Dijkstra(r.to, o.pickup)
		if path == nil {
			continue
		}
		r.route = path[1:]
		r.status = model.RiderToPickup
		r.orderID = o.id
		o.status = model.OrderAssigned
		o.riderID = r.id
		e.pushDecision(model.Decision{
			Seq: e.seq, T: e.nowMs, Strategy: e.strategy.Name(),
			OrderID: o.id, RiderID: r.id, DistM: math.Round(a.DistM),
			Reason: a.Reason,
		})
	}
}

func (e *Engine) pushDecision(d model.Decision) {
	e.decisions = append([]model.Decision{d}, e.decisions...)
	if len(e.decisions) > 25 {
		e.decisions = e.decisions[:25]
	}
}

func (e *Engine) riderPos(r *rider) (float64, float64) {
	if r.from == r.to {
		return e.g.NodeLat(r.to), e.g.NodeLon(r.to)
	}
	edgeLen := e.g.EdgeLenM(r.from, r.to)
	f := 0.0
	if edgeLen > 0 {
		f = r.progressM / edgeLen
	}
	if f > 1 {
		f = 1
	}
	lat := e.g.NodeLat(r.from) + (e.g.NodeLat(r.to)-e.g.NodeLat(r.from))*f
	lon := e.g.NodeLon(r.from) + (e.g.NodeLon(r.to)-e.g.NodeLon(r.from))*f
	return lat, lon
}

func (e *Engine) activeOrder(id string) *order {
	for _, o := range e.orders {
		if o.id == id {
			return o
		}
	}
	return nil
}

func (e *Engine) removeOrder(id string) {
	kept := e.orders[:0]
	for _, o := range e.orders {
		if o.id != id {
			kept = append(kept, o)
		}
	}
	e.orders = kept
}

func (e *Engine) riderByID(id int) *rider {
	if id < 0 || id >= len(e.riders) {
		return nil
	}
	return e.riders[id]
}

// Snapshot builds the compact wire snapshot (rounds coords to 6 decimals).
func (e *Engine) Snapshot() model.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.snapshotLocked()
}

// FullSnapshot adds decisions (explainability) — REST only.
func (e *Engine) FullSnapshot() model.FullSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	full := model.FullSnapshot{Snapshot: e.snapshotLocked(), Decisions: e.decisions}
	full.Stats.UptimeSec = int64(time.Since(e.startWall).Seconds())
	return full
}

// snapshotLocked harus dipanggil dengan lock terpasang.
func (e *Engine) snapshotLocked() model.Snapshot {
	s := model.Snapshot{
		T:      e.nowMs,
		Seq:    e.seq,
		Riders: make([]model.RiderPt, 0, len(e.riders)),
		Orders: make([]model.OrderPt, 0, len(e.orders)),
		Links:  make([]model.Link, 0, len(e.orders)),
	}
	idle := 0
	for _, r := range e.riders {
		if r.status == model.RiderIdle {
			idle++
		}
		lat, lon := e.riderPos(r)
		s.Riders = append(s.Riders, model.RiderPt{
			ID: r.id, S: int(r.status),
			Lat: round6(lat), Lon: round6(lon),
		})
	}
	for _, o := range e.orders {
		s.Orders = append(s.Orders, model.OrderPt{
			ID: o.id, S: int(o.status),
			Pla: round6(e.g.NodeLat(o.pickup)), Plo: round6(e.g.NodeLon(o.pickup)),
			Dla: round6(e.g.NodeLat(o.dropoff)), Dlo: round6(e.g.NodeLon(o.dropoff)),
		})
		if o.riderID >= 0 {
			s.Links = append(s.Links, model.Link{R: o.riderID, O: o.id})
		}
	}
	s.Stats = model.Stats{
		Delivered: e.delivered,
		Expired:   e.expired,
		Active:    len(e.orders),
		Idle:      idle,
		Strategy:  e.strategy.Name(),
	}
	return s
}

func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }
