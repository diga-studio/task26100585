package maplens

import (
	"math"
	"reflect"
	"sort"
	"testing"
)

func loadTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := LoadFile("testdata/regions.geojson")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return s
}

func findResult(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("结果中缺少区域 %q", id)
	return Result{}
}

// sortedStats 返回各分量面积（升序）与洞数（升序）。
func sortedStats(r Result) (areas []float64, holes []int) {
	for _, c := range r.Components {
		areas = append(areas, c.Area())
		holes = append(holes, c.NumHoles())
	}
	sort.Float64s(areas)
	sort.Ints(holes)
	return areas, holes
}

func assertStats(t *testing.T, r Result, wantAreas []float64, wantHoles []int) {
	t.Helper()
	areas, holes := sortedStats(r)
	if len(areas) != len(wantAreas) {
		t.Fatalf("区域 %q 连通分量数 = %d，期望 %d", r.ID, len(areas), len(wantAreas))
	}
	for i := range areas {
		if math.Abs(areas[i]-wantAreas[i]) > 1e-9 {
			t.Errorf("区域 %q 分量面积[%d] = %v，期望 %v", r.ID, i, areas[i], wantAreas[i])
		}
	}
	if !reflect.DeepEqual(holes, wantHoles) {
		t.Errorf("区域 %q 各分量洞数 = %v，期望 %v", r.ID, holes, wantHoles)
	}
}

func assertLonRange(t *testing.T, r Result) {
	t.Helper()
	for _, c := range r.Components {
		rings := append([]Ring{c.Outer}, c.Holes...)
		for _, ring := range rings {
			for i, p := range ring {
				if p.X < -180-1e-9 || p.X > 180+1e-9 {
					t.Errorf("区域 %q 输出经度越界: %v", r.ID, p.X)
				}
				next := ring[(i+1)%len(ring)]
				if math.Abs(next.X-p.X) > 180+1e-9 {
					t.Errorf("区域 %q 存在横跨地图的长边: %v -> %v", r.ID, p, next)
				}
			}
		}
	}
}

// 凹区域（H 形）被视窗切成上下两个分量，上方洞被北界切开成为缺口。
func TestConcaveSplitIntoComponents(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: 8, South: -1, East: 12, North: 8})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("结果区域数 = %d，期望 1", len(results))
	}
	r := findResult(t, results, "concave")
	// 下条 4x4 含一个 2x2 的洞（面积 12，1 洞）；
	// 上条 4x2 的洞被北界切成缺口（面积 6，0 洞）。
	assertStats(t, r, []float64{6, 12}, []int{0, 1})
	assertLonRange(t, r)
	if _, ok := r.Geometry().(MultiPolygon); !ok {
		t.Errorf("多分量结果应返回 MultiPolygon，实际 %T", r.Geometry())
	}
}

// 带洞区域跨日界线，使用跨线视窗：洞被视窗东西界切成缺口，结果在日界线分成两侧。
func TestPacificCrossAntimeridianViewport(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: 175, South: -5, East: -175, North: 15})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("结果区域数 = %d，期望 1", len(results))
	}
	r := findResult(t, results, "pacific")
	// 视窗覆盖展开经度 [175,185]：两侧各 5x10 的面，
	// 两个洞分别被西界、东界切开成缺口（各 3x6），无封闭洞。
	assertStats(t, r, []float64{32, 32}, []int{0, 0})
	assertLonRange(t, r)
}

// 跨线视窗完整包含跨日界线的区域：结果必须在日界线处分成两侧的面，
// 两侧各保留一个完整的洞。
func TestPacificContainedInCrossViewport(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: 160, South: -5, East: -160, North: 15})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	r := findResult(t, results, "pacific")
	assertStats(t, r, []float64{64, 64}, []int{1, 1})
	assertLonRange(t, r)
	// 两侧分量应分别位于日界线东西两边。
	var westSide, eastSide bool
	for _, c := range r.Components {
		minX, maxX := polyLonRange(c)
		if minX >= 170-1e-9 && maxX <= 180+1e-9 {
			westSide = true
		}
		if minX >= -180-1e-9 && maxX <= -170+1e-9 {
			eastSide = true
		}
	}
	if !westSide || !eastSide {
		t.Errorf("跨日界线结果未分成两侧的面: west=%v east=%v", westSide, eastSide)
	}
}

// 同一区域使用全经度视窗：日界线两侧各一个分量，各保留一个完整的洞。
func TestPacificFullLongitudeViewport(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: -180, South: -90, East: 180, North: 90})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("结果区域数 = %d，期望 2", len(results))
	}
	pacific := findResult(t, results, "pacific")
	// 日界线两侧各 10x10 的面，各含一个 6x6 的洞。
	assertStats(t, pacific, []float64{64, 64}, []int{1, 1})
	assertLonRange(t, pacific)
	if _, ok := pacific.Geometry().(MultiPolygon); !ok {
		t.Errorf("多分量结果应返回 MultiPolygon，实际 %T", pacific.Geometry())
	}

	concave := findResult(t, results, "concave")
	// H 形整体在视窗内：48+48+8 减去两个 2x2 的洞。
	assertStats(t, concave, []float64{96}, []int{2})
	if _, ok := concave.Geometry().(Polygon); !ok {
		t.Errorf("单分量结果应返回 Polygon，实际 %T", concave.Geometry())
	}
}

// 视窗完全落在洞内时不返回该区域。
func TestViewportInsideHole(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: 173, South: 3, East: 177, North: 7})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("视窗落在洞内应无结果，实际 %d 个区域", len(results))
	}
}

// 仅接触视窗边的区域不返回。
func TestTouchingEdgeOnly(t *testing.T) {
	s := loadTestStore(t)
	results, err := s.Clip(Viewport{West: 0, South: 10, East: 12, North: 20})
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("仅接触视窗边应无结果，实际 %d 个区域", len(results))
	}
}

// 后续查询针对原始几何，重复查询结果一致。
func TestRepeatableQueries(t *testing.T) {
	s := loadTestStore(t)
	v := Viewport{West: -180, South: -90, East: 180, North: 90}
	first, err := s.Clip(v)
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	second, err := s.Clip(v)
	if err != nil {
		t.Fatalf("Clip: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("重复查询结果不一致")
	}
}

func TestInvalidViewport(t *testing.T) {
	s := loadTestStore(t)
	for _, v := range []Viewport{
		{West: 10, South: 0, East: 10, North: 5},   // 东西界相等
		{West: 0, South: 5, East: 10, North: 5},    // 南界不小于北界
		{West: -200, South: 0, East: 10, North: 5}, // 西界越界
	} {
		if _, err := s.Clip(v); err == nil {
			t.Errorf("非法视窗 %+v 应返回错误", v)
		}
	}
}
