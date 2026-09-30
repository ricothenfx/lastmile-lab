// ws-gateway — broadcast snapshot simulasi ke frontend via WebSocket.
//
// Polling rider-sim (internal network) pada ~10 Hz lalu fan-out ke semua
// klien WebSocket tanpa agregasi berat: satu poll untuk N klien.
//
//	GET /ws        WebSocket stream snapshot JSON
//	GET /healthz   liveness + status sim + jumlah klien
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
	sendBuffer = 4
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true }, // stream publik read-only
}

type hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	drops   uint64
}

func newHub() *hub { return &hub{clients: make(map[chan []byte]struct{})} }

func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, sendBuffer)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

func (h *hub) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// klien lambat: buang frame ini (lebih baik daripada menumpuk buffer)
			h.drops++
		}
	}
}

func (h *hub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

type poller struct {
	url    string
	client *http.Client
	mu     sync.RWMutex
	latest []byte
	seq    uint64
	simUp  bool
	lastOK time.Time
}

func newPoller(url string) *poller {
	return &poller{
		url:    url,
		client: &http.Client{Timeout: 2 * time.Second},
	}
}

func (p *poller) run(h *hub, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		p.runOnce(h)
	}
}

func (p *poller) runOnce(h *hub) {
	resp, err := p.client.Get(p.url + "/internal/state")
	if err != nil {
		p.mu.Lock()
		p.simUp = false
		p.mu.Unlock()
		return
	}
	var snap model.Snapshot
	err = json.NewDecoder(resp.Body).Decode(&snap)
	resp.Body.Close()
	if err != nil {
		p.mu.Lock()
		p.simUp = false
		p.mu.Unlock()
		return
	}
	buf, err := json.Marshal(snap)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.latest = buf
	p.seq = snap.Seq
	p.simUp = true
	p.lastOK = time.Now()
	p.mu.Unlock()
	h.broadcast(buf)
}

func (p *poller) status() (up bool, seq uint64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.simUp, p.seq
}

func (p *poller) lastFrame() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.latest
}

func main() {
	port := envStr("PORT", "3012")
	simURL := envStr("RIDER_SIM_URL", "http://127.0.0.1:4201")
	pollMs := envInt("POLL_MS", 100)

	h := newHub()
	p := newPoller(simURL)
	go p.run(h, time.Duration(pollMs)*time.Millisecond)

	mux := http.NewServeMux()
	start := time.Now()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		up, seq := p.status()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"service":    "ws-gateway",
			"uptime_sec": int64(time.Since(start).Seconds()),
			"sim_ok":     up,
			"last_seq":   seq,
			"clients":    h.count(),
		})
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade: %v", err)
			return
		}
		ch := h.subscribe()
		go writePump(conn, ch, p)
		readPump(conn)
		h.unsubscribe(ch)
	})

	addr := "0.0.0.0:" + port
	log.Printf("ws-gateway listening on %s (sim %s, poll %dms)", addr, simURL, pollMs)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// writePump mengirim frame snapshot + ping keepalive ke satu klien.
func writePump(conn *websocket.Conn, ch chan []byte, p *poller) {
	defer conn.Close()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	if last := p.lastFrame(); last != nil {
		// kirim snapshot terakhir segera agar UI langsung hidup
		_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteMessage(websocket.TextMessage, last); err != nil {
			return
		}
	}
	for {
		select {
		case msg, ok := <-ch:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump menjaga koneksi tetap hidup (pong) sampai klien tutup.
func readPump(conn *websocket.Conn) {
	defer conn.Close()
	conn.SetReadLimit(512)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
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
