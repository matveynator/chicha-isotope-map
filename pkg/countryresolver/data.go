package countryresolver

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"chicha-isotope-map/public_html/geojson"
)

// datasetPath reflects the URL exposed by the web server so error messages
// stay useful to operators watching logs.
const datasetPath = "/static/geojson/ne_10m_admin_0_countries.geojson"

// countries holds the parsed polygons grouped by ISO code.  The slice is
// populated during package initialisation so Resolve can stay allocation
// free at runtime.
var countries []country

// spatialIndex keeps a packed R-tree of every polygon.  Querying this tree
// gives us a tiny candidate set before we run the slower point-in-polygon
// math.
var spatialIndex *treeNode

// nameByCode is constructed from the loaded dataset so NameFor can keep its
// simple map lookup behaviour.
var nameByCode map[string]string

// init loads the embedded GeoJSON once so that worker goroutines in
// resolver.go always see a ready-to-use index.
func init() {
	loadedCountries, index, err := loadDataset()
	if err != nil {
		panic(fmt.Sprintf("countryresolver: %v", err))
	}
	countries = loadedCountries
	spatialIndex = index
	nameByCode = buildNameIndex(countries)
}

// ===== GeoJSON model =====

type geoFeatureCollection struct {
	Type     string       `json:"type"`
	Features []geoFeature `json:"features"`
}

type geoFeature struct {
	Properties geoProperties `json:"properties"`
	Geometry   geoGeometry   `json:"geometry"`
}

type geoProperties struct {
	ISOA2    string `json:"iso_a2"`
	ISOA2EH  string `json:"iso_a2_eh"`
	WBA2     string `json:"wb_a2"`
	Postal   string `json:"postal"`
	ADM0A3   string `json:"adm0_a3"`
	NameEN   string `json:"name_en"`
	Name     string `json:"name"`
	NameLong string `json:"name_long"`
}

type geoGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// ===== Public dataset structures =====

type country struct {
	code     string
	name     string
	polygons []polygon
}

type polygon struct {
	bbox  boundingBox
	rings []ring
	area  float64
}

type boundingBox struct {
	minLat, maxLat float64
	minLon, maxLon float64
}

type ring struct {
	points []point
}

type point struct {
	lat float64
	lon float64
}

type treeEntry struct {
	box          boundingBox
	countryIndex int
	polygonIndex int
}

type treeNode struct {
	box      boundingBox
	leaf     bool
	children []*treeNode
	entries  []treeEntry
}

type candidate struct {
	countryIndex int
	polygonIndex int
}

// codeInfo notes how an ISO code was obtained so loaders can treat
// fallbacks carefully without guesswork.
type codeInfo struct {
	code       string
	canonical  bool
	fromPostal bool
}

// ===== Dataset loading =====

func loadDataset() ([]country, *treeNode, error) {
	var collection geoFeatureCollection
	if len(geojson.Countries) == 0 {
		return nil, nil, fmt.Errorf("embedded dataset %s is empty", datasetPath)
	}
	if err := json.Unmarshal(geojson.Countries, &collection); err != nil {
		return nil, nil, fmt.Errorf("decode geojson %s: %w", datasetPath, err)
	}
	if strings.ToLower(collection.Type) != "featurecollection" {
		return nil, nil, fmt.Errorf("unexpected geojson type %q in %s", collection.Type, datasetPath)
	}
	if len(collection.Features) == 0 {
		return nil, nil, fmt.Errorf("geojson %s contains no features", datasetPath)
	}

	grouped := make(map[string]*country)
	canonicalSeen := make(map[string]bool)
	for _, feature := range collection.Features {
		info := normaliseISO(feature.Properties)
		if info.code == "" {
			continue
		}
		polys, err := feature.Geometry.asPolygons()
		if err != nil {
			// Skip malformed geometries so one bad polygon never
			// takes the resolver offline.
			continue
		}
		if len(polys) == 0 {
			continue
		}
		name := feature.Properties.englishName()
		c, ok := grouped[info.code]
		if !ok {
			c = &country{code: info.code}
			grouped[info.code] = c
		}
		if info.canonical {
			canonicalSeen[info.code] = true
			if name != "" {
				c.name = name
			}
			c.polygons = append(c.polygons, polys...)
			continue
		}
		if info.fromPostal && canonicalSeen[info.code] {
			// Postal codes are often reused for disputed regions.
			// Once we have a canonical country we skip these
			// entries to avoid renaming unrelated states.
			continue
		}
		if c.name == "" && name != "" {
			c.name = name
		}
		c.polygons = append(c.polygons, polys...)
	}

	if len(grouped) == 0 {
		return nil, nil, fmt.Errorf("geojson %s produced no usable countries", datasetPath)
	}

	list := make([]country, 0, len(grouped))
	for _, c := range grouped {
		list = append(list, *c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].code < list[j].code })

	entries := make([]treeEntry, 0)
	for countryIdx := range list {
		polys := list[countryIdx].polygons
		for polyIdx := range polys {
			entries = append(entries, treeEntry{
				box:          polys[polyIdx].bbox,
				countryIndex: countryIdx,
				polygonIndex: polyIdx,
			})
		}
	}
	index := buildRTree(entries, 16)
	if index == nil {
		return nil, nil, fmt.Errorf("failed to build spatial index")
	}
	return list, index, nil
}

func (p geoProperties) englishName() string {
	for _, candidate := range []string{p.NameEN, p.NameLong, p.Name} {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normaliseISO(p geoProperties) codeInfo {
	candidates := []struct {
		value      string
		canonical  bool
		fromPostal bool
	}{
		{p.ISOA2, true, false},
		{p.ISOA2EH, true, false},
		{p.WBA2, true, false},
		{p.Postal, false, true},
	}
	for _, cand := range candidates {
		code := strings.ToUpper(strings.TrimSpace(cand.value))
		if len(code) == 2 && code != "-9" && code != "-99" {
			return codeInfo{code: code, canonical: cand.canonical, fromPostal: cand.fromPostal}
		}
	}
	upper := strings.ToUpper(strings.TrimSpace(p.ADM0A3))
	if code, ok := specialA3ToA2[upper]; ok {
		return codeInfo{code: code}
	}
	if len(upper) == 2 && upper != "" {
		return codeInfo{code: upper}
	}
	return codeInfo{}
}

var specialA3ToA2 = map[string]string{
	"BJN": "CO", // Bajo Nuevo is disputed but treated as part of Colombia for reporting.
	"CNM": "CY", // The UN buffer zone is associated with Cyprus for display.
	"IOA": "AU", // Australian Indian Ocean Territories operate under Australia.
	"NOR": "NO", // Norway only exposes an ADM0 code in this dataset.
}

func (g geoGeometry) asPolygons() ([]polygon, error) {
	switch g.Type {
	case "Polygon":
		var coords [][][]float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
			return nil, fmt.Errorf("decode polygon: %w", err)
		}
		poly := buildPolygon(coords)
		if len(poly.rings) == 0 {
			return nil, nil
		}
		return []polygon{poly}, nil
	case "MultiPolygon":
		var coords [][][][]float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
			return nil, fmt.Errorf("decode multipolygon: %w", err)
		}
		polys := make([]polygon, 0, len(coords))
		for _, raw := range coords {
			poly := buildPolygon(raw)
			if len(poly.rings) == 0 {
				continue
			}
			polys = append(polys, poly)
		}
		return polys, nil
	default:
		return nil, nil
	}
}

func buildPolygon(raw [][][]float64) polygon {
	rings := make([]ring, 0, len(raw))
	box := newEmptyBox()
	for _, segment := range raw {
		pts := make([]point, 0, len(segment))
		for _, coord := range segment {
			if len(coord) < 2 {
				continue
			}
			lon := coord[0]
			lat := coord[1]
			pts = append(pts, point{lat: lat, lon: lon})
			box.expand(lat, lon)
		}
		if len(pts) >= 3 {
			rings = append(rings, ring{points: pts})
		}
	}
	if len(rings) == 0 || !box.valid() {
		return polygon{}
	}
	return polygon{bbox: box, rings: rings, area: polygonArea(rings)}
}

// ===== R-tree construction =====

func buildRTree(entries []treeEntry, maxEntries int) *treeNode {
	if len(entries) == 0 {
		return nil
	}
	nodes := packEntries(entries, maxEntries)
	for len(nodes) > 1 {
		nodes = packNodes(nodes, maxEntries)
	}
	return nodes[0]
}

func packEntries(entries []treeEntry, maxEntries int) []*treeNode {
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].box.minLon < entries[j].box.minLon })
	nodeCount := int(math.Ceil(float64(len(entries)) / float64(maxEntries)))
	sliceCount := int(math.Ceil(math.Sqrt(float64(nodeCount))))
	if sliceCount < 1 {
		sliceCount = 1
	}
	sliceCapacity := int(math.Ceil(float64(len(entries)) / float64(sliceCount)))
	nodes := make([]*treeNode, 0, nodeCount)
	for i := 0; i < len(entries); i += sliceCapacity {
		end := i + sliceCapacity
		if end > len(entries) {
			end = len(entries)
		}
		slice := append([]treeEntry(nil), entries[i:end]...)
		sort.Slice(slice, func(i, j int) bool { return slice[i].box.minLat < slice[j].box.minLat })
		for j := 0; j < len(slice); j += maxEntries {
			blockEnd := j + maxEntries
			if blockEnd > len(slice) {
				blockEnd = len(slice)
			}
			block := append([]treeEntry(nil), slice[j:blockEnd]...)
			node := &treeNode{leaf: true, entries: block}
			node.box = entriesBounds(block)
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func packNodes(children []*treeNode, maxEntries int) []*treeNode {
	if len(children) == 0 {
		return nil
	}
	if len(children) <= maxEntries {
		parent := &treeNode{leaf: false, children: append([]*treeNode(nil), children...)}
		parent.box = nodesBounds(parent.children)
		return []*treeNode{parent}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].box.minLon < children[j].box.minLon })
	nodeCount := int(math.Ceil(float64(len(children)) / float64(maxEntries)))
	sliceCount := int(math.Ceil(math.Sqrt(float64(nodeCount))))
	if sliceCount < 1 {
		sliceCount = 1
	}
	sliceCapacity := int(math.Ceil(float64(len(children)) / float64(sliceCount)))
	parents := make([]*treeNode, 0, nodeCount)
	for i := 0; i < len(children); i += sliceCapacity {
		end := i + sliceCapacity
		if end > len(children) {
			end = len(children)
		}
		slice := append([]*treeNode(nil), children[i:end]...)
		sort.Slice(slice, func(i, j int) bool { return slice[i].box.minLat < slice[j].box.minLat })
		for j := 0; j < len(slice); j += maxEntries {
			blockEnd := j + maxEntries
			if blockEnd > len(slice) {
				blockEnd = len(slice)
			}
			block := append([]*treeNode(nil), slice[j:blockEnd]...)
			parent := &treeNode{leaf: false, children: block}
			parent.box = nodesBounds(block)
			parents = append(parents, parent)
		}
	}
	return parents
}

// ===== Geometry helpers =====

func (b *boundingBox) expand(lat, lon float64) {
	if lat < b.minLat {
		b.minLat = lat
	}
	if lat > b.maxLat {
		b.maxLat = lat
	}
	if lon < b.minLon {
		b.minLon = lon
	}
	if lon > b.maxLon {
		b.maxLon = lon
	}
}

func (b boundingBox) contains(lat, lon float64) bool {
	if !b.valid() {
		return false
	}
	if lat < b.minLat || lat > b.maxLat {
		return false
	}
	if lon < b.minLon || lon > b.maxLon {
		return false
	}
	return true
}

func (b boundingBox) valid() bool {
	return b.minLat <= b.maxLat && b.minLon <= b.maxLon
}

func (b *boundingBox) include(other boundingBox) {
	if !other.valid() {
		return
	}
	b.expand(other.minLat, other.minLon)
	b.expand(other.minLat, other.maxLon)
	b.expand(other.maxLat, other.minLon)
	b.expand(other.maxLat, other.maxLon)
}

func newEmptyBox() boundingBox {
	return boundingBox{
		minLat: math.MaxFloat64,
		minLon: math.MaxFloat64,
		maxLat: -math.MaxFloat64,
		maxLon: -math.MaxFloat64,
	}
}

func entriesBounds(entries []treeEntry) boundingBox {
	box := newEmptyBox()
	for _, e := range entries {
		box.include(e.box)
	}
	return box
}

func nodesBounds(nodes []*treeNode) boundingBox {
	box := newEmptyBox()
	for _, n := range nodes {
		box.include(n.box)
	}
	return box
}

func polygonArea(rings []ring) float64 {
	if len(rings) == 0 {
		return 0
	}
	outer := math.Abs(ringArea(rings[0]))
	holes := 0.0
	for _, hole := range rings[1:] {
		holes += math.Abs(ringArea(hole))
	}
	area := outer - holes
	if area < 0 {
		return 0
	}
	return area
}

func ringArea(r ring) float64 {
	pts := r.points
	if len(pts) < 3 {
		return 0
	}
	sum := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		sum += pts[i].lon*pts[j].lat - pts[j].lon*pts[i].lat
	}
	return sum / 2
}

func (p polygon) contains(lat, lon float64) bool {
	if !p.bbox.contains(lat, lon) {
		return false
	}
	if len(p.rings) == 0 {
		return false
	}
	if !pointInRing(p.rings[0], lat, lon) {
		return false
	}
	for _, hole := range p.rings[1:] {
		if pointInRing(hole, lat, lon) {
			return false
		}
	}
	return true
}

func pointInRing(r ring, lat, lon float64) bool {
	pts := r.points
	if len(pts) < 3 {
		return false
	}
	inside := false
	j := len(pts) - 1
	for i := 0; i < len(pts); i++ {
		pi := pts[i]
		pj := pts[j]
		if pointOnSegment(pi, pj, lat, lon) {
			return true
		}
		if (pi.lat > lat) != (pj.lat > lat) {
			crossLon := (pj.lon-pi.lon)*(lat-pi.lat)/(pj.lat-pi.lat) + pi.lon
			if lon < crossLon {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

func pointOnSegment(a, b point, lat, lon float64) bool {
	cross := (b.lon-a.lon)*(lat-a.lat) - (b.lat-a.lat)*(lon-a.lon)
	if math.Abs(cross) > 1e-9 {
		return false
	}
	minLon := math.Min(a.lon, b.lon) - 1e-9
	maxLon := math.Max(a.lon, b.lon) + 1e-9
	minLat := math.Min(a.lat, b.lat) - 1e-9
	maxLat := math.Max(a.lat, b.lat) + 1e-9
	return lon >= minLon && lon <= maxLon && lat >= minLat && lat <= maxLat
}

// streamCandidates walks the R-tree and emits every polygon bounding box
// that may contain the point.  A done channel stops the traversal as soon
// as the caller finds a match.
func streamCandidates(lat, lon float64, out chan<- candidate, done <-chan struct{}) {
	walkTree(spatialIndex, lat, lon, out, done)
}

func walkTree(node *treeNode, lat, lon float64, out chan<- candidate, done <-chan struct{}) {
	if node == nil || !node.box.contains(lat, lon) {
		return
	}
	if node.leaf {
		for _, entry := range node.entries {
			if !entry.box.contains(lat, lon) {
				continue
			}
			select {
			case out <- candidate{countryIndex: entry.countryIndex, polygonIndex: entry.polygonIndex}:
			case <-done:
				return
			}
		}
		return
	}
	for _, child := range node.children {
		if !child.box.contains(lat, lon) {
			continue
		}
		walkTree(child, lat, lon, out, done)
		select {
		case <-done:
			return
		default:
		}
	}
}
