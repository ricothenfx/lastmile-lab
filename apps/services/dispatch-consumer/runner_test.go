package main

import (
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

func TestSeenSetRingEviction(t *testing.T) {
	s := newSeenSet(4)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		s.Add(id)
	}
	if s.Has("a") {
		t.Fatal("a harus sudah ter-evict (kapasitas 4)")
	}
	for _, id := range []string{"b", "c", "d", "e"} {
		if !s.Has(id) {
			t.Fatalf("%s harus masih ada", id)
		}
	}
	s.Add("a")
	if !s.Has("a") {
		t.Fatal("a baru harus ada")
	}
	if s.Has("b") {
		t.Fatal("b harus ter-evict setelah a masuk lagi")
	}
}

func TestPlanBatchFIFOWithIdleRiders(t *testing.T) {
	msgs := []model.OrderMsg{
		{ID: "o2", CreatedMs: 2000, PickupLat: 0.005, PickupLon: 0.005},
		{ID: "o1", CreatedMs: 1000, PickupLat: 0.005, PickupLon: 0.005},
		{ID: "o3", CreatedMs: 3000, PickupLat: 0.005, PickupLon: 0.005},
	}
	riders := []dispatch.RiderView{
		{ID: 7, Status: model.RiderIdle, Lat: 0, Lon: 0},
		{ID: 3, Status: model.RiderIdle, Lat: 0, Lon: 0},
	}
	plan := planBatch(msgs, riders, dispatch.FIFO{})
	if len(plan.orders) != 3 {
		t.Fatalf("plan harus memuat 3 order, dapat %d", len(plan.orders))
	}
	byID := map[string]model.ExternalOrder{}
	for _, o := range plan.orders {
		byID[o.ID] = o
	}
	if byID["o1"].RiderID < 0 || byID["o2"].RiderID < 0 {
		t.Fatal("dua order tertua harus dapat rider")
	}
	if byID["o3"].RiderID != -1 {
		t.Fatal("order ketiga harus queued (rider habis)")
	}
	if byID["o1"].RiderID == byID["o2"].RiderID {
		t.Fatal("satu rider tidak boleh dapat dua order")
	}
}

func TestPlanBatchNoRidersAllQueued(t *testing.T) {
	msgs := []model.OrderMsg{
		{ID: "o1", CreatedMs: 1000, PickupLat: 52.5, PickupLon: 13.4},
	}
	plan := planBatch(msgs, nil, dispatch.FIFO{})
	if len(plan.orders) != 1 || plan.orders[0].RiderID != -1 {
		t.Fatal("tanpa rider idle semua order harus queued (rider -1)")
	}
}

func TestPlanBatchPreservesFields(t *testing.T) {
	msgs := []model.OrderMsg{{
		ID: "o9", CreatedMs: 1234,
		PickupLat: 52.5, PickupLon: 13.4, DropoffLat: 52.51, DropoffLon: 13.41,
	}}
	plan := planBatch(msgs, nil, dispatch.FIFO{})
	o := plan.orders[0]
	if o.ID != "o9" || o.CreatedMs != 1234 || o.PickupLat != 52.5 ||
		o.DropoffLon != 13.41 || o.DistM != 0 {
		t.Fatalf("field rusak saat plan: %+v", o)
	}
}

// guard kompatibilitas: Strategy interface dipakai lintas service.
var _ dispatch.Strategy = dispatch.FIFO{}
var _ SimClient = (*httpSim)(nil)
var _ DB = (*pgDB)(nil)
