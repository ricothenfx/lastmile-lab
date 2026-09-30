// Package dispatch holds the Strategy contract shared by rider-sim and
// dispatch-consumer (Fase 2): strategi assignment dipindah ke path bersama
// agar pipeline Kafka memakai algoritma yang sama dengan engine internal,
// tanpa refactor frontend (kontrak model.Snapshot tetap).
package dispatch

import (
	"fmt"
	"math"
	"sort"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// OrderView / RiderView melindungi Strategy dari state internal engine.
// NowMs (Fase 3) opsional: jam virtual pemanggil saat Assign dipanggil —
// dipakai strategi berbasis window (Batching); 0 = fallback ke CreatedMs
// terbaru. Field aditif, kontrak lama tetap compile tanpa perubahan.
type OrderView struct {
	ID        string
	CreatedMs int64
	PickupLat float64
	PickupLon float64
	NowMs     int64
}

type RiderView struct {
	ID     int
	Status model.RiderStatus
	Lat    float64
	Lon    float64
}

// Assignment adalah satu keputusan dispatch + alasannya (explainability).
type Assignment struct {
	OrderID string
	RiderID int
	DistM   float64
	Reason  string
}

// Strategy dipanggil dengan antrean order tertua-dulu.
// Implementasi TIDAK BOLEH memutasi state pemanggil.
type Strategy interface {
	Name() string
	Assign(orders []OrderView, riders []RiderView) []Assignment
}

// FIFO menugaskan order tertua ke rider idle terdekat (haversine).
// Fase 3 menambah Batching/Zone/Optimal di belakang interface yang sama.
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
	for _, o := range orders { // sudah terurut oldest-first
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

// HaversineM diekspos untuk consumer (jarak assignment dilaporkan di event DB).
func HaversineM(latA, lonA, latB, lonB float64) float64 { return haversine(latA, lonA, latB, lonB) }
