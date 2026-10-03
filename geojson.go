package maplens

import (
	"encoding/json"
	"fmt"
	"os"
)

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

type feature struct {
	Type       string          `json:"type"`
	ID         any             `json:"id"`
	Properties map[string]any  `json:"properties"`
	Geometry   geojsonGeometry `json:"geometry"`
}

type geojsonGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// LoadFile 从本地 GeoJSON 文件装载区域集合，返回内存中的 Store。
// 输入须为 FeatureCollection，几何仅含 Polygon 或 MultiPolygon，
// 每个要素须含唯一标识（feature.id，或 properties 中的 id / name）。
// 装载时把每个面展开到宽度小于 180 的连续经度区间，后续查询直接用展开坐标。
func LoadFile(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("maplens: 读取文件失败: %w", err)
	}
	var fc featureCollection
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("maplens: 解析 GeoJSON 失败: %w", err)
	}
	if fc.Type != "FeatureCollection" {
		return nil, fmt.Errorf("maplens: 顶层类型须为 FeatureCollection，实际为 %q", fc.Type)
	}
	store := &Store{}
	seen := map[string]bool{}
	for i := range fc.Features {
		id, err := featureID(&fc.Features[i])
		if err != nil {
			return nil, fmt.Errorf("maplens: 第 %d 个要素: %w", i, err)
		}
		if seen[id] {
			return nil, fmt.Errorf("maplens: 区域标识 %q 重复", id)
		}
		seen[id] = true
		polys, err := parseGeometry(fc.Features[i].Geometry)
		if err != nil {
			return nil, fmt.Errorf("maplens: 区域 %q: %w", id, err)
		}
		store.regions = append(store.regions, storedRegion{id: id, polys: polys})
	}
	return store, nil
}

func featureID(f *feature) (string, error) {
	if f.ID != nil {
		return idString(f.ID)
	}
	for _, key := range []string{"id", "name"} {
		if v, ok := f.Properties[key]; ok && v != nil {
			return idString(v)
		}
	}
	return "", fmt.Errorf("缺少区域标识（feature.id 或 properties.id / properties.name）")
}

func idString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "", fmt.Errorf("区域标识为空字符串")
		}
		return t, nil
	case float64:
		return fmt.Sprintf("%v", t), nil
	default:
		return "", fmt.Errorf("不支持的标识类型 %T", v)
	}
}

func parseGeometry(g geojsonGeometry) ([]Polygon, error) {
	switch g.Type {
	case "Polygon":
		var rings [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &rings); err != nil {
			return nil, fmt.Errorf("解析 Polygon 坐标失败: %w", err)
		}
		p, err := buildPolygon(rings)
		if err != nil {
			return nil, err
		}
		return []Polygon{p}, nil
	case "MultiPolygon":
		var polys [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &polys); err != nil {
			return nil, fmt.Errorf("解析 MultiPolygon 坐标失败: %w", err)
		}
		out := make([]Polygon, 0, len(polys))
		for _, rings := range polys {
			p, err := buildPolygon(rings)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("仅支持 Polygon / MultiPolygon，实际为 %q", g.Type)
	}
}

// buildPolygon 把 GeoJSON 环数组转为内部面：校验坐标范围，
// 外环与洞分别展开，并把洞对齐到外环的展开坐标系。
func buildPolygon(rings [][][2]float64) (Polygon, error) {
	if len(rings) == 0 {
		return Polygon{}, fmt.Errorf("Polygon 至少需要一个外环")
	}
	converted := make([]Ring, len(rings))
	for i, rr := range rings {
		if len(rr) < 4 {
			return Polygon{}, fmt.Errorf("环至少需要 4 个点（闭合），实际 %d 个", len(rr))
		}
		ring := make(Ring, len(rr))
		for j, c := range rr {
			if c[0] < -180 || c[0] > 180 || c[1] < -85 || c[1] > 85 {
				return Polygon{}, fmt.Errorf("坐标越界: (%v, %v)", c[0], c[1])
			}
			ring[j] = Point{X: c[0], Y: c[1]}
		}
		converted[i] = unwrapRing(ring)
	}
	outer := openRing(converted[0])
	if outer.SignedArea() < 0 {
		outer = reverseRing(outer)
	}
	p := Polygon{Outer: outer}
	for _, h := range converted[1:] {
		hole := openRing(alignRing(h, outer[0]))
		if hole.SignedArea() > 0 {
			hole = reverseRing(hole)
		}
		p.Holes = append(p.Holes, hole)
	}
	return p, nil
}
