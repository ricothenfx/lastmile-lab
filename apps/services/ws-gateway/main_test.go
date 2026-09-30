package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// httptest server yang meniru rider-sim /internal/state.
func fakeSim() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"t": 1000, "seq": 1,
			"r":  []map[string]interface{}{{"i": 0, "s": 0, "la": 52.5, "lo": 13.4}},
			"o":  []map[string]interface{}{},
			"l":  []map[string]interface{}{},
			"st": map[string]interface{}{"dl": 0, "ex": 0, "ac": 0, "id": 1},
		})
	}))
}

func TestGatewayStreamsSnapshotOverWS(t *testing.T) {
	simSrv := fakeSim()
	defer simSrv.Close()

	h := newHub()
	p := newPoller(simSrv.URL)
	p.runOnce(h) // satu poll manual, tanpa loop goroutine

	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		ch := h.subscribe()
		go writePump(conn, ch, p)
		readPump(conn)
		h.unsubscribe(ch)
	}))
	defer wsSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(wsSrv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var snap struct {
		Seq uint64 `json:"seq"`
		R   []struct {
			I int `json:"i"`
		} `json:"r"`
	}
	if err := json.Unmarshal(msg, &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Seq != 1 || len(snap.R) != 1 || snap.R[0].I != 0 {
		t.Fatalf("snapshot tidak sesuai: %+v", snap)
	}
}

func TestHubSlowClientDropsFrames(t *testing.T) {
	h := newHub()
	ch := h.subscribe()
	// penuhi buffer tanpa membaca
	for i := 0; i < sendBuffer+4; i++ {
		h.broadcast([]byte("x"))
	}
	if h.count() != 1 {
		t.Fatal("klien harus tetap terdaftar (frame dibuang, bukan koneksi)")
	}
	h.unsubscribe(ch)
	if h.count() != 0 {
		t.Fatal("unsubscribe gagal")
	}
}
