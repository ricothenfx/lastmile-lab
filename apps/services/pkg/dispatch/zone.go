package dispatch

import (
	"fmt"
	"math"
	"sort"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// Zone mempartisi peta menjadi grid zona: order di-assign ke rider idle DI DALAM
// zona pickup-nya; bila zona kosong, dicari per cincin tetangga (ring 1..2),
// lalu global sebagai fallback terakhir. Filosofi: armada dibiarkan tetap lokal —
// kandidat lintas zona yang sedikit lebih dekat tidak dibiarkan "menyedot" rider
// dari zona lain sehingga cakupan per-zona terjaga.
type Zone struct {
	CellDeg float64 // ukuran zona dalam derajat
	MaxRing int     // cincin tetangga maksimum sebelum fallback global
}

// NewZone membangun strategi zone; nilai <= 0 diganti default (cell 0.01°, ring 2).
func NewZone(cellDeg float64, maxRing int) Zone {
	if cellDeg <= 0 {
		cellDeg = 0.01
	}
	if maxRing <= 0 {
		maxRing = 2
	}
	return Zone{CellDeg: cellDeg, MaxRing: maxRing}
}

func (z Zone) Name() string { return "zone" }

func (z Zone) Assign(orders []OrderView, riders []RiderView) []Assignment {
	if len(orders) == 0 || len(riders) == 0 {
		return nil
	}
	// oldest-first tanpa memutasi input pemanggil.
	sorted := make([]OrderView, len(orders))
	copy(sorted, orders)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedMs < sorted[j].CreatedMs })

	byCell := make(map[cellKey][]RiderView, len(riders))
	for _, r := range riders {
		k := cellOf(r.Lat, r.Lon, z.CellDeg)
		byCell[k] = append(byCell[k], r)
	}

	used := make(map[int]bool, len(riders))
	out := make([]Assignment, 0, len(sorted))
	for _, o := range sorted {
		center := cellOf(o.PickupLat, o.PickupLon, z.CellDeg)
		best := -1
		bestD := math.MaxFloat64
		fallback := ""
		found := false
	forRing:
		for ring := 0; ring <= z.MaxRing; ring++ {
			for _, cand := range z.ringCells(center, ring) {
				for _, r := range byCell[cand] {
					if used[r.ID] || r.Status != model.RiderIdle {
						continue
					}
					d := haversine(r.Lat, r.Lon, o.PickupLat, o.PickupLon)
					if d < bestD {
						bestD, best, found = d, r.ID, true
					}
				}
				if best >= 0 {
					if ring == 0 {
						fallback = "ring 0 (zona sama)"
					} else {
						fallback = fmt.Sprintf("ring %d (zona pickup kosong)", ring)
					}
					break forRing
				}
			}
		}
		if !found { // global fallback — semua zona sekitar kosong
			for _, r := range riders {
				if used[r.ID] || r.Status != model.RiderIdle {
					continue
				}
				d := haversine(r.Lat, r.Lon, o.PickupLat, o.PickupLon)
				if d < bestD {
					bestD, best = d, r.ID
				}
			}
			if best < 0 {
				break // tidak ada rider idle tersisa
			}
			fallback = "global (zona + cincin kosong)"
		}
		used[best] = true
		out = append(out, Assignment{
			OrderID: o.ID,
			RiderID: best,
			DistM:   bestD,
			Reason: fmt.Sprintf(
				"zone: pickup in z=(%d,%d), %s → nearest idle rider r%d (%.0f m, haversine)",
				center.x, center.y, fallback, best, bestD),
		})
	}
	return out
}

// ringCells mengembalikan sel-sel pada chebyshev ring k di sekitar center,
// terurut deterministik (y naik, lalu x naik).
func (z Zone) ringCells(center cellKey, k int) []cellKey {
	if k == 0 {
		return []cellKey{center}
	}
	out := make([]cellKey, 0, 8*k)
	for dy := -k; dy <= k; dy++ {
		for dx := -k; dx <= k; dx++ {
			if maxAbs(dx, dy) != k {
				continue
			}
			out = append(out, cellKey{x: center.x + dx, y: center.y + dy})
		}
	}
	return out
}

func maxAbs(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}
