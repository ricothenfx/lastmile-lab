// KPI live (Fase 4) — ring metrik performa terkini untuk KPI Command Deck.
//
// Semua angka diukur dari engine yang sama yang menggerakkan demo: durasi
// delivery created→delivered, latensi keputusan dispatch (wall-clock di
// sekitar Strategy.Assign), utilisasi armada, dan cost/order. Tidak ada angka
// sintetis; ring berkapasitas tetap sehingga metrik = performa TERKINI
// (bukan rata-rata sepanjang umur proses).
package sim

import (
	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// KPIRingCap adalah kapasitas ring delivery/dispatch (≤512 sampel terakhir).
const KPIRingCap = 512

// recordDispatch mencatat durasi satu panggilan Strategy.Assign (ns).
// Dipanggil hanya untuk panggilan nyata (ada order & rider kandidat).
func (e *Engine) recordDispatch(d int64) {
	e.dispatchRing[e.dispatchRingIdx] = d
	e.dispatchRingIdx = (e.dispatchRingIdx + 1) % KPIRingCap
	e.dispatchRingN++
	e.dispatchTotalNs += d
	e.dispatchCalls++
}

// KPI adalah ringkasan metrik live engine (read-only copy).
type KPI struct {
	NowMs         int64   `json:"now_ms"`
	Seq           uint64  `json:"seq"`
	Riders        int     `json:"riders"`
	IdleRiders    int     `json:"idle_riders"`
	OrdersActive  int     `json:"orders_active"`
	OrdersWaiting int     `json:"orders_waiting"`
	Created       int     `json:"orders_created"`
	Delivered     int     `json:"orders_delivered"`
	Expired       int     `json:"orders_expired"`
	Surge         float64 `json:"surge"`
	Weather       float64 `json:"weather"`
	Strategy      string  `json:"strategy"`

	// Lifetime counters (dipakai cost/order & utilisation).
	TaskDistM float64 `json:"task_dist_m"`
	BusyMs    int64   `json:"busy_ms"`
	RiderMs   int64   `json:"rider_ms"`

	// Ring terbaru — salinan; pemanggil bebas mengurutkan/memutasi.
	DeliveryRecentMs []int64 `json:"delivery_recent_ms"`
	DispatchRecentNs []int64 `json:"dispatch_recent_ns"`
	DispatchCalls    uint64  `json:"dispatch_calls"`
	DispatchTotalNs  int64   `json:"dispatch_total_ns"`
}

// KPI mengembalikan snapshot metrik live (copy aman, murah — ring fixed).
func (e *Engine) KPI() KPI {
	e.mu.RLock()
	defer e.mu.RUnlock()
	k := KPI{
		NowMs:           e.nowMs,
		Seq:             e.seq,
		Riders:          len(e.riders),
		OrdersActive:    len(e.orders),
		Created:         e.createdTotal,
		Delivered:       e.delivered,
		Expired:         e.expired,
		Surge:           e.surgeFactor,
		Weather:         e.weatherFactor,
		Strategy:        e.strategy.Name(),
		TaskDistM:       e.taskDistM,
		BusyMs:          e.busyMs,
		RiderMs:         int64(len(e.riders)) * e.nowMs,
		DispatchCalls:   e.dispatchCalls,
		DispatchTotalNs: e.dispatchTotalNs,
	}
	k.DeliveryRecentMs = e.copyRing(e.deliveryRing[:], e.deliveryRingIdx, e.deliveryRingN)
	k.DispatchRecentNs = e.copyRing(e.dispatchRing[:], e.dispatchRingIdx, e.dispatchRingN)
	for _, r := range e.riders {
		if r.status == model.RiderIdle {
			k.IdleRiders++
		}
	}
	for _, o := range e.orders {
		if o.status == model.OrderWaiting {
			k.OrdersWaiting++
		}
	}
	return k
}

// copyRing mengembalikan isi ring dalam urutan oldest→newest.
// Harus dipanggil dengan lock engine terpasang.
func (e *Engine) copyRing(ring []int64, idx int, n uint64) []int64 {
	count := int(n)
	if count > len(ring) {
		count = len(ring)
	}
	out := make([]int64, 0, count)
	if count == 0 {
		return out
	}
	start := 0
	if n >= uint64(len(ring)) {
		start = idx // ring penuh: elemen tertua tepat di posisi idx
	}
	for i := 0; i < count; i++ {
		out = append(out, ring[(start+i)%len(ring)])
	}
	return out
}
