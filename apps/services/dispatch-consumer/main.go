package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// ---- SimClient via HTTP internal rider-sim ----

type httpSim struct {
	url string
	hc  *http.Client
}

func (s *httpSim) IdleRiders(ctx context.Context) ([]dispatch.RiderView, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+"/internal/state", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{status: resp.StatusCode, path: "/internal/state"}
	}
	var snap model.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, err
	}
	out := make([]dispatch.RiderView, 0, len(snap.Riders))
	for _, r := range snap.Riders {
		if model.RiderStatus(r.S) == model.RiderIdle {
			out = append(out, dispatch.RiderView{
				ID: r.ID, Status: model.RiderIdle, Lat: r.Lat, Lon: r.Lon,
			})
		}
	}
	return out, nil
}

func (s *httpSim) Inject(ctx context.Context, o model.ExternalOrder) (string, int, error) {
	return postInject(ctx, s.hc, s.url, o)
}

// ---- DB via pgx ----

type pgDB struct {
	pool *pgxpool.Pool
}

func (d *pgDB) UpsertOrder(ctx context.Context, m model.OrderMsg) error {
	_, err := d.pool.Exec(ctx, `INSERT INTO orders
		(id, idempotency_key, pickup_lat, pickup_lon, dropoff_lat, dropoff_lon, created_ms, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'received')
		ON CONFLICT (id) DO NOTHING`,
		m.ID, m.IdempotencyKey, m.PickupLat, m.PickupLon, m.DropoffLat, m.DropoffLon, m.CreatedMs)
	return err
}

func (d *pgDB) EventIngested(ctx context.Context, orderID, key string) error {
	payload, _ := json.Marshal(map[string]string{"key": key})
	_, err := d.pool.Exec(ctx, `INSERT INTO order_events (order_id, event_type, payload)
		VALUES ($1,'ingested',$2) ON CONFLICT (order_id, event_type) DO NOTHING`,
		orderID, payload)
	return err
}

func (d *pgDB) SetAssigned(ctx context.Context, orderID string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE orders SET status='assigned', updated_at=now() WHERE id=$1`, orderID)
	return err
}

func (d *pgDB) EventAssigned(ctx context.Context, orderID string, riderID int, distM float64, strategy string) error {
	payload, _ := json.Marshal(map[string]interface{}{"rider": riderID, "dist_m": distM, "strategy": strategy})
	_, err := d.pool.Exec(ctx, `INSERT INTO order_events (order_id, event_type, payload)
		VALUES ($1,'assigned',$2) ON CONFLICT (order_id, event_type) DO NOTHING`,
		orderID, payload)
	return err
}

func main() {
	port := envStr("PORT", "4203")
	brokers := strings.Split(envStr("KAFKA_BROKERS", "127.0.0.1:19092"), ",")
	topic := envStr("KAFKA_TOPIC", "orders")
	group := envStr("KAFKA_GROUP", "dispatch-consumer")
	simURL := envStr("RIDER_SIM_URL", "http://127.0.0.1:4201")
	dbURL := envStr("DATABASE_URL", "postgres://lastmile:postgres@127.0.0.1:5434/lastmile?sslmode=disable")
	batchMax := envInt("BATCH_MAX", 200)

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(), // commit manual setelah batch sukses (at-least-once)
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.FetchMaxWait(200*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("kafka client: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}

	// Strategi dispatch pipeline (Fase 3) — default fifo, perilaku lama utuh.
	stratName := envStr("DISPATCH_STRATEGY", "fifo")
	strategy, err := dispatch.ByName(stratName)
	if err != nil {
		log.Fatalf("DISPATCH_STRATEGY: %v", err)
	}

	runner := NewRunner(cl,
		&httpSim{url: simURL, hc: &http.Client{Timeout: 3 * time.Second}},
		&pgDB{pool: pool},
		batchMax, strategy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runner.Run(ctx)

	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		runner.cnt.mu.Lock()
		last := runner.cnt.lastCommit
		runner.cnt.mu.Unlock()
		ageMs := -1
		if last > 0 {
			ageMs = int(time.Now().UnixMilli() - last)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": ctx.Err() == nil, "service": "dispatch-consumer",
			"uptime_sec":         int64(time.Since(start).Seconds()),
			"last_commit_age_ms": ageMs, "group": group, "topic": topic,
			"strategy": stratName,
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service": "dispatch-consumer", "counters": runner.cnt.snap(),
		})
	})

	addr := "0.0.0.0:" + port
	log.Printf("dispatch-consumer listening on %s (kafka %v topic %s group %s sim %s)",
		addr, brokers, topic, group, simURL)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
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

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
