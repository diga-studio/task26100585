package maplens

import (
	"math"
	"sort"
)

// viewport 表示一个视窗。x0/x1 为展开经度区间：
// west < east 时 [west, east]；west > east（跨日界线）时 [west, east+360]。
type viewport struct {
	x0, x1    float64
	south     float64
	north     float64
	wrapsDate bool
}

func newViewport(west, south, east, north float64) viewport {
	vp := viewport{x0: west, x1: east, south: south, north: north}
	if west > east {
		vp.x1 = east + 360
		vp.wrapsDate = true
	}
	return vp
}

// clipFeature 返回区域与视窗的交集（仅正面积部分），无交集时返回 nil。
func clipFeature(f Feature, vp viewport) *Feature {
	var srcs []Polygon
	switch g := f.Geometry.(type) {
	case Polygon:
		srcs = []Polygon{g}
	case MultiPolygon:
		srcs = []Polygon(g)
	default:
		return nil
	}
	var faces []Polygon
	for _, src := range srcs {
		norm := normalizePolygon(src)
		if len(norm) == 0 || len(norm[0]) < 4 {
			continue
		}
		gmin, gmax := polygonLonRange(norm)
		kmin := int(math.Ceil((vp.x0-gmax)/360 - 1e-9))
		kmax := int(math.Floor((vp.x1-gmin)/360 + 1e-9))
		for k := kmin; k <= kmax; k++ {
			shifted := shiftPolygon(norm, 360*float64(k))
			for _, face := range clipPolygonRect(shifted, vp.x0, vp.south, vp.x1, vp.north) {
				faces = append(faces, shiftPolygon(face, -360*float64(k)))
			}
		}
	}
	var out []Polygon
	for _, face := range faces {
		out = append(out, splitAtAntimeridian(face)...)
	}
	out = filterAndClamp(out)
	if len(out) == 0 {
		return nil
	}
	var geom Geometry
	if len(out) == 1 {
		geom = out[0]
	} else {
		geom = MultiPolygon(out)
	}
	return &Feature{ID: f.ID, Geometry: geom}
}

func polygonLonRange(p Polygon) (float64, float64) {
	min, max := math.Inf(1), math.Inf(-1)
	for _, ring := range p {
		for _, pt := range ring {
			if pt.Lon < min {
				min = pt.Lon
			}
			if pt.Lon > max {
				max = pt.Lon
			}
		}
	}
	return min, max
}

func shiftPolygon(p Polygon, d float64) Polygon {
	out := make(Polygon, len(p))
	for i, ring := range p {
		r := make(Ring, len(ring))
		for j, pt := range ring {
			r[j] = Point{Lon: pt.Lon + d, Lat: pt.Lat}
		}
		out[i] = r
	}
	return out
}

// splitAtAntimeridian 将面在 ±180 经线处切开，切出的部分平移回 [-180,180]。
func splitAtAntimeridian(face Polygon) []Polygon {
	min, max := polygonLonRange(face)
	if min >= -180-eps && max <= 180+eps {
		return []Polygon{face}
	}
	var out []Polygon
	if min < 180 {
		out = append(out, clipPolygonRect(face, -720, -90, 180, 90)...)
	}
	if max > 180 {
		for _, p := range clipPolygonRect(face, 180, -90, 720, 90) {
			out = append(out, shiftPolygon(p, -360))
		}
	}
	return out
}

// filterAndClamp 丢弃零面积面，并把经度收拢到 [-180,180]、纬度到 [-90,90]。
func filterAndClamp(faces []Polygon) []Polygon {
	out := faces[:0]
	for _, f := range faces {
		if len(f) == 0 || ringArea(f[0]) <= eps {
			continue
		}
		for _, ring := range f {
			for i := range ring {
				ring[i].Lon = math.Min(180, math.Max(-180, ring[i].Lon))
				ring[i].Lat = math.Min(90, math.Max(-90, ring[i].Lat))
			}
		}
		out = append(out, f)
	}
	return out
}

// ---------- 矩形裁剪核心 ----------

type seg struct{ a, b Point }

// clipPolygonRect 计算规范化多边形（外环逆时针、洞顺时针、经度连续）
// 与轴对齐矩形的交集，返回若干面（每个面同样满足外环逆时针、洞顺时针）。
func clipPolygonRect(p Polygon, x0, y0, x1, y1 float64) []Polygon {
	pool := newPointPool()
	var segs []seg
	// 1) 多边形各环落在矩形内的边（保持原方向）。
	for _, ring := range p {
		for i := 0; i+1 < len(ring); i++ {
			if s, ok := clipSegToRect(ring[i], ring[i+1], x0, y0, x1, y1); ok {
				segs = append(segs, seg{pool.get(s.a), pool.get(s.b)})
			}
		}
	}
	// 2) 矩形边界落在多边形内的部分（逆时针方向）。
	corners := [4]Point{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	for s := 0; s < 4; s++ {
		a, b := corners[s], corners[(s+1)%4]
		ts := []float64{0, 1}
		for _, ring := range p {
			for i := 0; i+1 < len(ring); i++ {
				if t, ok := intersectSide(a, b, ring[i], ring[i+1]); ok {
					ts = append(ts, t)
				}
			}
		}
		sort.Float64s(ts)
		ts = dedupeSorted(ts)
		for i := 0; i+1 < len(ts); i++ {
			t0, t1 := ts[i], ts[i+1]
			if t1-t0 < 1e-12 {
				continue
			}
			if sideSubInside(a, b, t0, t1, p) {
				pa := Point{a.Lon + t0*(b.Lon-a.Lon), a.Lat + t0*(b.Lat-a.Lat)}
				pb := Point{a.Lon + t1*(b.Lon-a.Lon), a.Lat + t1*(b.Lat-a.Lat)}
				segs = append(segs, seg{pool.get(pa), pool.get(pb)})
			}
		}
	}
	// 3) 抵消共线反向重复边。
	segs = cancelOpposite(segs)
	if len(segs) == 0 {
		return nil
	}
	// 4) 追踪面。
	return traceFaces(segs)
}

// pointPool 把坐标量化到 1e-9 网格，保证不同路径算出的同一点严格相等。
type pointPool struct {
	m map[[2]int64]Point
}

func newPointPool() *pointPool { return &pointPool{m: make(map[[2]int64]Point)} }

func (pp *pointPool) get(p Point) Point {
	k := [2]int64{int64(math.Round(p.Lon / eps)), int64(math.Round(p.Lat / eps))}
	if v, ok := pp.m[k]; ok {
		return v
	}
	q := Point{float64(k[0]) * eps, float64(k[1]) * eps}
	pp.m[k] = q
	return q
}

// clipSegToRect 用 Liang-Barsky 算法裁剪有向线段，保留矩形内部分（方向不变）。
func clipSegToRect(a, b Point, x0, y0, x1, y1 float64) (seg, bool) {
	dx, dy := b.Lon-a.Lon, b.Lat-a.Lat
	t0, t1 := 0.0, 1.0
	clip := func(p, q float64) bool {
		if p == 0 {
			return q >= 0
		}
		r := q / p
		if p < 0 {
			if r > t1 {
				return false
			}
			if r > t0 {
				t0 = r
			}
		} else {
			if r < t0 {
				return false
			}
			if r < t1 {
				t1 = r
			}
		}
		return true
	}
	if !clip(-dx, a.Lon-x0) || !clip(dx, x1-a.Lon) ||
		!clip(-dy, a.Lat-y0) || !clip(dy, y1-a.Lat) {
		return seg{}, false
	}
	if t1-t0 < 1e-15 {
		return seg{}, false
	}
	return seg{
		a: Point{a.Lon + t0*dx, a.Lat + t0*dy},
		b: Point{a.Lon + t1*dx, a.Lat + t1*dy},
	}, true
}

// intersectSide 求边 c->d 与矩形边 a->b 的交点，返回交点在 a->b 上的参数 t。
// 平行（含共线）不相交。
func intersectSide(a, b, c, d Point) (float64, bool) {
	rx, ry := b.Lon-a.Lon, b.Lat-a.Lat
	sx, sy := d.Lon-c.Lon, d.Lat-c.Lat
	den := rx*sy - ry*sx
	if math.Abs(den) < 1e-18 {
		return 0, false
	}
	qx, qy := c.Lon-a.Lon, c.Lat-a.Lat
	t := (qx*sy - qy*sx) / den
	s := (qx*ry - qy*rx) / den
	if t < -eps || t > 1+eps || s < -eps || s > 1+eps {
		return 0, false
	}
	return math.Min(1, math.Max(0, t)), true
}

func dedupeSorted(ts []float64) []float64 {
	out := ts[:0]
	for _, t := range ts {
		if len(out) == 0 || t-out[len(out)-1] > 1e-9 {
			out = append(out, t)
		}
	}
	return out
}

// sideSubInside 判断矩形边 a->b 的参数子段 [t0,t1] 是否在多边形内。
// 中点落在多边形边界上（共线或相切）时，改测 1/4 与 3/4 处。
func sideSubInside(a, b Point, t0, t1 float64, p Polygon) bool {
	at := func(t float64) Point {
		return Point{a.Lon + t*(b.Lon-a.Lon), a.Lat + t*(b.Lat-a.Lat)}
	}
	mid := at((t0 + t1) / 2)
	in, on := pointInPolygon(mid, p)
	if on {
		in1, _ := pointInPolygon(at(t0+(t1-t0)*0.25), p)
		in2, _ := pointInPolygon(at(t0+(t1-t0)*0.75), p)
		return in1 || in2
	}
	return in
}

// cancelOpposite 抵消同一对端点间方向相反的边。
func cancelOpposite(segs []seg) []seg {
	type pair struct{ a, b Point }
	cnt := make(map[pair]int)
	for _, s := range segs {
		if s.a == s.b {
			continue
		}
		if lessPt(s.b, s.a) {
			cnt[pair{s.b, s.a}]--
		} else {
			cnt[pair{s.a, s.b}]++
		}
	}
	var out []seg
	for pr, n := range cnt {
		for i := 0; i < abs(n); i++ {
			if n > 0 {
				out = append(out, seg{pr.a, pr.b})
			} else {
				out = append(out, seg{pr.b, pr.a})
			}
		}
	}
	return out
}

func lessPt(a, b Point) bool {
	if a.Lon != b.Lon {
		return a.Lon < b.Lon
	}
	return a.Lat < b.Lat
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type halfEdge struct{ a, b Point }

// traceFaces 从有向边集合（内部在左侧）追踪所有面，
// 返回正面积的面：外环逆时针，洞顺时针并挂到所属外环。
func traceFaces(segs []seg) []Polygon {
	adj := make(map[Point][]Point)
	for _, s := range segs {
		adj[s.a] = append(adj[s.a], s.b)
	}
	visited := make(map[halfEdge]bool)
	var outers, holes []Ring
	for _, s := range segs {
		if visited[halfEdge{s.a, s.b}] {
			continue
		}
		loop := Ring{s.a}
		cur, nxt := s.a, s.b
		ok := true
		for {
			visited[halfEdge{cur, nxt}] = true
			loop = append(loop, nxt)
			if nxt == s.a {
				break
			}
			w, found := nextEdge(cur, nxt, adj[nxt], visited)
			if !found {
				ok = false
				break
			}
			cur, nxt = nxt, w
		}
		if !ok || len(loop) < 4 {
			continue
		}
		a := signedArea(loop)
		if a > eps {
			outers = append(outers, loop)
		} else if a < -eps {
			holes = append(holes, loop)
		}
	}
	faces := make([]Polygon, len(outers))
	for i, o := range outers {
		faces[i] = Polygon{o}
	}
	for _, h := range holes {
		idx := -1
		best := math.Inf(1)
		pt := holeSamplePoint(h)
		for i, o := range outers {
			in, _ := pointInRing(pt, o)
			if in {
				if a := ringArea(o); a < best {
					best, idx = a, i
				}
			}
		}
		if idx >= 0 {
			faces[idx] = append(faces[idx], h)
		}
	}
	return faces
}

// nextEdge 在顶点 v 处选择下一条边：相对入边 u->v 的反向，取顺时针最近的方向
// （即让面始终保持在左侧）。
func nextEdge(u, v Point, cands []Point, visited map[halfEdge]bool) (Point, bool) {
	thetaR := math.Atan2(u.Lat-v.Lat, u.Lon-v.Lon)
	best := math.Inf(1)
	var bw Point
	found := false
	for _, w := range cands {
		if visited[halfEdge{v, w}] {
			continue
		}
		phi := math.Atan2(w.Lat-v.Lat, w.Lon-v.Lon)
		turn := math.Mod(thetaR-phi+4*math.Pi, 2*math.Pi)
		if turn < best {
			best, bw, found = turn, w, true
		}
	}
	return bw, found
}

// holeSamplePoint 取洞环上一条边的中点作为归属测试点。
func holeSamplePoint(h Ring) Point {
	m := h[0]
	best := -1.0
	for i := 0; i+1 < len(h); i++ {
		d := math.Hypot(h[i+1].Lon-h[i].Lon, h[i+1].Lat-h[i].Lat)
		if d > best {
			best = d
			m = Point{(h[i].Lon + h[i+1].Lon) / 2, (h[i].Lat + h[i+1].Lat) / 2}
		}
	}
	return m
}
