package sim

import (
	"testing"
	"time"

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

func TestSetControlsScalesDemandAndSpeed(t *testing.T) {
	g := tinyGraph(t)
	base, _ := New(g, cfgFast(), FIFO{})
	surged, _ := New(g, cfgFast(), FIFO{})
	s10 := 10.0
	surged.SetControls(&s10, nil)
	for i := 0; i < 300; i++ { // 30 detik virtual @10Hz
		base.Tick(100)
		surged.Tick(100)
	}
	if surged.CreatedTotal() <= base.CreatedTotal() {
		t.Fatalf("surge ×10 harus menghasilkan lebih banyak order: %d vs %d",
			surged.CreatedTotal(), base.CreatedTotal())
	}
	if s, w := surged.Controls(); s != 10 || w != 1 {
		t.Fatalf("controls salah: surge=%v weather=%v", s, w)
	}

	// weather < 1 melambatkan rider: posisi sesudah tick sama harus lebih dekat
	// ke titik awal dibanding baseline (speed dikali faktor).
	rain := 0.6
	slow, _ := New(g, cfgFast(), FIFO{})
	slow.SetControls(nil, &rain)
	base0, slow0 := base.Snapshot(), slow.Snapshot()
	for i := 0; i < 30; i++ { // 3 detik — tanpa dispatch (order masih jarang)
		base.Tick(100)
		slow.Tick(100)
	}
	base1, slow1 := base.Snapshot(), slow.Snapshot()
	dist := func(a, b model.RiderPt) float64 {
		dx := a.Lat - b.Lat
		dy := a.Lon - b.Lon
		return dx*dx + dy*dy
	}
	movedMore := 0
	for i := range base1.Riders {
		if dist(base0.Riders[i], base1.Riders[i]) > dist(slow0.Riders[i], slow1.Riders[i]) {
			movedMore++
		}
	}
	if movedMore == 0 {
		t.Fatal("hujan (weather 0.6) harus memperlambat rider")
	}
}

func TestSetControlsClamped(t *testing.T) {
	g := tinyGraph(t)
	e, _ := New(g, cfgFast(), FIFO{})
	big, rain := 99.0, 0.01
	e.SetControls(&big, &rain)
	s, w := e.Controls()
	if s != 10 || w != 0.2 {
		t.Fatalf("clamp gagal: surge=%v weather=%v", s, w)
	}
}

func TestInjectExternalAssignDedupeQueue(t *testing.T) {
	g := tinyGraph(t)
	c := cfgFast()
	c.OrderRatePerMin = 0 // matikan generator — hanya order pipeline
	e, _ := New(g, c, FIFO{})

	if got, _ := e.InjectExternal(model.ExternalOrder{ID: ""}); got != "rejected" {
		t.Fatalf("id kosong harus rejected, dapat %s", got)
	}

	o1 := model.ExternalOrder{
		ID: "lg-1", PickupLat: 0.0, PickupLon: 0.0, DropoffLat: 0.01, DropoffLon: 0.01,
		RiderID: -1, CreatedMs: 1000,
	}
	if got, _ := e.InjectExternal(o1); got != "queued" {
		t.Fatalf("order tanpa rider harus queued, dapat %s", got)
	}
	if got, _ := e.InjectExternal(o1); got != "duplicate" {
		t.Fatalf("replay id sama harus duplicate, dapat %s", got)
	}
	if got, _ := e.InjectExternal(o1); got != "duplicate" {
		t.Fatalf("replay ketiga juga duplicate, dapat %s", got)
	}
	if e.CreatedTotal() != 1 {
		t.Fatalf("createdTotal harus 1, dapat %d", e.CreatedTotal())
	}

	// injection dengan rider idle → assigned + link terlihat di snapshot
	snap := e.Snapshot()
	rider := snap.Riders[0]
	o2 := model.ExternalOrder{
		ID: "lg-2", PickupLat: rider.Lat, PickupLon: rider.Lon,
		DropoffLat: 0.01, DropoffLon: 0.01, RiderID: rider.ID, CreatedMs: 2000,
	}
	got, rid := e.InjectExternal(o2)
	if got != "assigned" || rid != rider.ID {
		t.Fatalf("harus assigned ke r%d, dapat %s r%d", rider.ID, got, rid)
	}
	if s := e.Snapshot(); len(s.Links) == 0 {
		t.Fatal("link assignment harus muncul di snapshot")
	}
	if e.CreatedTotal() != 2 {
		t.Fatalf("createdTotal harus 2, dapat %d", e.CreatedTotal())
	}

	// order ter-inject harus di-dispatch FIFO internal juga bila rider sibuk:
	// rider r0 sekarang to_pickup → inject lg-3 dengan rider r0 → queued →
	// dispatch internal (FIFO) menugaskan rider idle lain pada tick berikut.
	o3 := model.ExternalOrder{
		ID: "lg-3", PickupLat: rider.Lat, PickupLon: rider.Lon,
		DropoffLat: 0.01, DropoffLon: 0.01, RiderID: rider.ID, CreatedMs: 3000,
	}
	if got, _ := e.InjectExternal(o3); got != "queued" {
		t.Fatalf("rider sibuk → queued, dapat %s", got)
	}
	e.Tick(100)
	e.Tick(100)
	found := false
	for _, l := range e.Snapshot().Links {
		if l.O == "lg-3" {
			found = true
		}
	}
	if !found {
		t.Fatal("order queued harus di-dispatch ulang FIFO internal")
	}
}

func TestInjectExternalTTLExpiry(t *testing.T) {
	g := tinyGraph(t)
	c := cfgFast()
	c.OrderRatePerMin = 0
	c.OrderTTLSec = 1
	c.Riders = 1
	e, _ := New(g, c, FIFO{})
	// order pertama mendudukkan satu-satunya rider (to_pickup ~ menit),
	// order kedua tidak kebagian rider → antrean (waiting) → tunduk TTL.
	e.InjectExternal(model.ExternalOrder{
		ID: "lg-a", PickupLat: 0.0, PickupLon: 0.0,
		DropoffLat: 0.01, DropoffLon: 0.01, CreatedMs: 1000,
	})
	e.Tick(100)
	// created_ms wall-clock (jauh di masa depan jam virtual) harus di-clamp —
	// kalau tidak, TTL tidak pernah terpicu dan order menumpuk selamanya.
	e.InjectExternal(model.ExternalOrder{
		ID: "lg-ttl", PickupLat: 0.005, PickupLon: 0.005,
		DropoffLat: 0.01, DropoffLon: 0.01, CreatedMs: time.Now().UnixMilli(),
	})
	for i := 0; i < 300; i++ { // 30 detik virtual — TTL 1 detik terlampaui
		e.Tick(100)
	}
	if s := e.Snapshot(); s.Stats.Expired == 0 {
		t.Fatal("order pipeline yang mengantre juga harus tunduk TTL")
	}
}

func TestInjectExternalStaleOrderExpiresFast(t *testing.T) {
	g := tinyGraph(t)
	c := cfgFast()
	c.OrderRatePerMin = 0
	c.OrderTTLSec = 90
	c.Riders = 1
	e, _ := New(g, c, FIFO{})
	// rider langsung disibukkan → order berikutnya mengantre
	e.InjectExternal(model.ExternalOrder{
		ID: "lg-busy", CreatedMs: 0,
	})
	e.Tick(100)
	// pesan "basi" (created_ms lebih tua dari TTL): masuk sebagai sekarang,
	// bukan dianggap kedaluwarsa seketika — tapi tetap tunduk TTL normal.
	e.InjectExternal(model.ExternalOrder{
		ID: "lg-stale", CreatedMs: -999_999, PickupLat: 0.005, PickupLon: 0.005,
	})
	if s := e.Snapshot(); s.Stats.Active != 2 {
		t.Fatalf("order basi harus tetap masuk antrean (active=2), dapat %d", s.Stats.Active)
	}
}
