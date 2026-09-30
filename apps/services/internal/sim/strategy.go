package sim

import "github.com/ricothenfx/lastmile-lab/apps/services/pkg/dispatch"

// Alias ke path bersama pkg/dispatch (Fase 2): Strategy dipindah agar
// dispatch-consumer memakai algoritma yang sama. Seluruh kode lama
// (engine, test, fixturegen) tetap kompil tanpa refactor.

type (
	Strategy   = dispatch.Strategy
	OrderView  = dispatch.OrderView
	RiderView  = dispatch.RiderView
	Assignment = dispatch.Assignment
	FIFO       = dispatch.FIFO
)

// haversine lokal dipakai engine untuk jarak intra-tick.
func haversine(latA, lonA, latB, lonB float64) float64 {
	return dispatch.HaversineM(latA, lonA, latB, lonB)
}
