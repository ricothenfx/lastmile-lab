// order-ingestion — API penerima order (Fase 2).
//
//	POST /orders   validasi → idempotency (Redis SET NX TTL 24h) → Postgres
//	               → publish Kafka `orders` (ack sync) → 201/200
//	GET /metrics   counters pipeline (JSON)
//	GET /healthz   liveness + status komponen (redis/kafka/postgres)
//
// Backpressure: antrean publish berkapasitas terbatas — penuh → 429.
// Akurasi zero-loss: 201 hanya setelah Kafka ack + baris Postgres tertulis.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// Batas koordinat area Berlin (lebih luas dari bbox peta — snap di rider-sim).
const (
	berlinLatMin = 52.30
	berlinLatMax = 52.65
	berlinLonMin = 13.10
	berlinLonMax = 13.70
	minTripM     = 50.0
	maxKeyLen    = 200
)

type OrderRow struct {
	ID             string
	IdempotencyKey string
	PickupLat      float64
	PickupLon      float64
	DropoffLat     float64
	DropoffLon     float64
	CreatedMs      int64
}

type Publisher interface {
	Publish(ctx context.Context, msg model.OrderMsg) error
}

type Store interface {
	InsertOrder(ctx context.Context, r OrderRow) error
	InsertEvent(ctx context.Context, orderID, eventType string, payload []byte) error
	MarkStatus(ctx context.Context, id, status string) error
}

type Idempotency interface {
	// Reserve mengembalikan (existingID, false) bila key sudah ada, atau
	// ("", true) bila orderID baru saja dicatat (first time).
	Reserve(ctx context.Context, key, orderID string) (string, bool, error)
}

type publishJob struct {
	msg  model.OrderMsg
	row  OrderRow
	done chan error
}

type Metrics struct {
	mu           sync.Mutex
	requests     uint64
	replays      uint64
	acked        uint64
	rejected429  uint64
	bad400       uint64
	errors503    uint64
	published    uint64
	publishErr   uint64
	inflight     int64
	lastAckMs    float64
	latencySumMs float64
}

func (m *Metrics) snapshot() map[string]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]uint64{
		"requests":        m.requests,
		"replays":         m.replays,
		"acked_201":       m.acked,
		"rejected_429":    m.rejected429,
		"bad_request_400": m.bad400,
		"errors_503":      m.errors503,
		"published":       m.published,
		"publish_errors":  m.publishErr,
		"inflight":        uint64(m.inflight),
	}
}

func (m *Metrics) addLatency(ms float64) {
	m.mu.Lock()
	m.lastAckMs = ms
	m.latencySumMs += ms
	m.mu.Unlock()
}

type Ingester struct {
	idem    Idempotency
	store   Store
	pub     Publisher
	queue   chan publishJob
	metrics Metrics
	nowMs   func() int64
}

func NewIngester(idem Idempotency, store Store, pub Publisher, queueCap int) *Ingester {
	ing := &Ingester{
		idem:  idem,
		store: store,
		pub:   pub,
		queue: make(chan publishJob, queueCap),
		nowMs: func() int64 { return time.Now().UnixMilli() },
	}
	go ing.writer()
	return ing
}

func (ing *Ingester) writer() {
	for job := range ing.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := ing.pub.Publish(ctx, job.msg)
		cancel()
		if err != nil {
			ing.metrics.mu.Lock()
			ing.metrics.publishErr++
			ing.metrics.mu.Unlock()
			// tandai baris agar tidak masuk hitungan "stored" zero-loss
			mctx, mcancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = ing.store.MarkStatus(mctx, job.row.ID, "publish_failed")
			mcancel()
		} else {
			ing.metrics.mu.Lock()
			ing.metrics.published++
			ing.metrics.mu.Unlock()
		}
		job.done <- err
	}
}

// HandlePOST adalah http.Handler untuk POST /orders.
func (ing *Ingester) HandlePOST(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ing.metrics.mu.Lock()
	ing.metrics.requests++
	ing.metrics.inflight++
	ing.metrics.mu.Unlock()
	defer func() {
		ing.metrics.mu.Lock()
		ing.metrics.inflight--
		ing.metrics.mu.Unlock()
	}()

	if r.Method != http.MethodPost {
		httpError(w, ing, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	var req model.OrderMsg
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, ing, http.StatusBadRequest, "bad_json")
		return
	}
	if !validCoords(req.PickupLat, req.PickupLon) || !validCoords(req.DropoffLat, req.DropoffLon) {
		httpError(w, ing, http.StatusBadRequest, "coords_out_of_berlin")
		return
	}
	dist := dispatchDistance(req.PickupLat, req.PickupLon, req.DropoffLat, req.DropoffLon)
	if dist < minTripM {
		httpError(w, ing, http.StatusBadRequest, "trip_too_short")
		return
	}

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	bodyHash := ""
	if key == "" {
		bodyHash = hashRequest(req)
		key = bodyHash
	}
	if len(key) > maxKeyLen {
		key = hashRequest(req)
	}

	orderID := newOrderID()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	existing, first, err := ing.idem.Reserve(ctx, key, orderID)
	cancel()
	if err != nil {
		log.Printf("idempotency reserve: %v", err)
		httpError(w, ing, http.StatusServiceUnavailable, "idempotency_unavailable")
		return
	}
	if !first {
		ing.metrics.mu.Lock()
		ing.metrics.replays++
		ing.metrics.mu.Unlock()
		w.Header().Set("Idempotent-Replay", "true")
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": existing, "duplicate": true})
		return
	}

	row := OrderRow{
		ID:             orderID,
		IdempotencyKey: key,
		PickupLat:      req.PickupLat,
		PickupLon:      req.PickupLon,
		DropoffLat:     req.DropoffLat,
		DropoffLon:     req.DropoffLon,
		CreatedMs:      ing.nowMs(),
	}
	msg := model.OrderMsg{
		ID:             row.ID,
		IdempotencyKey: key,
		PickupLat:      row.PickupLat,
		PickupLon:      row.PickupLon,
		DropoffLat:     row.DropoffLat,
		DropoffLon:     row.DropoffLon,
		CreatedMs:      row.CreatedMs,
		Source:         "api",
	}

	ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
	err = ing.store.InsertOrder(ctx, row)
	if err == nil {
		payload, _ := json.Marshal(map[string]string{"key": key})
		err = ing.store.InsertEvent(ctx, row.ID, "ingested", payload)
		if err != nil {
			// order utama sudah masuk — event gagal tidak membatalkan ingest
			log.Printf("insert event ingested: %v", err)
			err = nil
		}
	}
	cancel()
	if err != nil {
		log.Printf("insert order: %v", err)
		httpError(w, ing, http.StatusServiceUnavailable, "storage_unavailable")
		return
	}

	job := publishJob{msg: msg, row: row, done: make(chan error, 1)}
	select {
	case ing.queue <- job:
	default:
		ing.metrics.mu.Lock()
		ing.metrics.rejected429++
		ing.metrics.mu.Unlock()
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		_ = ing.store.MarkStatus(ctx, row.ID, "publish_failed")
		cancel()
		httpError(w, ing, http.StatusTooManyRequests, "queue_full")
		return
	}

	// ack hanya setelah Kafka benar-benar menerima (at-least-once, no-loss).
	select {
	case err = <-job.done:
	case <-time.After(10 * time.Second):
		err = errors.New("publish timeout")
	}
	if err != nil {
		log.Printf("publish %s: %v", row.ID, err)
		httpError(w, ing, http.StatusServiceUnavailable, "broker_unavailable")
		return
	}
	ing.metrics.mu.Lock()
	ing.metrics.acked++
	ing.metrics.mu.Unlock()
	ing.metrics.addLatency(float64(time.Since(start).Milliseconds()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": row.ID, "duplicate": false})
}

func httpError(w http.ResponseWriter, ing *Ingester, code int, msg string) {
	ing.metrics.mu.Lock()
	switch code {
	case http.StatusBadRequest:
		ing.metrics.bad400++
	case http.StatusTooManyRequests:
		// rejected429 dicatat di pemanggil (butuh mark DB)
	default:
		ing.metrics.errors503++
	}
	ing.metrics.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func validCoords(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return false
	}
	return lat >= berlinLatMin && lat <= berlinLatMax && lon >= berlinLonMin && lon <= berlinLonMax
}

func dispatchDistance(latA, lonA, latB, lonB float64) float64 {
	const r = 6371000.0
	dLat := (latB - latA) * math.Pi / 180
	dLon := (lonB - lonA) * math.Pi / 180
	la := latA * math.Pi / 180
	lb := latB * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

func hashRequest(req model.OrderMsg) string {
	h := sha256.New()
	fmt.Fprintf(h, "%.6f|%.6f|%.6f|%.6f", req.PickupLat, req.PickupLon, req.DropoffLat, req.DropoffLon)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func newOrderID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("o-%d", time.Now().UnixNano())
	}
	return "o-" + hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func envStr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
