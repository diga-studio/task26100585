// Package maplens 提供地图视窗裁剪能力：从 GeoJSON 装载行政区域，
// 按经纬度视窗计算几何交集，结果可直接用于绘制。
package maplens

import "math"

// Point 为经纬度平面上的点，X 为经度，Y 为纬度（单位：度）。
type Point struct {
	X, Y float64
}

// Ring 为线性环，内部按不闭合方式存储（首尾点不重复）。
// 第一个环为外环的约定体现在 Polygon 结构中。
type Ring []Point

// Polygon 为一个面：Outer 为外环，Holes 为洞环。
type Polygon struct {
	Outer Ring
	Holes []Ring
}

// MultiPolygon 为多个面的集合。
type MultiPolygon []Polygon

// Viewport 为地图视窗。南界必须严格小于北界；
// 西界小于东界为普通视窗，西界大于东界为跨日界线视窗；
// 西 -180、东 180 表示覆盖全部经度。不支持东西界相等。
type Viewport struct {
	West, South, East, North float64
}

// Result 为单个区域的裁剪结果，保留原区域标识。
// Components 为该区域与视窗交集的各连通分量（含洞）。
type Result struct {
	ID         string
	Components []Polygon
}

// Geometry 返回 Polygon（单分量）或 MultiPolygon（多分量）。
func (r Result) Geometry() any {
	if len(r.Components) == 1 {
		return r.Components[0]
	}
	return MultiPolygon(r.Components)
}

// Area 返回结果各分量面积之和（平方度）。
func (r Result) Area() float64 {
	total := 0.0
	for _, p := range r.Components {
		total += p.Area()
	}
	return total
}

// Closed 返回闭合形式的环（末尾追加首点），用于输出 GeoJSON 等场景。
func (r Ring) Closed() Ring {
	if len(r) == 0 {
		return r
	}
	out := make(Ring, len(r), len(r)+1)
	copy(out, r)
	out = append(out, r[0])
	return out
}

// SignedArea 返回环的有符号面积，逆时针为正。
func (r Ring) SignedArea() float64 {
	n := len(r)
	if n < 3 {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		a, b := r[i], r[(i+1)%n]
		sum += a.X*b.Y - b.X*a.Y
	}
	return sum / 2
}

// Area 返回面的净面积（外环减洞）。
func (p Polygon) Area() float64 {
	a := math.Abs(p.Outer.SignedArea())
	for _, h := range p.Holes {
		a -= math.Abs(h.SignedArea())
	}
	return a
}

// NumHoles 返回洞的数量。
func (p Polygon) NumHoles() int { return len(p.Holes) }
