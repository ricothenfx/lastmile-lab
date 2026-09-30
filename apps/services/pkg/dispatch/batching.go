package dispatch

import (
	"fmt"
	"math"
	"sort"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// Batching menunda order muda selama WindowMs (window dispatch), lalu
// memproses batch yang matang dikelompokkan per kluster pickup (grid cell):
// order yang dipickup di resto yang sama/berdekatan dilayani berurutan, sehingga
// leg pickup armada efisien. Stateless — "window" dihitung dari umur order
// (CreatedMs vs timestamp order terbaru), sama hasilnya tanpa buffer internal.
//
// Trade-off dijelaskan di Reason: order selalu menunggu ≥ WindowMs; penukarnya
// adalah pickup leg yang lebih pendek saat demand mengelompok.
type Batching struct {
	WindowMs int64 // umur minimum order sebelum boleh di-assign
	MaxBatch int   // batas ukuran batch per tick (paling tua dulu)
	cellDeg  float64
}

// NewBatching membangun strategi batching; nilai <= 0 diganti default
// (window 2 s, batch 64, cell 0.01°).
func NewBatching(windowMs int64, maxBatch int) Batching {
	if windowMs <= 0 {
		windowMs = 2000
	}
	if maxBatch <= 0 {
		maxBatch = 64
	}
	return Batching{WindowMs: windowMs, MaxBatch: maxBatch, cellDeg: 0.01}
}

func (b Batching) Name() string { return "batching" }

func (b Batching) Assign(orders []OrderView, riders []RiderView) []Assignment {
	if len(orders) == 0 || len(riders) == 0 {
		return nil
	}
	// Jangan mutasi slice pemanggil (kontrak Strategy).
	sort.SliceStable(orders, func(i, j int) bool { return orders[i].CreatedMs < orders[j].CreatedMs })
	// Jam batch: NowMs pemanggil bila tersedia, fallback ke CreatedMs terbaru
	// (order terbaru dengan umur 0 akan matang di tick berikutnya).
	var nowMs int64
	for _, o := range orders {
		if o.NowMs > nowMs {
			nowMs = o.NowMs
		}
		if o.CreatedMs > nowMs {
			nowMs = o.CreatedMs
		}
	}

	ready := make([]OrderView, 0, len(orders))
	for _, o := range orders {
		if nowMs-o.CreatedMs >= b.WindowMs {
			ready = append(ready, o)
		}
	}
	if len(ready) > b.MaxBatch {
		ready = ready[:b.MaxBatch]
	}
	if len(ready) == 0 {
		return nil
	}

	// Kluster pickup per grid cell; urutan kluster deterministik (row,col),
	// order di dalam kluster oldest-first.
	type cluster struct {
		key    cellKey
		orders []OrderView
	}
	byCell := map[cellKey]*cluster{}
	for _, o := range ready {
		k := cellOf(o.PickupLat, o.PickupLon, b.cellDeg)
		c, ok := byCell[k]
		if !ok {
			c = &cluster{key: k}
			byCell[k] = c
		}
		c.orders = append(c.orders, o)
	}
	cells := make([]cellKey, 0, len(byCell))
	for k := range byCell {
		cells = append(cells, k)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].y != cells[j].y {
			return cells[i].y < cells[j].y
		}
		return cells[i].x < cells[j].x
	})

	used := make(map[int]bool, len(riders))
	readyCount := len(ready)
	out := make([]Assignment, 0, readyCount)
	for _, k := range cells {
		for _, o := range byCell[k].orders {
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
				return out // rider idle habis
			}
			used[best] = true
			ageS := float64(nowMs-o.CreatedMs) / 1000
			out = append(out, Assignment{
				OrderID: o.ID,
				RiderID: best,
				DistM:   bestD,
				Reason: fmt.Sprintf(
					"batching: window %.1fs terlampaui (umur %.1fs), kluster pickup z=(%d,%d) dari %d order siap → rider r%d idle terdekat (%.0f m, haversine)",
					float64(b.WindowMs)/1000, ageS, k.x, k.y, readyCount, best, bestD),
			})
		}
	}
	return out
}
