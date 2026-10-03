package maplens

import "math"

const eps = 1e-9

// signedArea 返回环的有向面积（逆时针为正）。
func signedArea(r Ring) float64 {
	sum := 0.0
	for i := 0; i+1 < len(r); i++ {
		sum += r[i].Lon*r[i+1].Lat - r[i+1].Lon*r[i].Lat
	}
	return sum / 2
}

// ringArea 返回环的绝对面积。
func ringArea(r Ring) float64 { return math.Abs(signedArea(r)) }

// unwrapRing 将环展开到连续经度区间：相邻顶点取经度差绝对值小于 180 的唯一走向。
// 输入环已闭合；展开后仍闭合（宽度小于 180 由调用方保证）。
func unwrapRing(r Ring) Ring {
	if len(r) == 0 {
		return nil
	}
	out := make(Ring, len(r))
	out[0] = r[0]
	for i := 1; i < len(r); i++ {
		d := r[i].Lon - r[i-1].Lon
		for d >= 180 {
			d -= 360
		}
		for d < -180 {
			d += 360
		}
		out[i] = Point{Lon: out[i-1].Lon + d, Lat: r[i].Lat}
	}
	return out
}

// normalizePolygon 将多边形各环展开到同一连续经度区间，
// 并规范方向：外环逆时针，洞顺时针。
func normalizePolygon(p Polygon) Polygon {
	if len(p) == 0 {
		return nil
	}
	out := make(Polygon, len(p))
	out[0] = unwrapRing(p[0])
	if signedArea(out[0]) < 0 {
		reverseRing(out[0])
	}
	base := out[0][0].Lon
	for i := 1; i < len(p); i++ {
		h := unwrapRing(p[i])
		// 洞严格位于外环内，整体平移使其起点靠近外环起点。
		shift := math.Round((base - h[0].Lon) / 360)
		for j := range h {
			h[j].Lon += shift * 360
		}
		if signedArea(h) > 0 {
			reverseRing(h)
		}
		out[i] = h
	}
	return out
}

func reverseRing(r Ring) {
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
}

// pointInRing 判断点是否在环内（射线法，边界上返回 on=true）。
func pointInRing(pt Point, r Ring) (inside, on bool) {
	on = false
	inside = false
	n := len(r)
	for i := 0; i+1 < n; i++ {
		a, b := r[i], r[i+1]
		if pointOnSegment(pt, a, b) {
			return false, true
		}
		if (a.Lat > pt.Lat) != (b.Lat > pt.Lat) {
			x := a.Lon + (pt.Lat-a.Lat)*(b.Lon-a.Lon)/(b.Lat-a.Lat)
			if x > pt.Lon {
				inside = !inside
			}
		}
	}
	return inside, false
}

// pointInPolygon 判断点是否在多边形内（外环内且不在任何洞内；边界上返回 on=true）。
func pointInPolygon(pt Point, p Polygon) (inside, on bool) {
	in, on := pointInRing(pt, p[0])
	if on || !in {
		return false, on
	}
	for _, h := range p[1:] {
		hin, hon := pointInRing(pt, h)
		if hon {
			return false, true
		}
		if hin {
			return false, false
		}
	}
	return true, false
}

func pointOnSegment(pt, a, b Point) bool {
	cross := (b.Lon-a.Lon)*(pt.Lat-a.Lat) - (b.Lat-a.Lat)*(pt.Lon-a.Lon)
	scale := math.Abs(b.Lon-a.Lon) + math.Abs(b.Lat-a.Lat) + 1
	if math.Abs(cross) > eps*scale*scale {
		return false
	}
	return pt.Lon >= math.Min(a.Lon, b.Lon)-eps && pt.Lon <= math.Max(a.Lon, b.Lon)+eps &&
		pt.Lat >= math.Min(a.Lat, b.Lat)-eps && pt.Lat <= math.Max(a.Lat, b.Lat)+eps
}
