package dispatch

// cellKey adalah kunci zona grid untuk strategi Zone & Batching.
// Grid dibagi per derajat (cellDeg ≈ 0.01° ≈ 1.1 km × 0.7 km di Berlin).
type cellKey struct{ x, y int }

func cellOf(lat, lon, deg float64) cellKey {
	return cellKey{x: int(floor(lon / deg)), y: int(floor(lat / deg))}
}

func floor(v float64) float64 {
	if v >= 0 {
		return float64(int(v))
	}
	f := float64(int(v))
	if f > v {
		f--
	}
	return f
}
