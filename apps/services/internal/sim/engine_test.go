package sim

import (
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/internal/graph"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

func tinyGraph(t *testing.T) *graph.Graph {
	t.Helper()
	// segiempat dengan diagonal: 0-1-3-2-0 + 1-2
	meta := graph.Meta{City: "Test", Source: "test", BBox: []float64{0, 0, 1, 1}}
	nodes := [][2]float64{
		{0.000, 0.000}, {0.000, 0.010}, {0.010, 0.000}, {0.010, 0.010},
	}
	edges := [][3]float64{{0, 1, 800}, {0, 2, 800}, {1, 3, 800}, {2, 3, 800}, {1, 2, 1100}}
	g, err := graph.New(meta, nodes, edges, nil, []int{0, 3})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func cfgFast() Config {
	c := DefaultConfig()
	c.Riders = 8
	c.OrderRatePerMin = 240 // agresif supaya order muncul dalam hitungan tick
	c.PickupDwellSec = 0.2
	c.DropoffDwellSec = 0.1
	c.OrderTTLSec = 30
	c.MinTripM = 300
	return c
}

func TestEngineRidersMove(t *testing.T) {
	g := tinyGraph(t)
	e, err := New(g, cfgFast(), FIFO{})
	if err != nil {
		t.Fatal(err)
	}
	before := e.Snapshot()
	for i := 0; i < 50; i++ {
		e.Tick(100)
	}
	after := e.Snapshot()
	moved := 0
	for i, r := range after.Riders {
		if r.Lat != before.Riders[i].Lat || r.Lon != before.Riders[i].Lon {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("no rider moved after 5s of sim")
	}
}

func TestFIFOAssignsOldestFirst(t *testing.T) {
	orders := []OrderView{
		{ID: "o2", CreatedMs: 2000, PickupLat: 0.005, PickupLon: 0.005},
		{ID: "o1", CreatedMs: 1000, PickupLat: 0.005, PickupLon: 0.005},
		{ID: "o3", CreatedMs: 3000, PickupLat: 0.005, PickupLon: 0.005},
	}
	riders := []RiderView{
		{ID: 7, Status: model.RiderIdle, Lat: 0.0, Lon: 0.0},
		{ID: 3, Status: model.RiderIdle, Lat: 0.0, Lon: 0.0},
	}
	out := FIFO{}.Assign(orders, riders)
	if len(out) != 2 {
		t.Fatalf("want 2 assignments (rider habis), got %d", len(out))
	}
	if out[0].OrderID != "o1" {
		t.Fatalf("oldest order harus lebih dulu, dapat %s", out[0].OrderID)
	}
	if out[1].OrderID != "o2" {
		t.Fatalf("assignment kedua harus o2, dapat %s", out[1].OrderID)
	}
	if out[0].RiderID == out[1].RiderID {
		t.Fatal("satu rider tidak boleh dapat dua order sekaligus")
	}
	if out[0].Reason == "" {
		t.Fatal("explainability stub wajib mengisi reason")
	}
}

func TestEngineEndToEndDelivery(t *testing.T) {
	g := tinyGraph(t)
	e, err := New(g, cfgFast(), FIFO{})
	if err != nil {
		t.Fatal(err)
	}
	// 3 menit simulasi @10Hz
	var sawAssigned bool
	for i := 0; i < 1800; i++ {
		e.Tick(100)
		s := e.Snapshot()
		if len(s.Links) > 0 {
			sawAssigned = true
		}
		if s.Stats.Delivered > 0 {
			break
		}
	}
	if !sawAssigned {
		t.Fatal("tidak ada assignment dalam 3 menit sim")
	}
	if s := e.Snapshot(); s.Stats.Delivered == 0 {
		t.Fatal("tidak ada order terkirim dalam 3 menit sim")
	}
}

func TestEngineTTLExpiry(t *testing.T) {
	g := tinyGraph(t)
	c := cfgFast()
	c.OrderRatePerMin = 600
	c.OrderTTLSec = 1
	// hanya 1 rider → antrean menumpuk → expiry
	c.Riders = 1
	e, err := New(g, c, FIFO{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ { // 40 detik
		e.Tick(100)
	}
	if s := e.Snapshot(); s.Stats.Expired == 0 {
		t.Fatal("order melebihi TTL harus expire")
	}
}

func TestSnapshotDeterminismSameSeed(t *testing.T) {
	run := func() model.Snapshot {
		g := tinyGraph(t)
		e, _ := New(g, cfgFast(), FIFO{})
		for i := 0; i < 300; i++ {
			e.Tick(100)
		}
		return e.Snapshot()
	}
	a, b := run(), run()
	if a.T != b.T || a.Seq != b.Seq {
		t.Fatal("waktu/seq berbeda antar run")
	}
	if len(a.Riders) != len(b.Riders) {
		t.Fatal("jumlah rider berbeda")
	}
	for i := range a.Riders {
		if a.Riders[i] != b.Riders[i] {
			t.Fatalf("rider %d beda posisi antar run: %+v vs %+v", i, a.Riders[i], b.Riders[i])
		}
	}
	if len(a.Orders) != len(b.Orders) || len(a.Links) != len(b.Links) {
		t.Fatal("order/link beda antar run")
	}
}

func TestStrategyInterfaceDecouplesEngine(t *testing.T) {
	// strategi "no-op" — bukti Strategy bisa ditukar tanpa refactor engine
	var s Strategy = noopStrategy{}
	g := tinyGraph(t)
	e, err := New(g, cfgFast(), s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		e.Tick(100)
	}
	if s := e.Snapshot(); len(s.Links) != 0 {
		t.Fatal("strategi no-op tidak boleh menghasilkan assignment")
	}
	if s.Name() != "noop" {
		t.Fatal("nama strategi salah")
	}
}

type noopStrategy struct{}

func (noopStrategy) Name() string { return "noop" }
func (noopStrategy) Assign(orders []OrderView, riders []RiderView) []Assignment {
	return nil
}
