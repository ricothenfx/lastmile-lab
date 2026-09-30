package sim

import (
	"fmt"
	"math"
	"sort"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// FIFO menugaskan order tertua ke rider idle terdekat (haversine).
// Interface Strategy dipertahankan untuk Fase 3 (Batching, Zone, Optimal).
type FIFO struct{}

func (FIFO) Name() string { return "fifo" }

func (FIFO) Assign(orders []OrderView, riders []RiderView) []Assignment {
	// defensif: pastikan oldest-first meski pemanggil lupa mengurutkan
	sort.SliceStable(orders, func(i, j int) bool { return orders[i].CreatedMs < orders[j].CreatedMs })
	used := make(map[int]bool, len(riders))
	out := make([]Assignment, 0, len(orders))
	var nowMs int64
	if len(orders) > 0 {
		nowMs = orders[len(orders)-1].CreatedMs
	}
	for _, o := range orders { // sudah terurut oldest-first oleh engine
		best := -1
		bestD := math.MaxFloat64
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
		used[best] = true
		ageS := 0.0
		if nowMs > o.CreatedMs {
			ageS = float64(nowMs-o.CreatedMs) / 1000
		}
		out = append(out, Assignment{
			OrderID: o.ID,
			RiderID: best,
			DistM:   bestD,
			Reason: fmt.Sprintf("fifo: antrean tertua (umur %.0fs) → rider r%d idle terdekat (%.0f m, haversine)",
				ageS, best, bestD),
		})
	}
	return out
}

func haversine(latA, lonA, latB, lonB float64) float64 {
	const r = 6371000.0
	dLat := (latB - latA) * math.Pi / 180
	dLon := (lonB - lonA) * math.Pi / 180
	la := latA * math.Pi / 180
	lb := latB * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}
