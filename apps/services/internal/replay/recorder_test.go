package replay

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

func snap(t int64, riders int) model.Snapshot {
	s := model.Snapshot{T: t, Seq: uint64(t), Riders: make([]model.RiderPt, 0, riders)}
	for i := 0; i < riders; i++ {
		s.Riders = append(s.Riders, model.RiderPt{ID: i, S: i % 4, Lat: 52.51 + float64(i)*1e-5, Lon: 13.41 + float64(i)*1e-5})
	}
	s.Stats = model.Stats{Delivered: int(t / 1000), Active: 3, Idle: riders - 3, Strategy: "fifo", SurgeFactor: 1, WeatherFactor: 1}
	return s
}

func gunzipAll(t *testing.T, b []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	out, err := io.ReadAll(r) // ReadAll meneruskan ke anggota berikutnya (multi-member)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return out
}

func TestRecordRingEviction(t *testing.T) {
	r := New(Meta{Seed: 7, Strategy: "fifo", Riders: 10, TickHz: 10})
	r.maxFrames = 10
	r.maxBytes = 1 << 30
	for i := 0; i < 25; i++ {
		r.Record(snap(int64(i)*200, 10), 1000+int64(i)*200)
	}
	info := r.SessionInfo()
	if info.Frames != 10 {
		t.Fatalf("frames = %d, want 10 (ring bounded)", info.Frames)
	}
	if info.TFirst != int64(15)*200 {
		t.Fatalf("TFirst = %d, want %d (FIFO evict tertua)", info.TFirst, 15*200)
	}
	if info.Dropped != 15 {
		t.Fatalf("dropped = %d, want 15", info.Dropped)
	}
	if info.TLast-info.TFirst != int64(9)*200 {
		t.Fatalf("span = %d, rentang frame tidak kontigu", info.TLast-info.TFirst)
	}
}

func TestRecordByteBudgetEviction(t *testing.T) {
	r := New(Meta{})
	r.maxFrames = 1 << 20
	big := snap(0, 400) // frame besar → pagar byte yang kena
	r.Record(big, 0)
	per := r.SessionInfo().FrameBytes
	if per <= 0 {
		t.Fatalf("frameBytes = 0")
	}
	r.maxBytes = per*3 + per/2 // muat 3 frame, 4 tergusur
	for i := 1; i <= 8; i++ {
		r.Record(snap(int64(i)*200, 400), int64(i)*200)
	}
	info := r.SessionInfo()
	if info.Frames != 3 {
		t.Fatalf("frames = %d, want 3 (pagar byte)", info.Frames)
	}
	if info.FrameBytes > r.maxBytes {
		t.Fatalf("frameBytes %d > budget %d", info.FrameBytes, r.maxBytes)
	}
}

func TestDecisionsDedupeBySeq(t *testing.T) {
	r := New(Meta{})
	mk := func(seq uint64, order string) model.Decision {
		return model.Decision{Seq: seq, T: int64(seq) * 100, Strategy: "fifo", OrderID: order, RiderID: int(seq), Reason: "nearest idle"}
	}
	r.RecordDecisions([]model.Decision{mk(3, "o3"), mk(2, "o2"), mk(1, "o1")}) // terbaru di depan
	r.RecordDecisions([]model.Decision{mk(5, "o5"), mk(4, "o4"), mk(3, "o3")}) // overlap seq 3
	got := r.SessionInfo().Decisions
	if got != 5 {
		t.Fatalf("decisions = %d, want 5 (dup seq 3 tidak masuk dua kali)", got)
	}
	if r.decisions[0].OrderID != "o1" || r.decisions[4].OrderID != "o5" {
		t.Fatalf("urutan ascending salah: %v", r.decisions)
	}
	r.RecordDecisions([]model.Decision{mk(2, "o2")}) // seq lama → diabaikan
	if r.SessionInfo().Decisions != 5 {
		t.Fatalf("keputusan lama tidak boleh menambah")
	}
}

func TestDecisionsCap(t *testing.T) {
	r := New(Meta{})
	r.maxDecides = 10
	for i := 0; i < 25; i++ {
		r.RecordDecisions([]model.Decision{{Seq: uint64(i + 1), T: int64(i), OrderID: "x", RiderID: i, Reason: "r"}})
	}
	if got := r.SessionInfo().Decisions; got != 10 {
		t.Fatalf("decisions = %d, want 10", got)
	}
	if r.decisions[0].Seq != 16 {
		t.Fatalf("harus menyimpan seq terbaru, dapat %d", r.decisions[0].Seq)
	}
}

func TestWriteDumpGzipRoundTrip(t *testing.T) {
	r := New(Meta{Seed: 42, Strategy: "fifo", Riders: 100, TickHz: 10, StartedWallMs: 1_000_000})
	for i := 0; i < 5; i++ {
		r.Record(snap(int64(i)*200, 100), 1_000_000+int64(i)*200)
	}
	r.RecordDecisions([]model.Decision{{Seq: 1, T: 50, Strategy: "fifo", OrderID: "o1", RiderID: 3, DistM: 421, Reason: " FIFO: antrean tertua"}})

	var buf bytes.Buffer
	if err := r.WriteDump(&buf, true); err != nil {
		t.Fatalf("WriteDump: %v", err)
	}
	raw := gunzipAll(t, buf.Bytes())

	var dump struct {
		Meta      Meta             `json:"meta"`
		Wall0     int64            `json:"wall0"`
		T0        int64            `json:"t0"`
		Count     int              `json:"count"`
		Frames    []model.Snapshot `json:"frames"`
		Decisions []model.Decision `json:"decisions"`
	}
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("dump bukan JSON valid: %v\npayload: %.200s", err, raw)
	}
	if dump.Count != 5 || len(dump.Frames) != 5 {
		t.Fatalf("count=%d frames=%d, want 5", dump.Count, len(dump.Frames))
	}
	for i, f := range dump.Frames {
		if f.T != int64(i)*200 {
			t.Fatalf("frame[%d].T = %d", i, f.T)
		}
		if len(f.Riders) != 100 {
			t.Fatalf("frame[%d] riders = %d", i, len(f.Riders))
		}
	}
	if len(dump.Decisions) != 1 || dump.Decisions[0].Reason != " FIFO: antrean tertua" {
		t.Fatalf("decisions dump salah: %+v", dump.Decisions)
	}
	if dump.Meta.Seed != 42 || dump.Wall0 != 1_000_000 {
		t.Fatalf("meta/wall0 salah: %+v", dump.Meta)
	}
}

func TestWriteDumpIdentityFallback(t *testing.T) {
	r := New(Meta{Strategy: "optimal", Riders: 5})
	for i := 0; i < 3; i++ {
		r.Record(snap(int64(i)*200, 5), int64(i)*200)
	}
	var buf bytes.Buffer
	if err := r.WriteDump(&buf, false); err != nil {
		t.Fatalf("WriteDump identity: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte{0x1f, 0x8b}) {
		t.Fatalf("mode identitas tidak boleh mengandung blob gzip")
	}
	var dump struct {
		Count  int              `json:"count"`
		Frames []model.Snapshot `json:"frames"`
	}
	if err := json.Unmarshal(buf.Bytes(), &dump); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if dump.Count != 3 || len(dump.Frames) != 3 {
		t.Fatalf("count=%d frames=%d, want 3", dump.Count, len(dump.Frames))
	}
}

func TestWriteDumpEmpty(t *testing.T) {
	r := New(Meta{})
	var buf bytes.Buffer
	if err := r.WriteDump(&buf, true); err != nil {
		t.Fatalf("WriteDump kosong: %v", err)
	}
	var dump struct {
		Count  int              `json:"count"`
		Frames []model.Snapshot `json:"frames"`
	}
	if err := json.Unmarshal(gunzipAll(t, buf.Bytes()), &dump); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if dump.Count != 0 || len(dump.Frames) != 0 {
		t.Fatalf("harus kosong, dapat %d/%d", dump.Count, len(dump.Frames))
	}
}

func TestGzipRatioBudget(t *testing.T) {
	// Bukti anggaran RAM: 100 rider @ 5 Hz × 15 menit harus muat ≤ 24 MiB.
	r := New(Meta{Riders: 100})
	r.maxFrames = 1 << 30
	r.maxBytes = 1 << 30
	totalRaw := 0
	for i := 0; i < DefaultMaxFrames; i++ {
		s := snap(int64(i)*200, 100)
		raw, _ := json.Marshal(s)
		totalRaw += len(raw)
		r.Record(s, int64(i)*200)
	}
	info := r.SessionInfo()
	if info.FrameBytes > DefaultMaxBytes {
		t.Fatalf("ring %d B > budget %d B", info.FrameBytes, DefaultMaxBytes)
	}
	t.Logf("4500 frame: raw %d KB → gzip %d KB (rasio %.1f×)",
		totalRaw/1024, info.FrameBytes/1024, float64(totalRaw)/float64(info.FrameBytes))
}
