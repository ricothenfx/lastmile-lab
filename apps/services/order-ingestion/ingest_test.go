package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// ---- fakes ----

type fakeStore struct {
	mu      sync.Mutex
	orders  map[string]OrderRow
	events  []string
	failIns bool
	marked  []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{orders: map[string]OrderRow{}}
}

func (s *fakeStore) InsertOrder(_ context.Context, r OrderRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failIns {
		return errors.New("db down")
	}
	if _, ok := s.orders[r.ID]; !ok {
		s.orders[r.ID] = r
	}
	return nil
}

func (s *fakeStore) InsertEvent(_ context.Context, orderID, eventType string, _ []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, orderID+":"+eventType)
	return nil
}

func (s *fakeStore) MarkStatus(_ context.Context, id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marked = append(s.marked, id+"="+status)
	return nil
}

type fakePub struct {
	mu    sync.Mutex
	count int
	err   error
	block chan struct{} // bila non-nil: Publish memblokir sampai ditutup
}

func (p *fakePub) Publish(_ context.Context, _ model.OrderMsg) error {
	if p.block != nil {
		<-p.block
	}
	p.mu.Lock()
	p.count++
	p.mu.Unlock()
	return p.err
}

func (p *fakePub) published() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

const okBody = `{"pickup_lat":52.510,"pickup_lon":13.410,"dropoff_lat":52.520,"dropoff_lon":13.430}`

func post(t *testing.T, srv *httptest.Server, key, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/orders", bytes.NewBufferString(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func newTestIngester(t *testing.T, pub *fakePub, store *fakeStore, queueCap int) *Ingester {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ing := NewIngester(&redisIdem{rdb: rdb, ttl: 24 * time.Hour}, store, pub, queueCap)
	ing.nowMs = func() int64 { return 1000 }
	t.Cleanup(func() { _ = rdb.Close() })
	return ing
}

// DoD inti fase 2: dua POST dengan key sama → 1 order.
func TestIdempotencyTwoPostsSameKeyOneOrder(t *testing.T) {
	pub := &fakePub{}
	store := newFakeStore()
	ing := newTestIngester(t, pub, store, 64)
	srv := httptest.NewServer(http.HandlerFunc(ing.HandlePOST))
	defer srv.Close()

	resp1 := post(t, srv, "client-key-1", okBody)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("POST pertama harus 201, dapat %d", resp1.StatusCode)
	}
	var r1 struct {
		ID        string `json:"id"`
		Duplicate bool   `json:"duplicate"`
	}
	json.NewDecoder(resp1.Body).Decode(&r1)

	resp2 := post(t, srv, "client-key-1", okBody)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST kedua harus 200 (replay), dapat %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Idempotent-Replay") != "true" {
		t.Fatal("header Idempotent-Replay wajib ada di replay")
	}
	var r2 struct {
		ID        string `json:"id"`
		Duplicate bool   `json:"duplicate"`
	}
	json.NewDecoder(resp2.Body).Decode(&r2)

	if r1.ID != r2.ID {
		t.Fatalf("id replay harus sama: %s vs %s", r1.ID, r2.ID)
	}
	if r1.Duplicate || !r2.Duplicate {
		t.Fatalf("flag duplicate salah: %+v %+v", r1, r2)
	}
	if n := pub.published(); n != 1 {
		t.Fatalf("hanya boleh 1 pesan ter-publish, dapat %d", n)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.orders) != 1 {
		t.Fatalf("hanya boleh 1 baris order, dapat %d", len(store.orders))
	}
}

// Idempotency tanpa header: key = hash konten.
func TestIdempotencyBodyHash(t *testing.T) {
	pub := &fakePub{}
	store := newFakeStore()
	ing := newTestIngester(t, pub, store, 64)
	srv := httptest.NewServer(http.HandlerFunc(ing.HandlePOST))
	defer srv.Close()

	if resp := post(t, srv, "", okBody); resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST pertama harus 201, dapat %d", resp.StatusCode)
	}
	resp2 := post(t, srv, "", okBody) // body identik, tanpa key
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST body sama tanpa key harus 200 replay, dapat %d", resp2.StatusCode)
	}
	if n := pub.published(); n != 1 {
		t.Fatalf("hash body yang sama harus replay (1 publish), dapat %d", n)
	}
}

// Backpressure: antrean penuh → 429, tidak ada publish tambahan.
func TestBackpressure429(t *testing.T) {
	pub := &fakePub{block: make(chan struct{})}
	store := newFakeStore()
	ing := newTestIngester(t, pub, store, 1)
	srv := httptest.NewServer(http.HandlerFunc(ing.HandlePOST))
	defer srv.Close()

	post(t, srv, "k1", okBody) // mengisi antrean + blok di writer
	post(t, srv, "k2", okBody) // menunggu di job kedua
	resp := post(t, srv, "k3", okBody)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("antrean penuh harus 429, dapat %d", resp.StatusCode)
	}
	close(pub.block) // luruskan lagi
}

// Kafka gagal → 503, order tidak dihitung acked.
func TestPublishError503(t *testing.T) {
	pub := &fakePub{err: errors.New("broker down")}
	store := newFakeStore()
	ing := newTestIngester(t, pub, store, 8)
	srv := httptest.NewServer(http.HandlerFunc(ing.HandlePOST))
	defer srv.Close()

	resp := post(t, srv, "kx", okBody)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("publish gagal harus 503, dapat %d", resp.StatusCode)
	}
	if m := ing.metrics.snapshot(); m["acked_201"] != 0 {
		t.Fatal("tidak boleh ada acked saat publish gagal")
	}
}

func TestValidation400(t *testing.T) {
	pub := &fakePub{}
	store := newFakeStore()
	ing := newTestIngester(t, pub, store, 8)
	srv := httptest.NewServer(http.HandlerFunc(ing.HandlePOST))
	defer srv.Close()

	if resp := post(t, srv, "v1", `{"pickup_lat":10,"pickup_lon":13.4,"dropoff_lat":52.52,"dropoff_lon":13.43}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatal("koordinat di luar Berlin harus 400")
	}
	if resp := post(t, srv, "v2", `{bukan json`); resp.StatusCode != http.StatusBadRequest {
		t.Fatal("json rusak harus 400")
	}
	if resp := post(t, srv, "v3", `{"pickup_lat":52.510,"pickup_lon":13.410,"dropoff_lat":52.510,"dropoff_lon":13.4101}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatal("trip < 50 m harus 400")
	}
}

func TestHashRequestStable(t *testing.T) {
	a := hashRequest(model.OrderMsg{PickupLat: 52.5, PickupLon: 13.4, DropoffLat: 52.52, DropoffLon: 13.42})
	b := hashRequest(model.OrderMsg{PickupLat: 52.5, PickupLon: 13.4, DropoffLat: 52.52, DropoffLon: 13.42})
	c := hashRequest(model.OrderMsg{PickupLat: 52.5, PickupLon: 13.4, DropoffLat: 52.6, DropoffLon: 13.42})
	if a != b || a == c {
		t.Fatal("hash body harus stabil & sensitif konten")
	}
	if len(a) != 24 {
		t.Fatalf("panjang hash 24, dapat %d", len(a))
	}
}
