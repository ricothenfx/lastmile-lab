// Command graphgen builds the Berlin road graph + map layers from OpenStreetMap.
//
// One-off data pipeline (output is committed to the repo):
//   - compact routing graph  -> rider-sim/data/berlin_graph.json
//   - roads layer (GeoJSON)  -> ../../apps/web/public/berlin/roads.geojson
//   - water layer (GeoJSON)  -> ../../apps/web/public/berlin/water.geojson
//
// Source: Overpass API. Run from apps/services:
//
//	go run ./tools/graphgen
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var bbox = [4]float64{52.4900, 13.3800, 52.5320, 13.4500} // minLat, minLon, maxLat, maxLon

type overpassElem struct {
	Type     string            `json:"type"`
	ID       int64             `json:"id"`
	Lat      float64           `json:"lat,omitempty"`
	Lon      float64           `json:"lon,omitempty"`
	Geometry []overpassCoord   `json:"geometry,omitempty"`
	Members  []overpassMember  `json:"members,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
}

type overpassCoord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type overpassMember struct {
	Type     string          `json:"type"`
	Ref      int64           `json:"ref"`
	Role     string          `json:"role"`
	Geometry []overpassCoord `json:"geometry,omitempty"`
}

type overpassResp struct {
	Elements []overpassElem `json:"elements"`
}

// ---- routing graph (compact, hand-shaped for size) ----

type graphFile struct {
	Meta    meta         `json:"meta"`
	Nodes   [][2]float64 `json:"nodes"`   // [lat, lon]
	Edges   [][3]float64 `json:"edges"`   // [a, b, meters] undirected
	Classes []int        `json:"classes"` // per edge: 0 minor, 1 major
	POIs    []int        `json:"pois"`    // node indices usable as pickup/dropoff
}

type meta struct {
	City        string    `json:"city"`
	Source      string    `json:"source"`
	BBox        []float64 `json:"bbox"`
	GeneratedAt string    `json:"generatedAt"`
}

const (
	classMinor = 0
	classMajor = 1
)

var drivable = map[string]int{
	"motorway": classMajor, "motorway_link": classMajor,
	"trunk": classMajor, "trunk_link": classMajor,
	"primary": classMajor, "primary_link": classMajor,
	"secondary": classMajor, "secondary_link": classMajor,
	"tertiary": classMajor, "tertiary_link": classMajor,
	"unclassified":  classMinor,
	"residential":   classMinor,
	"living_street": classMinor,
	"service":       classMinor,
}

func main() {
	graphOut := flag.String("graph", "rider-sim/data/berlin_graph.json", "output routing graph")
	roadsOut := flag.String("roads", "../web/public/berlin/roads.geojson", "output roads GeoJSON")
	waterOut := flag.String("water", "../web/public/berlin/water.geojson", "output water GeoJSON")
	bboxFlag := flag.String("bbox", "", "minLat,minLon,maxLat,maxLat override")
	flag.Parse()
	if *bboxFlag != "" {
		var b [4]float64
		if _, err := fmt.Sscanf(*bboxFlag, "%f,%f,%f,%f", &b[0], &b[1], &b[2], &b[3]); err != nil {
			log.Fatalf("bad bbox: %v", err)
		}
		bbox = b
	}

	bb := fmt.Sprintf("(%f,%f,%f,%f)", bbox[0], bbox[1], bbox[2], bbox[3])
	log.Printf("bbox %s", bb)

	roadQ := fmt.Sprintf(`[out:json][timeout:120];
way["highway"~"^(motorway|trunk|primary|secondary|tertiary|unclassified|residential|living_street|service|motorway_link|trunk_link|primary_link|secondary_link|tertiary_link)$"]%s;
out geom;`, bb)

	waterQ := fmt.Sprintf(`[out:json][timeout:120];
(
  way["natural"="water"]%s;
  way["waterway"~"^(river|canal|stream|riverbank)$"]%s;
  relation["natural"="water"]%s;
);
out geom;`, bb, bb, bb)

	poiQ := fmt.Sprintf(`[out:json][timeout:120];
node["amenity"~"^(restaurant|cafe|fast_food|bar|pub)$"]%s;
out skel;`, bb)

	log.Print("querying roads…")
	roads := query(roadQ)
	log.Printf("road ways: %d", len(roads.Elements))
	log.Print("querying water…")
	water := query(waterQ)
	log.Printf("water elements: %d", len(water.Elements))
	log.Print("querying POIs…")
	pois := query(poiQ)
	log.Printf("poi nodes: %d", len(pois.Elements))

	// ---- build node-keyed graph from OSM ways ----
	type key [2]int64 // lat/lon scaled by 1e6
	nodeIdx := map[key]int{}
	nodes := [][2]float64{}
	type rawEdge struct{ a, b, class int }
	rawEdges := []rawEdge{}

	getNode := func(c overpassCoord) int {
		k := key{int64(math.Round(c.Lat * 1e6)), int64(math.Round(c.Lon * 1e6))}
		if i, ok := nodeIdx[k]; ok {
			return i
		}
		i := len(nodes)
		nodeIdx[k] = i
		nodes = append(nodes, [2]float64{round5(c.Lat), round5(c.Lon)})
		return i
	}

	for _, el := range roads.Elements {
		if el.Type != "way" || len(el.Geometry) < 2 {
			continue
		}
		cls, ok := drivable[el.Tags["highway"]]
		if !ok {
			continue
		}
		prev := getNode(el.Geometry[0])
		for _, c := range el.Geometry[1:] {
			cur := getNode(c)
			if cur != prev {
				rawEdges = append(rawEdges, rawEdge{prev, cur, cls})
				prev = cur
			}
		}
	}
	log.Printf("raw nodes: %d, raw edges: %d", len(nodes), len(rawEdges))

	// ---- simplify: strip degree-2 nodes, merge each chain into one edge ----
	inc := make([][]int, len(nodes)) // incident raw-edge indices per node
	for i, e := range rawEdges {
		inc[e.a] = append(inc[e.a], i)
		inc[e.b] = append(inc[e.b], i)
	}
	other := func(ei, v int) int {
		if rawEdges[ei].a == v {
			return rawEdges[ei].b
		}
		return rawEdges[ei].a
	}

	keep := make([]bool, len(nodes))
	for v := range nodes {
		keep[v] = len(inc[v]) != 2
	}

	type simpEdge struct {
		a, b, cls int
		dist      float64
	}
	seenPair := map[[2]int]bool{}
	var simplified []simpEdge
	for v := 0; v < len(nodes); v++ {
		if !keep[v] {
			continue
		}
		for _, ei := range inc[v] {
			start := other(ei, v)
			pair := [2]int{v, start}
			if start < v {
				pair = [2]int{start, v}
			}
			if seenPair[pair] {
				continue
			}
			seenPair[pair] = true
			cls := rawEdges[ei].class
			dist := haversine(nodes[v], nodes[start])
			prev, cur := v, start
			for !keep[cur] { // walk chain of degree-2 nodes
				next := -1
				for _, ej := range inc[cur] {
					cand := other(ej, cur)
					if cand != prev {
						next = cand
						if rawEdges[ej].class == classMajor {
							cls = classMajor
						}
					}
				}
				if next < 0 {
					break
				}
				dist += haversine(nodes[cur], nodes[next])
				prev, cur = cur, next
			}
			if cur != v {
				simplified = append(simplified, simpEdge{v, cur, cls, dist})
			}
		}
	}

	// ---- keep only the largest connected component ----
	parent := make([]int, len(nodes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, e := range simplified {
		ra, rb := find(e.a), find(e.b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	rootSize := map[int]int{}
	for v := range nodes {
		rootSize[find(v)]++
	}
	bestRoot, bestSize := -1, 0
	for r, n := range rootSize {
		if n > bestSize {
			bestRoot, bestSize = r, n
		}
	}

	relabel := make([]int, len(nodes))
	finalNodes := [][2]float64{}
	deg := map[int]int{}
	for _, e := range simplified {
		if find(e.a) != bestRoot {
			continue
		}
		deg[e.a]++
		deg[e.b]++
	}
	for v := range nodes {
		if find(v) == bestRoot && deg[v] > 0 {
			relabel[v] = len(finalNodes)
			finalNodes = append(finalNodes, nodes[v])
		} else {
			relabel[v] = -1
		}
	}
	finalEdges := [][3]float64{}
	finalClasses := []int{}
	for _, e := range simplified {
		a, b := relabel[e.a], relabel[e.b]
		if a < 0 || b < 0 || a == b {
			continue
		}
		finalEdges = append(finalEdges, [3]float64{float64(a), float64(b), math.Round(e.dist*10) / 10})
		finalClasses = append(finalClasses, e.cls)
	}
	log.Printf("simplified: %d nodes, %d edges (largest component: %d raw nodes)", len(finalNodes), len(finalEdges), bestSize)

	// ---- POIs: snap to nearest kept node, dedupe, cap at 1500 ----
	poiSet := map[int]bool{}
	for _, el := range pois.Elements {
		if el.Type != "node" {
			continue
		}
		lat, lon := el.Lat, el.Lon
		if lat < bbox[0] || lat > bbox[2] || lon < bbox[1] || lon > bbox[3] {
			continue
		}
		poiSet[nearest(finalNodes, lat, lon)] = true
	}
	poiList := make([]int, 0, len(poiSet))
	for v := range poiSet {
		poiList = append(poiList, v)
	}
	sort.Ints(poiList)
	if len(poiList) > 1500 {
		rng := rand.New(rand.NewSource(42))
		rng.Shuffle(len(poiList), func(i, j int) { poiList[i], poiList[j] = poiList[j], poiList[i] })
		poiList = poiList[:1500]
		sort.Ints(poiList)
	}
	log.Printf("poi snapped: %d unique nodes", len(poiList))

	g := graphFile{
		Meta: meta{
			City:        "Berlin",
			Source:      "© OpenStreetMap contributors (Overpass API)",
			BBox:        bbox[:],
			GeneratedAt: time.Now().UTC().Format("2006-01-02"),
		},
		Nodes:   finalNodes,
		Edges:   finalEdges,
		Classes: finalClasses,
		POIs:    poiList,
	}
	writeJSON(*graphOut, g)

	// ---- roads GeoJSON (one feature per edge) ----
	type feature struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
		Geometry   map[string]interface{} `json:"geometry"`
	}
	type featureColl struct {
		Type     string    `json:"type"`
		Features []feature `json:"features"`
	}
	rf := featureColl{Type: "FeatureCollection"}
	rf.Features = make([]feature, 0, len(finalEdges))
	for i, e := range finalEdges {
		a, b := finalNodes[int(e[0])], finalNodes[int(e[1])]
		rf.Features = append(rf.Features, feature{
			Type:       "Feature",
			Properties: map[string]interface{}{"c": finalClasses[i]},
			Geometry: map[string]interface{}{
				"type":        "LineString",
				"coordinates": [][2]float64{{a[1], a[0]}, {b[1], b[0]}},
			},
		})
	}
	writeJSON(*roadsOut, rf)

	// ---- water GeoJSON (polygons + wide lines for river/canal centerlines) ----
	wf := featureColl{Type: "FeatureCollection"}
	addLine := func(coords []overpassCoord, width int) {
		if len(coords) < 2 {
			return
		}
		cs := make([][2]float64, 0, len(coords))
		for _, c := range coords {
			cs = append(cs, [2]float64{round5(c.Lon), round5(c.Lat)})
		}
		wf.Features = append(wf.Features, feature{
			Type: "Feature", Properties: map[string]interface{}{"w": width},
			Geometry: map[string]interface{}{"type": "LineString", "coordinates": cs},
		})
	}
	addPoly := func(coords []overpassCoord) {
		if len(coords) < 3 {
			return
		}
		cs := make([][2]float64, 0, len(coords))
		for _, c := range coords {
			cs = append(cs, [2]float64{round5(c.Lon), round5(c.Lat)})
		}
		if cs[0] != cs[len(cs)-1] {
			cs = append(cs, cs[0])
		}
		wf.Features = append(wf.Features, feature{
			Type: "Feature", Properties: map[string]interface{}{},
			Geometry: map[string]interface{}{"type": "Polygon", "coordinates": [][][2]float64{cs}},
		})
	}
	for _, el := range water.Elements {
		switch el.Type {
		case "way":
			switch t := el.Tags["waterway"]; t {
			case "river", "canal":
				addLine(el.Geometry, 14)
			case "stream":
				addLine(el.Geometry, 5)
			case "riverbank":
				addPoly(el.Geometry)
			default:
				if el.Tags["natural"] == "water" {
					addPoly(el.Geometry)
				}
			}
		case "relation":
			for _, m := range el.Members {
				if m.Role == "outer" || m.Role == "" {
					addPoly(m.Geometry)
				}
			}
		}
	}
	log.Printf("water features: %d", len(wf.Features))
	writeJSON(*waterOut, wf)
}

func query(q string) overpassResp {
	endpoints := []string{
		"https://overpass-api.de/api/interpreter",
		"https://overpass.kumi.systems/api/interpreter",
	}
	var lastErr error
	for attempt, ep := range endpoints {
		form := url.Values{"data": {q}}
		req, err := http.NewRequest("POST", ep, strings.NewReader(form.Encode()))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "lastmile-lab-graphgen/1.0 (portfolio project)")
		client := &http.Client{Timeout: 180 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("endpoint %s failed: %v", ep, err)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s -> HTTP %d", ep, resp.StatusCode)
			log.Printf("%v", lastErr)
			if attempt == len(endpoints)-1 {
				time.Sleep(10 * time.Second)
			}
			continue
		}
		var out overpassResp
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			resp.Body.Close()
			log.Fatalf("decode: %v", err)
		}
		resp.Body.Close()
		return out
	}
	log.Fatalf("all overpass endpoints failed: %v", lastErr)
	return overpassResp{}
}

func nearest(nodes [][2]float64, lat, lon float64) int {
	best, bestD := 0, math.MaxFloat64
	for i, n := range nodes {
		d := (n[0]-lat)*(n[0]-lat) + (n[1]-lon)*(n[1]-lon)
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func round5(v float64) float64 { return math.Round(v*1e5) / 1e5 }

func haversine(a, b [2]float64) float64 {
	const r = 6371000.0
	dLat := (b[0] - a[0]) * math.Pi / 180
	dLon := (b[1] - a[1]) * math.Pi / 180
	la := a[0] * math.Pi / 180
	lb := b[0] * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la)*math.Cos(lb)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

func writeJSON(path string, v interface{}) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(v); err != nil {
		log.Fatal(err)
	}
	fi, _ := f.Stat()
	log.Printf("wrote %s (%d KB)", path, fi.Size()/1024)
}
