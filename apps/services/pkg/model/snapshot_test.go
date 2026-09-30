package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotCompactKeys(t *testing.T) {
	s := Snapshot{
		T:   1234,
		Seq: 7,
		Riders: []RiderPt{
			{ID: 1, S: int(RiderIdle), Lat: 52.5, Lon: 13.4},
		},
		Orders: []OrderPt{
			{ID: "o000001", S: int(OrderWaiting), Pla: 52.5, Plo: 13.4, Dla: 52.51, Dlo: 13.41},
		},
		Links: []Link{{R: 1, O: "o000001"}},
		Stats: Stats{Delivered: 2, Expired: 1, Active: 1, Idle: 59, Strategy: "fifo"},
	}
	buf, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	js := string(buf)
	for _, key := range []string{`"t":`, `"seq":`, `"r":`, `"o":`, `"l":`, `"st":`, `"i":`, `"la":`, `"lo":`, `"pa":`, `"da":`, `"dl":`, `"stg":"fifo"`} {
		if !strings.Contains(js, key) {
			t.Fatalf("compact key %s tidak ada di: %s", key, js)
		}
	}
	// wire payload harus kecil
	if len(js) > 400 {
		t.Fatalf("snapshot terlalu besar: %d bytes", len(js))
	}
}

func TestRiderStatusString(t *testing.T) {
	cases := map[RiderStatus]string{
		RiderIdle:       "idle",
		RiderToPickup:   "to_pickup",
		RiderPickup:     "pickup",
		RiderDelivering: "delivering",
		RiderStatus(99): "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Fatalf("status %d: want %s got %s", s, want, got)
		}
	}
}

func TestFullSnapshotCarriesDecisions(t *testing.T) {
	f := FullSnapshot{
		Snapshot: Snapshot{Seq: 1},
		Decisions: []Decision{{
			Seq: 1, T: 100, Strategy: "fifo", OrderID: "o1", RiderID: 3,
			DistM: 421, Reason: "fifo: antrean tertua",
		}},
	}
	buf, _ := json.Marshal(f)
	if !strings.Contains(string(buf), `"reason":"fifo: antrean tertua"`) {
		t.Fatalf("decisions hilang: %s", buf)
	}
}
