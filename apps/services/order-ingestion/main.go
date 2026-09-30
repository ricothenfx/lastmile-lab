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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// ---- Redis idempotency (SET NX + TTL 24h) ----

type redisIdem struct {
	rdb *redis.Client
	ttl time.Duration
}

func (r *redisIdem) Reserve(ctx context.Context, key, orderID string) (string, bool, error) {
	k := "idem:" + key
	ok, err := r.rdb.SetNX(ctx, k, orderID, r.ttl).Result()
	if err != nil {
		return "", false, err
	}
	if ok {
		return "", true, nil
	}
	val, err := r.rdb.Get(ctx, k).Result()
	if err == redis.Nil {
		// key expired di antara SetNX dan Get (jendela sangat kecil) — coba sekali lagi
		ok2, err2 := r.rdb.SetNX(ctx, k, orderID, r.ttl).Result()
		if err2 != nil {
			return "", false, err2
		}
		if ok2 {
			return "", true, nil
		}
		val, err = r.rdb.Get(ctx, k).Result()
	}
	if err != nil {
		return "", false, err
	}
	return val, false, nil
}

// ---- Postgres store ----

type pgStore struct {
	pool *pgxpool.Pool
}

func (s *pgStore) InsertOrder(ctx context.Context, r OrderRow) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO orders
		(id, idempotency_key, pickup_lat, pickup_lon, dropoff_lat, dropoff_lon, created_ms, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'received')
		ON CONFLICT (id) DO NOTHING`,
		r.ID, r.IdempotencyKey, r.PickupLat, r.PickupLon, r.DropoffLat, r.DropoffLon, r.CreatedMs)
	return err
}

func (s *pgStore) InsertEvent(ctx context.Context, orderID, eventType string, payload []byte) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO order_events (order_id, event_type, payload)
		VALUES ($1,$2,$3) ON CONFLICT (order_id, event_type) DO NOTHING`,
		orderID, eventType, payload)
	return err
}

func (s *pgStore) MarkStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE orders SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// ---- Kafka publisher (ack sync — 201 hanya setelah broker menerima) ----

type kafkaPub struct {
	cl    *kgo.Client
	topic string
}

func (p *kafkaPub) Publish(ctx context.Context, msg model.OrderMsg) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.cl.ProduceSync(ctx,
		&kgo.Record{Topic: p.topic, Key: []byte(msg.ID), Value: b}).FirstErr()
}

// ---- status komponen (cached probe untuk /healthz & /metrics) ----

type componentStatus struct {
	mu     sync.Mutex
	status map[string]string
}

func main() {
	port := envStr("PORT", "4202")
	redisAddr := envStr("REDIS_ADDR", "127.0.0.1:6380")
	brokers := strings.Split(envStr("KAFKA_BROKERS", "127.0.0.1:19092"), ",")
	topic := envStr("KAFKA_TOPIC", "orders")
	dbURL := envStr("DATABASE_URL", "postgres://lastmile:postgres@127.0.0.1:5434/lastmile?sslmode=disable")
	queueCap := envInt("QUEUE_CAPACITY", 8192)
	idemTTL := time.Duration(envFloat("IDEM_TTL_HOURS", 24)) * time.Hour

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, DialTimeout: 2 * time.Second})
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}
	kcl, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		// default franz-go: producer idempotent + acks=all — guarantee terkuat
		// untuk zero-loss (201 hanya setelah broker majoritas menulis).
	)
	if err != nil {
		log.Fatalf("kafka client: %v", err)
	}

	ing := NewIngester(&redisIdem{rdb: rdb, ttl: idemTTL}, &pgStore{pool: pool},
		&kafkaPub{cl: kcl, topic: topic}, queueCap)

	status := &componentStatus{status: map[string]string{}}
	go func() { // probe komponen tiap 5 detik (cached — /healthz murah)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			st := map[string]string{}
			if err := rdb.Ping(ctx).Err(); err != nil {
				st["redis"] = "down"
			} else {
				st["redis"] = "ok"
			}
			if err := pool.Ping(ctx); err != nil {
				st["postgres"] = "down"
			} else {
				st["postgres"] = "ok"
			}
			if err := kcl.Ping(ctx); err != nil {
				st["kafka"] = "down"
			} else {
				st["kafka"] = "ok"
			}
			cancel()
			status.mu.Lock()
			status.status = st
			status.mu.Unlock()
			time.Sleep(5 * time.Second)
		}
	}()

	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/orders", ing.HandlePOST)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status.mu.Lock()
		st := status.status
		status.mu.Unlock()
		writeJSON(w, http.StatusOK, model.Health{
			OK: true, Service: "order-ingestion",
			UptimeSec: int64(time.Since(start).Seconds()),
			Detail:    "queue=" + strconv.Itoa(queueCap),
		})
		_ = st // readiness detail diekspos via /metrics; liveness tetap ok
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		status.mu.Lock()
		st := status.status
		status.mu.Unlock()
		m := ing.metrics.snapshot()
		ing.metrics.mu.Lock()
		m["last_ack_ms"] = uint64(ing.metrics.lastAckMs)
		ing.metrics.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service": "order-ingestion", "components": st, "counters": m,
		})
	})

	addr := "0.0.0.0:" + port
	log.Printf("order-ingestion listening on %s (redis %s, kafka %v, topic %s, queue %d)",
		addr, redisAddr, brokers, topic, queueCap)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
