package dispatch

import (
	"math"
	"reflect"
	"testing"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// gridRider membangun RiderView idle di koordinat (lat, lon).
func gridRider(id int, lat, lon float64) RiderView {
	return RiderView{ID: id, Status: model.RiderIdle, Lat: lat, Lon: lon}
}

func gridOrder(id string, ageMs int64, lat, lon float64) OrderView {
	return OrderView{ID: id, CreatedMs: 10_000 + ageMs, PickupLat: lat, PickupLon: lon}
}

func assertUniqueAndValid(t *testing.T, got []Assignment, orders []OrderView, riders []RiderView) {
	t.Helper()
	seenOrder := map[string]bool{}
	seenRider := map[int]bool{}
	byID := map[string]OrderView{}
	for _, o := range orders {
		byID[o.ID] = o
	}
	byRider := map[int]RiderView{}
	for _, r := range riders {
		byRider[r.ID] = r
	}
	for _, a := range got {
		if seenOrder[a.OrderID] {
			t.Fatalf("order %s di-assign dua kali", a.OrderID)
		}
		if seenRider[a.RiderID] {
			t.Fatalf("rider %d dapat dua order", a.RiderID)
		}
		if _, ok := byID[a.OrderID]; !ok {
			t.Fatalf("order %s tidak ada di input", a.OrderID)
		}
		if _, ok := byRider[a.RiderID]; !ok {
			t.Fatalf("rider %d tidak ada di input", a.RiderID)
		}
		if byRider[a.RiderID].Status != model.RiderIdle {
			t.Fatalf("rider %d tidak idle", a.RiderID)
		}
		if a.Reason == "" {
			t.Fatalf("reason kosong untuk %s (explainability wajib)", a.OrderID)
		}
		seenOrder[a.OrderID] = true
		seenRider[a.RiderID] = true
	}
}

func allStrategies(t *testing.T) map[string]Strategy {
	t.Helper()
	out := map[string]Strategy{}
	for _, n := range Names() {
		s, err := ByName(n)
		if err != nil {
			t.Fatalf("ByName(%s): %v", n, err)
		}
		out[n] = s
	}
	return out
}

func TestAllStrategiesDeterministicAndUnique(t *testing.T) {
	orders := make([]OrderView, 0, 40)
	for i := 0; i < 40; i++ {
		orders = append(orders, gridOrder(
			"o"+string(rune('a'+i)), int64(i*500),
			52.50+float64(i%5)*0.005, 13.40+float64(i%7)*0.004))
	}
	riders := make([]RiderView, 0, 20)
	for i := 0; i < 20; i++ {
		riders = append(riders, gridRider(i, 52.50+float64(i%4)*0.006, 13.40+float64(i%6)*0.005))
	}
	for name, s := range allStrategies(t) {
		a1 := s.Assign(orders, riders)
		a2 := s.Assign(orders, riders)
		if !reflect.DeepEqual(a1, a2) {
			t.Fatalf("%s: hasil Assign tidak deterministik", name)
		}
		assertUniqueAndValid(t, a1, orders, riders)
		if len(a1) != 20 {
			t.Fatalf("%s: 40 order × 20 rider → harus 20 pasangan, dapat %d", name, len(a1))
		}
		// input tidak boleh termutasi (kontrak Strategy)
		if orders[0].ID != "oa" {
			t.Fatalf("%s: input orders termutasi (slice re-ordered)", name)
		}
	}
}

func TestBatchingWindowHoldsYoungOrders(t *testing.T) {
	b := NewBatching(2000, 64)
	// jam pemanggil 12_000: o-old (9_000, umur 3s) matang; o-young (11_900,
	// umur 100ms) ditahan sampai window terlampaui.
	old := OrderView{ID: "o-old", CreatedMs: 9_000, PickupLat: 52.50, PickupLon: 13.40}
	young := OrderView{ID: "o-young", CreatedMs: 11_900, PickupLat: 52.50, PickupLon: 13.40, NowMs: 12_000}
	old.NowMs = 12_000
	riders := []RiderView{gridRider(1, 52.50, 13.40), gridRider(2, 52.50, 13.40)}
	got := b.Assign([]OrderView{young, old}, riders)
	if len(got) != 1 || got[0].OrderID != "o-old" {
		t.Fatalf("batching harus menahan order muda, dapat %+v", got)
	}
	// tanpa order matang → tidak ada assignment
	got = b.Assign([]OrderView{young}, riders)
	if len(got) != 0 {
		t.Fatalf("order di bawah window tidak boleh keluar, dapat %+v", got)
	}
	// tanpa NowMs pun (pemanggil lama) strategi tetap jalan via fallback
	oldF := OrderView{ID: "o-fallback", CreatedMs: 100}
	youngF := OrderView{ID: "o-fallback-young", CreatedMs: 2_500}
	got = b.Assign([]OrderView{oldF, youngF}, riders)
	if len(got) != 1 || got[0].OrderID != "o-fallback" {
		t.Fatalf("fallback CreatedMs terbaru gagal, dapat %+v", got)
	}
}

func TestBatchingClustersSamePickup(t *testing.T) {
	b := NewBatching(1000, 64)
	// 3 order matang di pickup sama + 1 di pickup jauh; rider terdekat per
	// kluster harus dipilih (batch diproses per kluster pickup).
	const now int64 = 20_000
	mk := func(id string, created int64, lat, lon float64) OrderView {
		return OrderView{ID: id, CreatedMs: created, PickupLat: lat, PickupLon: lon, NowMs: now}
	}
	orders := []OrderView{
		mk("oA", 16_000, 52.500, 13.400),
		mk("oB", 15_000, 52.500, 13.400),
		mk("oC", 14_000, 52.500, 13.400),
		mk("oD", 13_000, 52.540, 13.460),
	}
	riders := []RiderView{
		gridRider(1, 52.5001, 13.4001),
		gridRider(2, 52.5002, 13.4002),
		gridRider(3, 52.5401, 13.4601),
		gridRider(4, 52.5402, 13.4602),
	}
	got := b.Assign(orders, riders)
	assertUniqueAndValid(t, got, orders, riders)
	if len(got) != 4 {
		t.Fatalf("4 order × 4 rider → 4 pasangan, dapat %d", len(got))
	}
	for _, a := range got {
		// dalam kluster, order TERBARU dilayani rider terdekat (oldest-first
		// diproses duluan) → oC dapat r1, lalu oB → r2, oA → r3; oD kluster
		// lain → r4.
		wantRider := map[string]int{"oA": 3, "oB": 2, "oC": 1, "oD": 4}[a.OrderID]
		if a.RiderID != wantRider {
			t.Fatalf("%s harus ke r%d (kluster pickup terdekat), dapat r%d", a.OrderID, wantRider, a.RiderID)
		}
	}
}

func TestZonePrefersSameZoneAndFallsBack(t *testing.T) {
	z := NewZone(0.01, 2)
	const deg = 0.01
	pickup := [2]float64{52.500, 13.400}
	zk := cellOf(pickup[0], pickup[1], deg)
	// rider di pusat zona pickup
	sameZone := gridRider(2, (float64(zk.y)+0.5)*deg, (float64(zk.x)+0.5)*deg)
	// rider lintas batas zona x+1, tepat di seberang pickup (lebih dekat,
	// tapi zona lain — zone strategy tetap memilih yang satu zona).
	crossLon := (float64(zk.x)+1)*deg + 0.0001
	crossCloser := gridRider(1, pickup[0], crossLon)
	if cellOf(sameZone.Lat, sameZone.Lon, deg) != zk {
		t.Fatal("setup salah: rider 2 harus satu zona dengan pickup")
	}
	if cellOf(crossCloser.Lat, crossCloser.Lon, deg) == zk {
		t.Fatal("setup salah: rider 1 harus zona berbeda")
	}
	orders := []OrderView{gridOrder("o1", 5000, pickup[0], pickup[1])}
	got := z.Assign(orders, []RiderView{crossCloser, sameZone})
	if len(got) != 1 || got[0].RiderID != 2 {
		t.Fatalf("zone harus memilih rider zona sama, dapat %+v", got)
	}
	// semua zona kosong → global fallback tetap meng-assign
	far := gridRider(3, 52.60, 13.55)
	got = z.Assign(orders, []RiderView{far})
	if len(got) != 1 || got[0].RiderID != 3 {
		t.Fatalf("zone harus fallback global, dapat %+v", got)
	}
}

func TestOptimalBeatsGreedyOnClassicCase(t *testing.T) {
	o := Optimal{}
	// Kasus klasik di mana greedy per-order tidak optimal:
	// pickup O1 dekat kedua rider, O2 hanya dekat r2.
	orders := []OrderView{
		gridOrder("o1", 2000, 52.5000, 13.4000),
		gridOrder("o2", 1000, 52.5000, 13.4080),
	}
	riders := []RiderView{
		gridRider(1, 52.5000, 13.4010), // dekat o1, cukup dekat o2
		gridRider(2, 52.5000, 13.4070), // sangat dekat o2
	}
	got := o.Assign(orders, riders)
	if len(got) != 2 {
		t.Fatalf("harus 2 pasangan, dapat %d", len(got))
	}
	byOrder := map[string]int{}
	var total float64
	for _, a := range got {
		byOrder[a.OrderID] = a.RiderID
		total += a.DistM
	}
	// matching optimal: o1→r1 (111 m), o2→r2 (111 m); greedy o1 bisa mengambil r2.
	if byOrder["o1"] != 1 || byOrder["o2"] != 2 {
		t.Fatalf("matching min-cost salah: %+v", got)
	}
	if total > 250 {
		t.Fatalf("total cost matching terlalu besar: %.0f m", total)
	}
}

func TestOptimalRectangularSides(t *testing.T) {
	o := Optimal{}
	// order >> rider
	orders := make([]OrderView, 10)
	riders := []RiderView{gridRider(1, 52.50, 13.40), gridRider(2, 52.53, 13.45)}
	for i := range orders {
		orders[i] = gridOrder("o"+string(rune('a'+i)), int64(i*100), 52.50+0.001*float64(i), 13.40)
	}
	got := o.Assign(orders, riders)
	if len(got) != 2 {
		t.Fatalf("order 10 × rider 2 → 2 pasangan, dapat %d", len(got))
	}
	assertUniqueAndValid(t, got, orders, riders)
	// rider >> order (semua order harus terpasang ke rider terdekatnya)
	riders = make([]RiderView, 8)
	for i := range riders {
		riders[i] = gridRider(i+1, 52.50+0.004*float64(i), 13.40)
	}
	two := orders[:2]
	got = o.Assign(two, riders)
	if len(got) != 2 {
		t.Fatalf("order 2 × rider 8 → 2 pasangan, dapat %d", len(got))
	}
	assertUniqueAndValid(t, got, two, riders)
}

func TestOptimalMatchesBruteForceOnRandomMatrix(t *testing.T) {
	// verifikasi kebenaran JV terhadap brute force permutasi (n kecil)
	coords := []float64{0.0, 0.003, 0.007, 0.011}
	orders := make([]OrderView, 4)
	for i := range orders {
		orders[i] = gridOrder("o"+string(rune('a'+i)), int64(i*100), 52.50+coords[i], 13.40)
	}
	riders := make([]RiderView, 4)
	for i := range riders {
		riders[i] = gridRider(i+1, 52.50+coords[3-i], 13.407)
	}
	got := Optimal{}.Assign(orders, riders)
	if len(got) != 4 {
		t.Fatalf("harus 4 pasangan, dapat %d", len(got))
	}
	var total float64
	for _, a := range got {
		total += a.DistM
	}
	best := bruteForceMin(orders, riders)
	if math.Abs(total-best) > 1e-6 {
		t.Fatalf("JV total %.4f ≠ brute force %.4f", total, best)
	}
}

func bruteForceMin(orders []OrderView, riders []RiderView) float64 {
	perm := func(n int) [][]int {
		var out [][]int
		var rec func([]int, int)
		rec = func(cur []int, k int) {
			if k == len(cur) {
				cp := make([]int, len(cur))
				copy(cp, cur)
				out = append(out, cp)
				return
			}
			for i := k; i < len(cur); i++ {
				cur[k], cur[i] = cur[i], cur[k]
				rec(cur, k+1)
				cur[k], cur[i] = cur[i], cur[k]
			}
		}
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		rec(idx, 0)
		return out
	}
	best := math.MaxFloat64
	for _, p := range perm(len(riders)) {
		var total float64
		for i, ri := range p {
			total += haversine(riders[ri].Lat, riders[ri].Lon, orders[i].PickupLat, orders[i].PickupLon)
		}
		if total < best {
			best = total
		}
	}
	return best
}

func TestRegistryUnknown(t *testing.T) {
	if _, err := ByName("gpt"); err == nil {
		t.Fatal("strategi tak dikenal harus error")
	}
	s, err := ByName("fifo")
	if err != nil || s.Name() != "fifo" {
		t.Fatalf("fifo harus valid, dapat %v %v", s, err)
	}
}

func TestStrategiesHandleEmptyAndBusyRiders(t *testing.T) {
	for name, s := range allStrategies(t) {
		if got := s.Assign(nil, []RiderView{gridRider(1, 52.5, 13.4)}); len(got) != 0 {
			t.Fatalf("%s: order kosong harus nil", name)
		}
		if got := s.Assign([]OrderView{gridOrder("o", 1000, 52.5, 13.4)}, nil); len(got) != 0 {
			t.Fatalf("%s: rider kosong harus nil", name)
		}
		busy := []RiderView{{ID: 1, Status: model.RiderDelivering, Lat: 52.5, Lon: 13.4}}
		if got := s.Assign([]OrderView{gridOrder("o", 1000, 52.5, 13.4)}, busy); len(got) != 0 {
			t.Fatalf("%s: rider sibuk tidak boleh di-assign", name)
		}
	}
}
