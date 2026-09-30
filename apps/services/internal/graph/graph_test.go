package graph

import (
	"math"
	"strings"
	"testing"
)

func testGraph() (*Graph, error) {
	// grid 2x2: 0(0,0) - 1(0,1) - 2(1,0) - 3(1,1)
	meta := Meta{City: "Test", Source: "test", BBox: []float64{0, 0, 1, 1}}
	nodes := [][2]float64{{0.000, 0.000}, {0.000, 0.010}, {0.010, 0.000}, {0.010, 0.010}}
	edges := [][3]float64{
		{0, 1, 1000}, {0, 2, 1000}, {1, 3, 1000}, {2, 3, 1000},
	}
	return New(meta, nodes, edges, []int{0, 0, 0, 0}, []int{1, 2})
}

func TestDijkstraShortestPath(t *testing.T) {
	g, err := testGraph()
	if err != nil {
		t.Fatal(err)
	}
	path := g.Dijkstra(0, 3)
	if len(path) != 3 {
		t.Fatalf("want path len 3, got %v", path)
	}
	if path[0] != 0 || path[2] != 3 {
		t.Fatalf("path endpoints wrong: %v", path)
	}
}

func TestDijkstraUnreachable(t *testing.T) {
	meta := Meta{City: "Test"}
	nodes := [][2]float64{{0, 0}, {0, 1}, {2, 2}}
	edges := [][3]float64{{0, 1, 100}}
	g, err := New(meta, nodes, edges, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := g.Dijkstra(0, 2); p != nil {
		t.Fatalf("want nil for unreachable, got %v", p)
	}
}

func TestNeighborsDeterministic(t *testing.T) {
	g, err := testGraph()
	if err != nil {
		t.Fatal(err)
	}
	a := g.Neighbors(0)
	b := g.Neighbors(0)
	if len(a) != len(b) {
		t.Fatal("neighbor count differs")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("neighbor order not deterministic: %v vs %v", a, b)
		}
	}
}

func TestNearestNode(t *testing.T) {
	g, err := testGraph()
	if err != nil {
		t.Fatal(err)
	}
	if got := g.NearestNode(0.001, 0.009); got != 1 {
		t.Fatalf("want node 1, got %d", got)
	}
}

func TestEdgeLenAndLoad(t *testing.T) {
	g, err := testGraph()
	if err != nil {
		t.Fatal(err)
	}
	if l := g.EdgeLenM(0, 1); l != 1000 {
		t.Fatalf("want 1000, got %f", l)
	}
	if l := g.EdgeLenM(1, 0); l != 1000 {
		t.Fatal("edge should be bidirectional")
	}
	if math.IsNaN(g.EdgeLenM(0, 3)) {
		// diagonal tidak terhubung — NaN valid
	} else {
		t.Fatal("non-adjacent nodes should not report a length")
	}
}

func TestHaversineSanity(t *testing.T) {
	// Berlin Alexanderplatz (~52.5219, 13.4132) → Potsdamer Platz (~52.5095, 13.3763) ≈ 2.6 km
	d := HaversineM(52.5219, 13.4132, 52.5095, 13.3763)
	if d < 2400 || d > 2900 {
		t.Fatalf("haversine out of expected range: %f", d)
	}
}

func TestLoadRejectsBadJSON(t *testing.T) {
	if _, err := Load(strings.NewReader(`{"nodes":[]}`)); err == nil {
		t.Fatal("want error for empty nodes")
	}
}
