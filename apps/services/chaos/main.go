// chaos — chaos injector + Incident Timeline API (Fase 4, jaringan internal).
//
//	GET  /healthz              liveness
//	GET  /state                target allowlist + status kesehatan live
//	POST /kill   {"target":"rider-sim"}   → SIGKILL container (allowlist ketat)
//	GET  /incidents             timeline incident + ringkasan MTTD/MTTR/budget
//
// KEAMANAN (aturan keras, lihat ADR D19):
//   - Allowlist EKSPLISIT tujuh service stateless `lastmile-*` di bawah —
//     tanpa wildcard. Infra ber-state (postgres/redis/redpanda), chaos itu
//     sendiri, dan semua container di luar project TIDAK PERNAH disentuh.
//   - Kill = SIGKILL ke PID 1 DI DALAM container via Docker exec API
//     (perintah fixed `kill -9 1`). Endpoint `/containers/{id}/kill` Docker
//     29 sengaja TIDAK dipakai: ia tidak memicu restart policy (terverifikasi
//     di VPS), sedangkan crash PID 1 dari dalam ya — self-heal via restart
//     policy `unless-stopped` adalah perilaku yang justru diukur; chaos tidak
//     pernah me-restart manual.
//   - Monitor 1 Hz + flap suppression 2 kegagalan beruntun membuka & menutup
//     incident: MTTD = t_detect − t_start, MTTR = t_recover − t_detect (unix ms).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// ---- allowlist eksplisit (jangan pernah wildcard) ----

type chaosTarget struct {
	Name      string `json:"name"`      // key publik (mis. "rider-sim")
	Container string `json:"container"` // nama container persis
	HealthURL string `json:"health_url"`
}

// allowlist_targets hanya service STATELESS. Infra ber-state & job one-shot
// sengaja di luar: merestart-nya bukan chaos, tapi risiko korupsi data.
var allowlistTargets = []chaosTarget{
	{Name: "rider-sim", Container: "lastmile-rider-sim", HealthURL: "http://rider-sim:4201/healthz"},
	{Name: "ws-gateway", Container: "lastmile-ws-gateway", HealthURL: "http://ws-gateway:3012/healthz"},
	{Name: "api-gateway", Container: "lastmile-api-gateway", HealthURL: "http://api-gateway:3010/healthz"},
	{Name: "sim-control", Container: "lastmile-sim-control", HealthURL: "http://sim-control:3013/healthz"},
	{Name: "order-ingestion", Container: "lastmile-order-ingestion", HealthURL: "http://order-ingestion:4202/healthz"},
	{Name: "dispatch-consumer", Container: "lastmile-dispatch-consumer", HealthURL: "http://dispatch-consumer:4203/healthz"},
	{Name: "strategy-lab", Container: "lastmile-strategy-lab", HealthURL: "http://strategy-lab:4205/healthz"},
}

// failThreshold = kegagalan healthz beruntun sebelum incident "health"
// dibuka (flap suppression — satu probe timeout saat load host tinggi
// bukan outage). Incident chaos-kill tidak lewat ambang ini (kill sudah
// diterbitkan eksplisit).
const failThreshold = 2

func targetByName(name string) (chaosTarget, bool) {
	for _, t := range allowlistTargets {
		if t.Name == name {
			return t, true
		}
	}
	return chaosTarget{}, false
}

// ---- incident ----

// Incident mengikuti skema spec fase 4: {id, t_start, t_detect, t_recover,
// kind, detail} (+target). MTTD = t_detect−t_start, MTTR = t_recover−t_detect.
// Timestamp unix ms; 0 = belum terjadi.
type Incident struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Kind     string `json:"kind"` // "chaos-kill" | "health"
	TStart   int64  `json:"t_start"`
	TDetect  int64  `json:"t_detect"`
	TRecover int64  `json:"t_recover"`
	Detail   string `json:"detail"`
}

func (i Incident) MTTDMs() int64 {
	if i.TDetect <= 0 {
		return 0
	}
	return i.TDetect - i.TStart
}

func (i Incident) MTTRMs() int64 {
	if i.TRecover <= 0 {
		return 0
	}
	return i.TRecover - i.TDetect
}

// tracker: mesin status kesehatan per target + daftar incident (bounded).
type tracker struct {
	mu        sync.Mutex
	period    time.Duration
	startedAt time.Time
	states    map[string]*targetState
	incidents []*Incident // terbaru dulu, maks maxIncidents
	maxInc    int
	now       func() time.Time
}

type targetState struct {
	t             chaosTarget
	everHealthy   bool
	healthy       bool
	lastHealthyAt time.Time
	open          *Incident
	lastProbeOK   bool
	lastProbeAt   time.Time
	failStreak    int
}

func newTracker(period time.Duration, now func() time.Time) *tracker {
	if now == nil {
		now = time.Now
	}
	tr := &tracker{
		period: period, startedAt: now(), states: map[string]*targetState{},
		maxInc: 200, now: now,
	}
	for _, t := range allowlistTargets {
		tr.states[t.Name] = &targetState{t: t}
	}
	return tr
}

// probe mencatat hasil satu probe kesehatan; return incident yang baru
// ditutup (recovery), kalau ada.
func (tr *tracker) probe(name string, ok bool) *Incident {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	st, exists := tr.states[name]
	if !exists {
		return nil
	}
	now := tr.now()
	st.lastProbeOK, st.lastProbeAt = ok, now
	if ok {
		st.failStreak = 0
		if st.open != nil {
			// pulih. Bila t_detect belum terisi (restart lebih cepat dari
			// probe — kasus langka), deteksi & pemulihan dianggap bersamaan.
			if st.open.TDetect <= 0 {
				st.open.TDetect = now.UnixMilli()
			}
			st.open.TRecover = now.UnixMilli()
			closed := st.open
			st.open = nil
			st.healthy, st.everHealthy, st.lastHealthyAt = true, true, now
			return closed
		}
		st.healthy, st.everHealthy, st.lastHealthyAt = true, true, now
		return nil
	}
	// gagal probe
	if !st.everHealthy {
		return nil // belum pernah hidup → standby (pipeline mati), bukan incident
	}
	if st.open == nil {
		// Flap suppression: butuh N kegagalan beruntun sebelum incident
		// dibuka — satu probe timeout saat load host tinggi bukan outage.
		st.failStreak++
		if st.failStreak < failThreshold {
			st.healthy = false
			return nil
		}
		st.open = &Incident{
			ID:      newIncidentID(now, name),
			Target:  name,
			Kind:    "health",
			TStart:  st.lastHealthyAt.UnixMilli(),
			TDetect: now.UnixMilli(),
			Detail:  "healthz gagal " + strconv.Itoa(failThreshold) + "× beruntun (monitor " + tr.period.String() + ")",
		}
		tr.push(st.open)
	} else if st.open.TDetect <= 0 {
		// incident chaos-kill: monitor baru saja melihat kegagalan pertama.
		st.open.TDetect = now.UnixMilli()
	}
	st.healthy = false
	return nil
}

// openChaos membuka incident chaos-kill (t_start = saat kill diterbitkan);
// t_detect/t_recover mengikuti monitor. Return error bila target tidak
// sehat (sudah down / belum pernah hidup).
func (tr *tracker) openChaos(name, detail string) (*Incident, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	st, exists := tr.states[name]
	if !exists {
		return nil, errDenied
	}
	if !st.everHealthy || !st.healthy {
		return nil, errNotHealthy
	}
	now := tr.now()
	inc := &Incident{
		ID: newIncidentID(now, name), Target: name, Kind: "chaos-kill",
		TStart: now.UnixMilli(), Detail: detail,
	}
	st.open = inc
	st.healthy = false
	st.lastHealthyAt = now
	tr.push(inc)
	return inc, nil
}

func (tr *tracker) push(inc *Incident) {
	tr.incidents = append([]*Incident{inc}, tr.incidents...)
	if len(tr.incidents) > tr.maxInc {
		tr.incidents = tr.incidents[:tr.maxInc]
	}
}

// snapshot status target untuk /state & /incidents.
type targetView struct {
	Name           string `json:"name"`
	Container      string `json:"container"`
	HealthURL      string `json:"health_url"`
	Status         string `json:"status"` // healthy|down|standby (never seen)
	LastOKMs       int64  `json:"last_ok_ms,omitempty"`
	LastProbeAgoMs int64  `json:"last_probe_ago_ms"`
}

func (tr *tracker) targets() []targetView {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]targetView, 0, len(allowlistTargets))
	now := tr.now()
	for _, t := range allowlistTargets {
		st := tr.states[t.Name]
		v := targetView{Name: t.Name, Container: t.Container, HealthURL: t.HealthURL}
		switch {
		case st.everHealthy && st.healthy:
			v.Status = "healthy"
		case st.everHealthy:
			v.Status = "down"
		default:
			v.Status = "standby"
		}
		if !st.lastHealthyAt.IsZero() {
			v.LastOKMs = st.lastHealthyAt.UnixMilli()
		}
		if !st.lastProbeAt.IsZero() {
			v.LastProbeAgoMs = now.Sub(st.lastProbeAt).Milliseconds()
		}
		out = append(out, v)
	}
	return out
}

func (tr *tracker) incidentList() []*Incident {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]*Incident, len(tr.incidents))
	copy(out, tr.incidents)
	return out
}

// summary ringkasan untuk error budget. Availability = 1 − downtime/service
// selama window (denominator hanya target yang pernah hidup — standby tidak
// menghukum angka).
type summaryView struct {
	WindowStartMs    int64   `json:"window_start_ms"`
	WindowSec        float64 `json:"window_sec"`
	Total            int     `json:"incidents_total"`
	Open             int     `json:"incidents_open"`
	Kills            int     `json:"chaos_kills"`
	MTTDAvgMs        float64 `json:"mttd_avg_ms"`
	MTTRAvgMs        float64 `json:"mttr_avg_ms"`
	DowntimeTotalSec float64 `json:"downtime_total_sec"`
	MonitoredTargets int     `json:"monitored_targets"`
	AvailabilityPct  float64 `json:"availability_pct"`
	BudgetPct        float64 `json:"budget_pct"`
	BudgetOK         bool    `json:"budget_ok"`
}

func (tr *tracker) summary(budgetPct float64) summaryView {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	now := tr.now()
	s := summaryView{
		WindowStartMs: tr.startedAt.UnixMilli(),
		WindowSec:     now.Sub(tr.startedAt).Seconds(),
		BudgetPct:     budgetPct,
	}
	if s.WindowSec < 1 {
		s.WindowSec = 1
	}
	monitored := 0
	for _, st := range tr.states {
		if st.everHealthy {
			monitored++
		}
	}
	s.MonitoredTargets = monitored
	var mttds, mttrs []float64
	for _, inc := range tr.incidents {
		s.Total++
		if inc.Kind == "chaos-kill" {
			s.Kills++
		}
		if inc.TRecover > 0 {
			mttds = append(mttds, float64(inc.MTTDMs()))
			mttrs = append(mttrs, float64(inc.MTTRMs()))
			s.DowntimeTotalSec += float64(inc.MTTRMs()) / 1000
		} else {
			s.Open++
			// downtime berjalan: dari deteksi sampai sekarang
			detect := inc.TDetect
			if detect <= 0 {
				detect = now.UnixMilli()
			}
			s.DowntimeTotalSec += float64(now.UnixMilli()-detect) / 1000
		}
	}
	if len(mttds) > 0 {
		s.MTTDAvgMs = avg(mttds)
	}
	if len(mttrs) > 0 {
		s.MTTRAvgMs = avg(mttrs)
	}
	if monitored > 0 {
		s.AvailabilityPct = 100 * (1 - s.DowntimeTotalSec/(s.WindowSec*float64(monitored)))
		if s.AvailabilityPct < 0 {
			s.AvailabilityPct = 0
		}
	} else {
		s.AvailabilityPct = 100
	}
	s.BudgetOK = s.AvailabilityPct >= 100-budgetPct
	return s
}

func avg(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func newIncidentID(t time.Time, target string) string {
	return fmt.Sprintf("inc-%s-%s", strconv.FormatInt(t.UnixMilli(), 36), target)
}

// ---- docker client (unix socket, tanpa dependency SDK) ----

type dockerClient struct {
	c *http.Client
}

func newDockerClient(sock string) *dockerClient {
	return &dockerClient{c: &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}}
}

// inspectRunning memastikan container ada & berjalan sebelum kill.
func (d *dockerClient) inspectRunning(container string) (bool, error) {
	resp, err := d.c.Get("http://docker/containers/" + container + "/json")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("docker inspect: HTTP %d", resp.StatusCode)
	}
	var st struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return false, err
	}
	return st.State.Running, nil
}

// kill mengirim SIGKILL ke PID 1 DI DALAM container via Docker exec API.
//
// PENTING (perilaku Docker 25+ di host ini terverifikasi): endpoint
// `/containers/{id}/kill` TIDAK memicu restart policy — container dianggap
// dihentikan manual dan `unless-stopped` membiarkannya mati. Crash PID 1
// dari dalam container justru memicu restart policy sungguhan (self-heal
// yang mau diukur) sekaligus lebih otentik sebagai kegagalan proses.
// Perintah exec FIXED (`kill -9 1`) — bukan pintu exec arbitrer.
func (d *dockerClient) kill(container string) error {
	b := strings.NewReader(`{"AttachOutput":false,"Cmd":["/bin/sh","-c","kill -9 1"]}`)
	req, err := http.NewRequest(http.MethodPost,
		"http://docker/containers/"+container+"/exec", b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("docker exec create: HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
		return fmt.Errorf("docker exec create: decode")
	}
	startReq, err := http.NewRequest(http.MethodPost,
		"http://docker/exec/"+created.ID+"/start", strings.NewReader(`{"Detach":true}`))
	if err != nil {
		return err
	}
	startReq.Header.Set("Content-Type", "application/json")
	startResp, err := d.c.Do(startReq)
	if err != nil {
		return err
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK && startResp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("docker exec start: HTTP %d", startResp.StatusCode)
	}
	return nil
}

// ---- penyimpanan incident (persist best-effort) ----

type incidentStore struct {
	mu   sync.Mutex
	path string
}

func (s *incidentStore) save(list []*Incident) {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(list)
	if err != nil {
		log.Printf("persist marshal: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("persist write: %v", err)
		return
	}
	_ = os.Rename(tmp, s.path)
}

func (s *incidentStore) load() []*Incident {
	if s.path == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var list []*Incident
	if err := json.Unmarshal(b, &list); err != nil {
		log.Printf("persisted incidents rusak, diabaikan: %v", err)
		return nil
	}
	return list
}

// ---- service ----

var (
	errDenied     = errors.New("target di luar allowlist")
	errNotHealthy = errors.New("target tidak sehat (sudah down / belum pernah hidup)")
)

type service struct {
	tr     *tracker
	docker *dockerClient
	store  *incidentStore
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *service) mux() *http.ServeMux {
	mux := http.NewServeMux()
	start := time.Now()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, model.Health{
			OK: true, Service: "chaos",
			UptimeSec: int64(time.Since(start).Seconds()),
			Detail:    fmt.Sprintf("targets=%d incidents=%d", len(allowlistTargets), len(s.tr.incidentList())),
		})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"targets": s.tr.targets(),
		})
	})
	mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		list := s.tr.incidentList()
		sort.Slice(list, func(i, j int) bool { return list[i].TStart > list[j].TStart })
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"incidents": list,
			"summary":   s.tr.summary(0.1), // budget 0,1% (SLO availability 99,9%)
		})
	})
	mux.HandleFunc("/kill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		var req struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_json — butuh target"})
			return
		}
		t, ok := targetByName(req.Target)
		if !ok {
			// DITOLAK — apapun namanya, hanya allowlist eksplisit yang lolos.
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "target_outside_allowlist", "target": req.Target,
			})
			return
		}
		running, err := s.docker.inspectRunning(t.Container)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "docker_unreachable"})
			return
		}
		if !running {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "container_not_running", "container": t.Container,
			})
			return
		}
		inc, err := s.tr.openChaos(t.Name, "SIGKILL oleh chaos injector ("+t.Container+")")
		if err != nil {
			code := http.StatusConflict
			if errors.Is(err, errDenied) {
				code = http.StatusForbidden
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		if err := s.docker.kill(t.Container); err != nil {
			// kill gagal — jangan biarkan incident palsu menggantung.
			s.tr.probe(t.Name, true)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "kill_failed: " + err.Error()})
			return
		}
		s.store.save(s.tr.incidentList())
		log.Printf("chaos: SIGKILL %s (incident %s)", t.Container, inc.ID)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"accepted": true, "target": t.Name, "container": t.Container, "incident": inc.ID,
		})
	})
	return mux
}

// runMonitor memulai satu goroutine probe per target (1 Hz, staggered).
func runMonitor(ctx context.Context, tr *tracker, period, timeout time.Duration, onClose func(*Incident)) {
	for i, t := range allowlistTargets {
		go func(t chaosTarget, offset int) {
			tick := time.NewTicker(period)
			// stagger supaya probe tidak menumpuk di milidetik yang sama
			select {
			case <-ctx.Done():
				tick.Stop()
				return
			case <-time.After(time.Duration(offset) * (period / time.Duration(len(allowlistTargets)+1))):
			}
			client := &http.Client{Timeout: timeout}
			for {
				select {
				case <-ctx.Done():
					tick.Stop()
					return
				case <-tick.C:
					ok := false
					if resp, err := client.Get(t.HealthURL); err == nil {
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						ok = resp.StatusCode == http.StatusOK
					}
					if closed := tr.probe(t.Name, ok); closed != nil && onClose != nil {
						onClose(closed)
					}
				}
			}
		}(t, i)
	}
}

func main() {
	port := envStr("PORT", "4206")
	dataDir := strings.TrimSuffix(envStr("DATA_DIR", ""), "/")
	sock := envStr("DOCKER_SOCK", "/var/run/docker.sock")
	periodMs := envInt("PROBE_PERIOD_MS", 1000)
	timeoutMs := envInt("PROBE_TIMEOUT_MS", 700)

	tr := newTracker(time.Duration(periodMs)*time.Millisecond, nil)
	store := &incidentStore{}
	if dataDir != "" {
		store.path = filepath.Join(dataDir, "incidents.json")
		for _, inc := range store.load() {
			if inc != nil && inc.TRecover <= 0 {
				// incident terbuka saat chaos restart — monitor akan menutupnya
				tr.probe(inc.Target, false)
				tr.states[inc.Target].open = inc
			}
		}
	}
	svc := &service{tr: tr, docker: newDockerClient(sock), store: store}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runMonitor(ctx, tr, time.Duration(periodMs)*time.Millisecond,
		time.Duration(timeoutMs)*time.Millisecond,
		func(closed *Incident) {
			log.Printf("chaos: incident %s (%s) pulih — MTTR %.1fs",
				closed.ID, closed.Target, float64(closed.MTTRMs())/1000)
			store.save(tr.incidentList())
		})

	addr := "0.0.0.0:" + port
	log.Printf("chaos listening on %s (sock %s, probe %dms, data %q)",
		addr, sock, periodMs, dataDir)
	srv := &http.Server{Addr: addr, Handler: svc.mux(), ReadHeaderTimeout: 10 * time.Second}
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
