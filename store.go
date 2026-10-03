package maplens

import (
	"fmt"
	"math"
)

// Store 保存装载后的区域集合（内存态），支持反复视窗查询。
// 查询始终针对原始几何，不受之前裁剪影响。
type Store struct {
	regions []storedRegion
}

type storedRegion struct {
	id    string
	polys []Polygon // 已展开到连续经度区间
}

// Clip 返回各区域与视窗的几何交集，只保留有正面积的部分。
// 同一区域的全部分量归在一个 Result 中；无交集的区域不出现在返回值中。
// 输出经度保持在 [-180, 180]，跨日界线的分量在日界线处分成两侧的面。
func (s *Store) Clip(v Viewport) ([]Result, error) {
	rc, err := viewportRect(v)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, region := range s.regions {
		var comps []Polygon
		for _, poly := range region.polys {
			minX, maxX := polyLonRange(poly)
			// 视窗在展开坐标系中可能跨 ±180，把面平移 360 的整数倍逐一求交。
			kMin := int(math.Ceil((rc.minX - maxX) / 360))
			kMax := int(math.Floor((rc.maxX - minX) / 360))
			for k := kMin; k <= kMax; k++ {
				shifted := shiftPolygon(poly, 360*float64(k))
				comps = append(comps, clipPolygonToRect(shifted, rc)...)
			}
		}
		var wrapped []Polygon
		for _, c := range comps {
			wrapped = append(wrapped, splitAtMeridians(c)...)
		}
		if len(wrapped) > 0 {
			results = append(results, Result{ID: region.id, Components: wrapped})
		}
	}
	return results, nil
}

// viewportRect 把视窗换算为展开坐标系中的矩形：
// 西<东为普通视窗；西>东时东界加 360 表示跨日界线；
// 西 -180、东 180 即覆盖全部经度。
func viewportRect(v Viewport) (rect, error) {
	if v.West < -180 || v.West > 180 || v.East < -180 || v.East > 180 {
		return rect{}, fmt.Errorf("maplens: 视窗东西界须在 [-180, 180] 内: %v, %v", v.West, v.East)
	}
	if v.South < -90 || v.North > 90 || v.South >= v.North {
		return rect{}, fmt.Errorf("maplens: 视窗南界须严格小于北界且在 [-90, 90] 内: %v, %v", v.South, v.North)
	}
	if v.West == v.East {
		return rect{}, fmt.Errorf("maplens: 不支持东西界相等的视窗: %v", v.West)
	}
	if v.West < v.East {
		return rect{minX: v.West, minY: v.South, maxX: v.East, maxY: v.North}, nil
	}
	return rect{minX: v.West, minY: v.South, maxX: v.East + 360, maxY: v.North}, nil
}

// splitAtMeridians 把展开坐标系中的面在 ±180+360k 经线处切开，
// 并平移回 [-180, 180]，避免输出横跨地图的长边。
func splitAtMeridians(p Polygon) []Polygon {
	minX, maxX := polyLonRange(p)
	mMin := int(math.Floor((minX + 180) / 360))
	mMax := int(math.Floor((maxX + 180) / 360))
	var out []Polygon
	for m := mMin; m <= mMax; m++ {
		band := rect{minX: -180 + 360*float64(m), minY: -90, maxX: 180 + 360*float64(m), maxY: 90}
		for _, piece := range clipPolygonToRect(p, band) {
			out = append(out, shiftPolygon(piece, -360*float64(m)))
		}
	}
	return out
}

func polyLonRange(p Polygon) (minX, maxX float64) {
	minX, maxX = math.Inf(1), math.Inf(-1)
	for _, pt := range p.Outer {
		minX = math.Min(minX, pt.X)
		maxX = math.Max(maxX, pt.X)
	}
	return minX, maxX
}

func shiftPolygon(p Polygon, dx float64) Polygon {
	if dx == 0 {
		return p
	}
	shiftRing := func(r Ring) Ring {
		out := make(Ring, len(r))
		for i, pt := range r {
			out[i] = Point{X: pt.X + dx, Y: pt.Y}
		}
		return out
	}
	out := Polygon{Outer: shiftRing(p.Outer)}
	for _, h := range p.Holes {
		out.Holes = append(out.Holes, shiftRing(h))
	}
	return out
}
