package dispatch

import "fmt"

// ByName memetakan nama strategi (env/flag/API) ke instance berdefault.
// Nama: "fifo", "batching", "zone", "optimal" — dipakai rider-sim,
// dispatch-consumer dan strategy-lab agar tidak ada hardcode duplikat.
func ByName(name string) (Strategy, error) {
	switch name {
	case "fifo":
		return FIFO{}, nil
	case "batching":
		return NewBatching(0, 0), nil
	case "zone":
		return NewZone(0, 0), nil
	case "optimal":
		return Optimal{}, nil
	}
	return nil, fmt.Errorf("strategi tidak dikenal %q (pilihan: batching fifo optimal zone)", name)
}

// Names mengembalikan daftar nama strategi terurut (untuk help/validation).
func Names() []string {
	return []string{"batching", "fifo", "optimal", "zone"}
}
