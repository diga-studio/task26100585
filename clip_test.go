package maplens

import (
	"math"
	"sort"
	"testing"
)

const tol = 1e-6

// components 把 Geometry 统一展开为面列表。
func components(g Geometry) []Polygon {
	switch v := g.(type) {
	case Polygon:
		return []Polygon{v}
	case MultiPolygon:
		return []Polygon(v)
	}
	return nil
}

func polygonArea(p Polygon) float64 {
	a := ringArea(p[0])
	for _, h := range p[1:] {
		a -= ringArea(h)
	}
	return a
}

func totalArea(comps []Polygon) float64 {
	sum := 0.0
	for _, c := range comps {
		sum += polygonArea(c)
	}
	return sum
}

// holeCounts 返回各面洞数（升序），便于断言。
func holeCounts(comps []Polygon) []int {
	out := make([]int, len(comps))
	for i, c := range comps {
		out[i] = len(c) - 1
	}
	sort.Ints(out)
	return out
}

// lonRange 返回面的经度范围。
func lonRange(p Polygon) (float64, float64) {
	min, max := math.Inf(1), math.Inf(-1)
	for _, pt := range p[0] {
		min = math.Min(min, pt.Lon)
		max = math.Max(max, pt.Lon)
	}
	return min, max
}

func findFeature(t *testing.T, feats []Feature, id string) Feature {
	t.Helper()
	for _, f := range feats {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("未找到区域 %q", id)
	return Feature{}
}

func almostEq(a, b float64) bool { return math.Abs(a-b) <= tol }

// 场景一：凹区域（拱形）被竖条视窗裁成两个分量，洞被切开消失。
//
// 区域：外环为拱形（顶带 y∈[2,4] 宽 12，两条腿 x∈[0,3] 与 x∈[9,12] 高 2），
// 洞为 [5,7]×[2.5,3.5]。视窗 [5.5,6.5]×[1,5] 只切到顶带，
// 洞把条带截成上下两个矩形 [5.5,6.5]×[2,2.5] 与 [5.5,6.5]×[3.5,4]，各 0.5。
func TestConcaveSplitAndHoleCut(t *testing.T) {
	lens, err := LoadFile("testdata/concave.geojson")
	if err != nil {
		t.Fatal(err)
	}
	feats := lens.Clip(5.5, 1, 6.5, 5)
	if len(feats) != 1 {
		t.Fatalf("期望 1 个区域，得到 %d", len(feats))
	}
	f := findFeature(t, feats, "concave-arch")
	comps := components(f.Geometry)
	if len(comps) != 2 {
		t.Fatalf("期望 2 个连通分量，得到 %d", len(comps))
	}
	for i, c := range comps {
		if got := len(c) - 1; got != 0 {
			t.Errorf("分量 %d: 期望 0 个洞（洞已被切开），得到 %d", i, got)
		}
	}
	if got := totalArea(comps); !almostEq(got, 1.0) {
		t.Errorf("期望总面积 1.0，得到 %v", got)
	}
	// 两个分量应分别为 y∈[2,2.5] 与 y∈[3.5,4] 的矩形。
	mids := []float64{}
	for _, c := range comps {
		minLat, maxLat := math.Inf(1), math.Inf(-1)
		for _, pt := range c[0] {
			minLat = math.Min(minLat, pt.Lat)
			maxLat = math.Max(maxLat, pt.Lat)
		}
		mids = append(mids, (minLat+maxLat)/2)
	}
	sort.Float64s(mids)
	if !almostEq(mids[0], 2.25) || !almostEq(mids[1], 3.75) {
		t.Errorf("分量纬度中心期望 [2.25, 3.75]，得到 %v", mids)
	}
}

// 场景二 a：带洞区域跨日界线，使用跨线视窗 [171,-171]×[1,9]。
//
// 区域外环 [170,190]×[0,10]（展开经度），洞 [172,176]×[2,8] 完全在东半侧。
// 交集在 180 处分成两侧：东侧 [171,180]×[1,9] 含完整洞（面积 72-24=48），
// 西侧 [-180,-171]×[1,9] 为矩形（面积 72）。
func TestDatelineWrappedViewport(t *testing.T) {
	lens, err := LoadFile("testdata/dateline.geojson")
	if err != nil {
		t.Fatal(err)
	}
	feats := lens.Clip(171, 1, -171, 9)
	if len(feats) != 1 {
		t.Fatalf("期望 1 个区域，得到 %d", len(feats))
	}
	f := findFeature(t, feats, "dateline-region")
	comps := components(f.Geometry)
	if len(comps) != 2 {
		t.Fatalf("期望 2 个连通分量（日界线两侧），得到 %d", len(comps))
	}
	hc := holeCounts(comps)
	if hc[0] != 0 || hc[1] != 1 {
		t.Fatalf("洞数期望 [0 1]，得到 %v", hc)
	}
	if got := totalArea(comps); !almostEq(got, 120.0) {
		t.Errorf("期望总面积 120，得到 %v", got)
	}
	// 东侧分量（经度为正）面积 48 且含洞；西侧（经度为负）面积 72。
	for _, c := range comps {
		min, max := lonRange(c)
		if max <= 180 && min >= 0 {
			if !almostEq(polygonArea(c), 48.0) || len(c)-1 != 1 {
				t.Errorf("东侧分量期望面积 48、1 洞，得到面积 %v、%d 洞", polygonArea(c), len(c)-1)
			}
		} else {
			if !almostEq(polygonArea(c), 72.0) || len(c)-1 != 0 {
				t.Errorf("西侧分量期望面积 72、0 洞，得到面积 %v、%d 洞", polygonArea(c), len(c)-1)
			}
		}
		if min < -180-tol || max > 180+tol {
			t.Errorf("分量经度超出 [-180,180]: [%v, %v]", min, max)
		}
	}
}

// 场景二 b：同一区域使用全经度视窗 [-180,180]。
//
// 整个区域被取中，但仍须在日界线处分成两侧：
// 东 [170,180]×[0,10] 含洞（100-24=76），西 [-180,-170]×[0,10]（100）。
func TestDatelineFullLongitudeViewport(t *testing.T) {
	lens, err := LoadFile("testdata/dateline.geojson")
	if err != nil {
		t.Fatal(err)
	}
	feats := lens.Clip(-180, -85, 180, 85)
	if len(feats) != 3 {
		t.Fatalf("期望 3 个区域，得到 %d", len(feats))
	}
	f := findFeature(t, feats, "dateline-region")
	comps := components(f.Geometry)
	if len(comps) != 2 {
		t.Fatalf("期望 2 个连通分量，得到 %d", len(comps))
	}
	hc := holeCounts(comps)
	if hc[0] != 0 || hc[1] != 1 {
		t.Fatalf("洞数期望 [0 1]，得到 %v", hc)
	}
	if got := totalArea(comps); !almostEq(got, 176.0) {
		t.Errorf("期望总面积 176，得到 %v", got)
	}
	// MultiPolygon 区域两个面完整保留。
	m := findFeature(t, feats, "multi-region")
	mcomps := components(m.Geometry)
	if len(mcomps) != 2 || !almostEq(totalArea(mcomps), 200.0) {
		t.Errorf("multi-region 期望 2 分量总面积 200，得到 %d 分量面积 %v", len(mcomps), totalArea(mcomps))
	}
}

// MultiPolygon 裁剪 + 仅接触视窗边的区域不返回。
func TestMultiPolygonClipAndEdgeTouchExcluded(t *testing.T) {
	lens, err := LoadFile("testdata/dateline.geojson")
	if err != nil {
		t.Fatal(err)
	}
	feats := lens.Clip(5, 5, 25, 25)
	if len(feats) != 1 {
		t.Fatalf("期望 1 个区域，得到 %d", len(feats))
	}
	f := findFeature(t, feats, "multi-region")
	comps := components(f.Geometry)
	if len(comps) != 2 {
		t.Fatalf("期望 2 个连通分量，得到 %d", len(comps))
	}
	if got := totalArea(comps); !almostEq(got, 50.0) {
		t.Errorf("期望总面积 50（[5,10]² 与 [20,25]²），得到 %v", got)
	}
	for _, c := range comps {
		if len(c)-1 != 0 {
			t.Errorf("期望 0 洞，得到 %d", len(c)-1)
		}
	}
}

// 视窗完全落在洞内时不返回；重复查询结果一致（针对原始几何）。
func TestViewportInsideHoleAndRepeatStable(t *testing.T) {
	lens, err := LoadFile("testdata/dateline.geojson")
	if err != nil {
		t.Fatal(err)
	}
	// 视窗 [173,175]×[3,7] 完全落在洞 [172,176]×[2,8] 内。
	if feats := lens.Clip(173, 3, 175, 7); len(feats) != 0 {
		t.Fatalf("视窗落在洞内，期望 0 个区域，得到 %d", len(feats))
	}
	a := lens.Clip(171, 1, -171, 9)
	b := lens.Clip(171, 1, -171, 9)
	if len(a) != len(b) {
		t.Fatalf("重复查询结果不一致: %d vs %d", len(a), len(b))
	}
	for i := range a {
		ca, cb := components(a[i].Geometry), components(b[i].Geometry)
		if len(ca) != len(cb) || !almostEq(totalArea(ca), totalArea(cb)) {
			t.Fatalf("重复查询几何不一致: 区域 %q", a[i].ID)
		}
	}
}
