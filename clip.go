package maplens

import (
	"math"
	"sort"
)

// eps 为坐标比较的容差（单位：度）。
const eps = 1e-9

// rect 为经纬度平面上的轴对齐矩形（可在展开坐标系中超出 ±180）。
type rect struct {
	minX, minY, maxX, maxY float64
}

// clipPolygonToRect 计算单个面（含洞）与矩形的交集，返回若干连通分量。
//
// 步骤：1) 用 Sutherland-Hodgman 把外环与各洞环分别裁到矩形；
// 2) 把所有环的有向边（实体始终在左侧）收集起来，落在矩形边界上的
// 共线边做一维拆分并抵消反向重复边（含挤压产生的零宽桥接、洞与外环
// 在视窗边上的重叠）；3) 在半边图上按"最右接续"规则追踪左侧 face，
// 正向环为分量外边界，负向环为洞，按包含关系归并。
func clipPolygonToRect(p Polygon, rc rect) []Polygon {
	var segs []seg
	collect := func(r Ring) {
		cr := clipRingToRect(r, rc)
		segs = append(segs, ringSegs(cr)...)
	}
	collect(p.Outer)
	for _, h := range p.Holes {
		collect(h)
	}
	segs = normalizeSegs(segs, rc)
	outers, holes := traceFaces(segs)
	return assemble(outers, holes)
}

// ---- Sutherland-Hodgman 环裁剪 ----

type halfPlane struct {
	axis   int // 0=X，1=Y
	value  float64
	keepGe bool // true 保留 coord>=value，false 保留 coord<=value
}

func axisCoord(p Point, axis int) float64 {
	if axis == 0 {
		return p.X
	}
	return p.Y
}

func clipRingToRect(r Ring, rc rect) Ring {
	planes := []halfPlane{
		{0, rc.minX, true},
		{0, rc.maxX, false},
		{1, rc.minY, true},
		{1, rc.maxY, false},
	}
	out := r
	for _, hp := range planes {
		out = clipHalfPlane(out, hp)
		if len(out) == 0 {
			return nil
		}
	}
	return out
}

func clipHalfPlane(r Ring, hp halfPlane) Ring {
	n := len(r)
	if n == 0 {
		return nil
	}
	inside := func(p Point) bool {
		c := axisCoord(p, hp.axis)
		if hp.keepGe {
			return c >= hp.value
		}
		return c <= hp.value
	}
	cross := func(a, b Point) Point {
		ca, cb := axisCoord(a, hp.axis), axisCoord(b, hp.axis)
		t := (hp.value - ca) / (cb - ca)
		p := Point{X: a.X + t*(b.X-a.X), Y: a.Y + t*(b.Y-a.Y)}
		if hp.axis == 0 {
			p.X = hp.value
		} else {
			p.Y = hp.value
		}
		return p
	}
	var out Ring
	s, sIn := r[n-1], inside(r[n-1])
	for i := 0; i < n; i++ {
		e := r[i]
		eIn := inside(e)
		if eIn {
			if !sIn {
				out = append(out, cross(s, e))
			}
			out = append(out, e)
		} else if sIn {
			out = append(out, cross(s, e))
		}
		s, sIn = e, eIn
	}
	return dedupeRing(out)
}

// dedupeRing 去掉相邻（含首尾）近似重复的点。
func dedupeRing(r Ring) Ring {
	out := r[:0]
	for _, p := range r {
		if len(out) == 0 || !nearEq(p, out[len(out)-1]) {
			out = append(out, p)
		}
	}
	for len(out) > 1 && nearEq(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

func nearEq(a, b Point) bool {
	return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps
}

// ---- 有向边收集、边界拆分与抵消 ----

type seg struct {
	a, b Point
}

// ringSegs 把环拆成有向边，跳过零长边。
func ringSegs(r Ring) []seg {
	n := len(r)
	var out []seg
	for i := 0; i < n; i++ {
		a, b := r[i], r[(i+1)%n]
		if !nearEq(a, b) {
			out = append(out, seg{a, b})
		}
	}
	return out
}

// normalizeSegs 把落在矩形四条边界上的共线边做一维拆分，
// 抵消正反重复的子边；其余边原样保留。
func normalizeSegs(segs []seg, rc rect) []seg {
	lines := []struct {
		axis  int
		value float64
	}{
		{0, rc.minX}, {0, rc.maxX}, {1, rc.minY}, {1, rc.maxY},
	}
	onLine := make([][]seg, len(lines))
	var plain []seg
next:
	for _, s := range segs {
		for li, ln := range lines {
			if math.Abs(axisCoord(s.a, ln.axis)-ln.value) < eps &&
				math.Abs(axisCoord(s.b, ln.axis)-ln.value) < eps {
				onLine[li] = append(onLine[li], s)
				continue next
			}
		}
		plain = append(plain, s)
	}
	for li, ln := range lines {
		plain = append(plain, cancelLineSegs(ln.axis, ln.value, onLine[li])...)
	}
	return plain
}

// cancelLineSegs 把同一条边界线上的有向边投影到一维，
// 按所有端点切成基本区间，正反计数抵消后重建有向边。
func cancelLineSegs(axis int, value float64, segs []seg) []seg {
	if len(segs) == 0 {
		return nil
	}
	var ts []float64
	for _, s := range segs {
		ts = append(ts, axisCoord(s.a, 1-axis), axisCoord(s.b, 1-axis))
	}
	sort.Float64s(ts)
	uniq := ts[:0]
	for _, t := range ts {
		if len(uniq) == 0 || t > uniq[len(uniq)-1] {
			uniq = append(uniq, t)
		}
	}
	mk := func(t1, t2 float64) seg {
		if axis == 0 {
			return seg{Point{value, t1}, Point{value, t2}}
		}
		return seg{Point{t1, value}, Point{t2, value}}
	}
	var out []seg
	for i := 0; i+1 < len(uniq); i++ {
		lo, hi := uniq[i], uniq[i+1]
		fwd, bwd := 0, 0
		for _, s := range segs {
			t1, t2 := axisCoord(s.a, 1-axis), axisCoord(s.b, 1-axis)
			if t1 > t2 {
				t1, t2 = t2, t1
			}
			if lo >= t1 && hi <= t2 {
				if axisCoord(s.a, 1-axis) < axisCoord(s.b, 1-axis) {
					fwd++
				} else {
					bwd++
				}
			}
		}
		switch {
		case fwd > bwd:
			out = append(out, mk(lo, hi))
		case bwd > fwd:
			out = append(out, mk(hi, lo))
		}
	}
	return out
}

// ---- 半边图 face 追踪 ----

// traceFaces 在有向边图上追踪每个 face（实体在边的左侧）。
// 返回正向环（分量外边界）与负向环（洞）。
func traceFaces(segs []seg) (outers, holes []Ring) {
	adj := map[Point][]int{}
	for i, s := range segs {
		adj[s.a] = append(adj[s.a], i)
	}
	visited := make([]bool, len(segs))
	pickNext := func(cur int) int {
		s := segs[cur]
		v := s.b
		twin := math.Atan2(s.a.Y-v.Y, s.a.X-v.X)
		best, bestDelta := -1, math.Inf(1)
		fallback := -1
		for _, j := range adj[v] {
			d := segs[j].b
			theta := math.Atan2(d.Y-v.Y, d.X-v.X)
			delta := math.Mod(twin-theta, 2*math.Pi)
			if delta < 0 {
				delta += 2 * math.Pi
			}
			if delta < 1e-12 {
				fallback = j // 原路返回（退化情形）
				continue
			}
			if delta < bestDelta {
				bestDelta, best = delta, j
			}
		}
		if best >= 0 {
			return best
		}
		return fallback
	}
	for start := range segs {
		if visited[start] {
			continue
		}
		var ring Ring
		cur := start
		for steps := 0; steps <= len(segs); steps++ {
			visited[cur] = true
			ring = append(ring, segs[cur].a)
			next := pickNext(cur)
			if next < 0 || next == start || visited[next] {
				break
			}
			cur = next
		}
		a := ring.SignedArea()
		switch {
		case a > eps:
			outers = append(outers, ring)
		case a < -eps:
			holes = append(holes, ring)
		}
	}
	return outers, holes
}

// assemble 把洞按包含关系归入对应分量。
func assemble(outers, holes []Ring) []Polygon {
	polys := make([]Polygon, len(outers))
	for i, o := range outers {
		polys[i] = Polygon{Outer: o}
	}
	for _, h := range holes {
		if len(h) == 0 {
			continue
		}
		for i := range polys {
			if pointInRing(h[0], polys[i].Outer) {
				polys[i].Holes = append(polys[i].Holes, h)
				break
			}
		}
	}
	return polys
}

// pointInRing 射线法判断点是否在环内（点在环边界上的情形按外部约定不出现）。
func pointInRing(pt Point, r Ring) bool {
	n := len(r)
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		a, b := r[i], r[j]
		if (a.Y > pt.Y) != (b.Y > pt.Y) &&
			pt.X < (b.X-a.X)*(pt.Y-a.Y)/(b.Y-a.Y)+a.X {
			inside = !inside
		}
		j = i
	}
	return inside
}
