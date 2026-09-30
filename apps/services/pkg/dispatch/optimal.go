package dispatch

import (
	"fmt"
	"math"
	"sort"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// Optimal menyelesaikan assignment bipartite min-cost global: matriks biaya =
// jarak haversine rider → pickup order, diselesaikan algoritma Hungarian /
// Jonker-Volgenant O(n²m) murni Go (tanpa CGO/OR-Tools — lihat ADR D17).
// Setiap pasangan yang terbentuk adalah bagian dari matching total-minimum;
// side yang lebih kecil (order atau rider) selalu terpasang penuh.
type Optimal struct{}

func (Optimal) Name() string { return "optimal" }

func (o Optimal) Assign(orders []OrderView, riders []RiderView) []Assignment {
	nOrders, nRiders := len(orders), len(riders)
	if nOrders == 0 || nRiders == 0 {
		return nil
	}
	// oldest-first tanpa memutasi input pemanggil (menstabilkan tie-break JV).
	sorted := make([]OrderView, nOrders)
	copy(sorted, orders)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedMs < sorted[j].CreatedMs })

	// Idle saja — baris/kolom rider non-idle tidak boleh dipakai.
	idle := make([]RiderView, 0, nRiders)
	for _, r := range riders {
		if r.Status == model.RiderIdle {
			idle = append(idle, r)
		}
	}
	if len(idle) == 0 {
		return nil
	}

	type pair struct {
		order int
		rider int
	}
	var pairs []pair
	if len(sorted) <= len(idle) {
		// baris = order (semua order dapat rider), kolom = rider
		cols := assignMinCost(makeMatrix(sorted, idle))
		pairs = make([]pair, len(cols))
		for row, col := range cols {
			pairs[row] = pair{order: row, rider: col}
		}
	} else {
		// baris = rider (semua rider dapat order), kolom = order
		cost := make([][]float64, len(idle))
		for i, r := range idle {
			cost[i] = make([]float64, len(sorted))
			for j, o := range sorted {
				cost[i][j] = haversine(r.Lat, r.Lon, o.PickupLat, o.PickupLon)
			}
		}
		cols := assignMinCost(cost)
		pairs = make([]pair, len(cols))
		for row, col := range cols {
			pairs[row] = pair{order: col, rider: row}
		}
	}

	out := make([]Assignment, 0, len(pairs))
	var totalCost float64
	for _, p := range pairs {
		o, r := sorted[p.order], idle[p.rider]
		d := haversine(r.Lat, r.Lon, o.PickupLat, o.PickupLon)
		totalCost += d
		out = append(out, Assignment{
			OrderID: o.ID,
			RiderID: r.ID,
			DistM:   d,
		})
	}
	// Total batch dicantumkan di setiap reason — explainability keputusan global.
	for i := range out {
		out[i].Reason = fmt.Sprintf(
			"optimal: matching bipartite min-cost (JV) batch %d pasangan, total pickup %.0f m → rider r%d (%.0f m, haversine)",
			len(out), totalCost, out[i].RiderID, out[i].DistM)
	}
	return out
}

func makeMatrix(rows []OrderView, cols []RiderView) [][]float64 {
	m := make([][]float64, len(rows))
	for i, o := range rows {
		m[i] = make([]float64, len(cols))
		for j, r := range cols {
			m[i][j] = haversine(r.Lat, r.Lon, o.PickupLat, o.PickupLon)
		}
	}
	return m
}

// assignMinCost: Hungarian/JV klasik (potensial + augmenting path terpendek),
// baris i ter-assign ke kolom result[i]; len(result) = jumlah baris, semua
// kolom unik. Deterministik: scan kolom selalu naik, tanpa randomness.
// Kompleksitas O(n²m) — n ≤ 500 masih << 50 ms pada CPU kelas VPS.
func assignMinCost(a [][]float64) []int {
	n := len(a)
	if n == 0 {
		return nil
	}
	m := len(a[0])
	const inf = math.MaxFloat64
	u := make([]float64, n+1)
	v := make([]float64, m+1)
	p := make([]int, m+1) // p[j] = baris (1-based) yang memakai kolom j
	way := make([]int, m+1)
	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, m+1)
		used := make([]bool, m+1)
		for j := range minv {
			minv[j] = inf
		}
		for {
			used[j0] = true
			i0, delta, j1 := p[j0], inf, 0
			for j := 1; j <= m; j++ {
				if !used[j] {
					cur := a[i0-1][j-1] - u[i0] - v[j]
					if cur < minv[j] {
						minv[j] = cur
						way[j] = j0
					}
					if minv[j] < delta {
						delta = minv[j]
						j1 = j
					}
				}
			}
			for j := 0; j <= m; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for j0 != 0 {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
		}
	}
	res := make([]int, n)
	for j := 1; j <= m; j++ {
		if p[j] > 0 {
			res[p[j]-1] = j - 1
		}
	}
	return res
}
