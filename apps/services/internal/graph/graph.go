// Package graph loads and serves the compact OSM-derived Berlin road graph
// (nodes/edges in berlin_graph.json) and provides routing.
package graph

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sort"
)

type Meta struct {
	City        string    `json:"city"`
	Source      string    `json:"source"`
	BBox        []float64 `json:"bbox"`
	GeneratedAt string    `json:"generatedAt"`
}

type fileFormat struct {
	Meta    Meta         `json:"meta"`
	Nodes   [][2]float64 `json:"nodes"`   // [lat, lon]
	Edges   [][3]float64 `json:"edges"`   // [a, b, meters]
	Classes []int        `json:"classes"` // 0 minor, 1 major
	POIs    []int        `json:"pois"`
}

type Graph struct {
	Meta    Meta
	nodes   [][2]float64
	classes []int
	pois    []int
	// adjList[n] = tetangga n terurut naik — iterasi deterministik sehingga
	// Dijkstra & fixture replay reproducible untuk seed yang sama.
	adjList [][]int
	adjMap  []map[int]float64
}

func Load(r io.Reader) (*Graph, error) {
	var f fileFormat
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode graph: %w", err)
	}
	return New(f.Meta, f.Nodes, f.Edges, f.Classes, f.POIs)
}

func New(meta Meta, nodes [][2]float64, edges [][3]float64, classes []int, pois []int) (*Graph, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("graph has no nodes")
	}
	g := &Graph{
		Meta:    meta,
		nodes:   nodes,
		classes: classes,
		pois:    pois,
		adjMap:  make([]map[int]float64, len(nodes)),
	}
	for i := range g.adjMap {
		g.adjMap[i] = map[int]float64{}
	}
	for _, e := range edges {
		a, b, m := int(e[0]), int(e[1]), e[2]
		if a < 0 || b < 0 || a >= len(nodes) || b >= len(nodes) {
			return nil, fmt.Errorf("edge [%v,%v] out of range", e[0], e[1])
		}
		if m <= 0 {
			m = HaversineM(nodes[a][0], nodes[a][1], nodes[b][0], nodes[b][1])
		}
		if cur, ok := g.adjMap[a][b]; !ok || m < cur {
			g.adjMap[a][b] = m
			g.adjMap[b][a] = m
		}
	}
	g.adjList = make([][]int, len(nodes))
	for i := range g.adjMap {
		for b := range g.adjMap[i] {
			g.adjList[i] = append(g.adjList[i], b)
		}
		sort.Ints(g.adjList[i])
	}
	return g, nil
}

func (g *Graph) NodeCount() int        { return len(g.nodes) }
func (g *Graph) POIs() []int           { return g.pois }
func (g *Graph) NodeLat(i int) float64 { return g.nodes[i][0] }
func (g *Graph) NodeLon(i int) float64 { return g.nodes[i][1] }

// Neighbors returns the nodes directly reachable from n, sorted ascending
// (deterministic iteration order).
func (g *Graph) Neighbors(n int) []int { return g.adjList[n] }

// EdgeLenM returns the length in meters of the link a↔b, or NaN when the
// nodes are not adjacent.
func (g *Graph) EdgeLenM(a, b int) float64 {
	if m, ok := g.adjMap[a][b]; ok {
		return m
	}
	return math.NaN()
}

// NearestNode returns the graph node closest to (lat, lon).
func (g *Graph) NearestNode(lat, lon float64) int {
	best, bestD := 0, math.MaxFloat64
	for i, n := range g.nodes {
		d := (n[0]-lat)*(n[0]-lat) + (n[1]-lon)*(n[1]-lon)
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// RandomPOI returns a random POI node index.
func (g *Graph) RandomPOI(rng *rand.Rand) int {
	return g.pois[rng.Intn(len(g.pois))]
}

// Dijkstra returns the shortest node path from→to including both endpoints,
// or nil when unreachable.
func (g *Graph) Dijkstra(from, to int) []int {
	if from == to {
		return []int{from}
	}
	n := len(g.nodes)
	dist := make([]float64, n)
	prev := make([]int, n)
	done := make([]bool, n)
	for i := range dist {
		dist[i] = math.Inf(1)
		prev[i] = -1
	}
	dist[from] = 0
	h := &minHeap{}
	h.push(heapItem{node: from, d: 0})
	for h.len() > 0 {
		it := h.pop()
		if done[it.node] {
			continue
		}
		done[it.node] = true
		if it.node == to {
			break
		}
		for _, b := range g.adjList[it.node] {
			w := g.adjMap[it.node][b]
			if done[b] {
				continue
			}
			nd := it.d + w
			if nd < dist[b] {
				dist[b] = nd
				prev[b] = it.node
				h.push(heapItem{node: b, d: nd})
			}
		}
	}
	if !done[to] {
		return nil
	}
	path := []int{to}
	for v := prev[to]; v != -1; v = prev[v] {
		path = append(path, v)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

type heapItem struct {
	node int
	d    float64
}

type minHeap []heapItem

func (h *minHeap) len() int         { return len(*h) }
func (h *minHeap) push(it heapItem) { *h = append(*h, it); h.up(len(*h) - 1) }
func (h *minHeap) pop() heapItem {
	out := (*h)[0]
	last := len(*h) - 1
	(*h)[0] = (*h)[last]
	*h = (*h)[:last]
	h.down(0)
	return out
}
func (h *minHeap) up(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if (*h)[p].d <= (*h)[i].d {
			break
		}
		(*h)[p], (*h)[i] = (*h)[i], (*h)[p]
		i = p
	}
}
func (h *minHeap) down(i int) {
	n := len(*h)
	for {
		l, r := 2*i+1, 2*i+2
		m := i
		if l < n && (*h)[l].d < (*h)[m].d {
			m = l
		}
		if r < n && (*h)[r].d < (*h)[m].d {
			m = r
		}
		if m == i {
			break
		}
		(*h)[m], (*h)[i] = (*h)[i], (*h)[m]
		i = m
	}
}

func HaversineM(latA, lonA, latB, lonB float64) float64 {
	const r = 6371000.0
	dLat := (latB - latA) * math.Pi / 180
	dLon := (lonB - lonA) * math.Pi / 180
	la := latA * math.Pi / 180
	lb := latB * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}
