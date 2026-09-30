// Package model holds the wire types shared by rider-sim, ws-gateway and
// api-gateway. JSON keys are intentionally short: snapshots stream at 10–15 Hz
// to every connected client.
package model

// RiderStatus — satu makna warna di seluruh sistem (DESIGN.md §2).
type RiderStatus int

const (
	RiderIdle       RiderStatus = iota // lime    — idle
	RiderToPickup                      // amber   — menuju resto
	RiderPickup                        // cyan    — pickup di resto
	RiderDelivering                    // violet  — mengantar
)

func (s RiderStatus) String() string {
	switch s {
	case RiderIdle:
		return "idle"
	case RiderToPickup:
		return "to_pickup"
	case RiderPickup:
		return "pickup"
	case RiderDelivering:
		return "delivering"
	}
	return "unknown"
}

// OrderStatus untuk order aktif (delivered/expired keluar dari snapshot).
type OrderStatus int

const (
	OrderWaiting   OrderStatus = iota // belum di-assign
	OrderAssigned                     // rider menuju pickup
	OrderInTransit                    // rider mengantar ke dropoff
)

// Snapshot adalah state penuh sistem pada satu tick simulasi.
type Snapshot struct {
	T      int64     `json:"t"` // sim time ms sejak epoch wall clock awal
	Seq    uint64    `json:"seq"`
	Riders []RiderPt `json:"r"`
	Orders []OrderPt `json:"o"`
	Links  []Link    `json:"l"` // assignment aktif rider↔order
	Stats  Stats     `json:"st"`
}

type RiderPt struct {
	ID  int     `json:"i"`
	S   int     `json:"s"`
	Lat float64 `json:"la"`
	Lon float64 `json:"lo"`
}

type OrderPt struct {
	ID  string  `json:"i"`
	S   int     `json:"s"`
	Pla float64 `json:"pa"` // pickup lat
	Plo float64 `json:"po"` // pickup lon
	Dla float64 `json:"da"` // dropoff lat
	Dlo float64 `json:"do"` // dropoff lon
}

type Link struct {
	R int    `json:"r"`
	O string `json:"o"`
}

type Stats struct {
	Delivered int    `json:"dl"`
	Expired   int    `json:"ex"`
	Active    int    `json:"ac"`
	Idle      int    `json:"id"`
	Strategy  string `json:"stg,omitempty"`
	UptimeSec int64  `json:"up,omitempty"`
}

// Decision adalah explainability stub: alasan keputusan dispatch.
type Decision struct {
	Seq      uint64  `json:"seq"`
	T        int64   `json:"t"`
	Strategy string  `json:"strategy"`
	OrderID  string  `json:"order"`
	RiderID  int     `json:"rider"`
	DistM    float64 `json:"dist_m"`
	Reason   string  `json:"reason"`
}

// FullSnapshot = snapshot wire + data debug/explainability (REST saja).
type FullSnapshot struct {
	Snapshot
	Decisions []Decision `json:"decisions"`
}

// Health adalah bentuk /healthz semua service.
type Health struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service"`
	UptimeSec int64  `json:"uptime_sec"`
	Detail    string `json:"detail,omitempty"`
}
