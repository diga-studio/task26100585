package maplens

import (
	"encoding/json"
	"fmt"
)

type gjCollection struct {
	Type     string      `json:"type"`
	Features []gjFeature `json:"features"`
}

type gjFeature struct {
	ID         json.RawMessage `json:"id"`
	Properties map[string]any  `json:"properties"`
	Geometry   gjGeometry      `json:"geometry"`
}

type gjGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// parseFeatureCollection 解析 GeoJSON FeatureCollection。
// 每个区域取 id 字段（或 properties.id）作为唯一标识，几何仅支持 Polygon/MultiPolygon。
func parseFeatureCollection(data []byte) ([]Feature, error) {
	var col gjCollection
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, fmt.Errorf("maplens: 解析 GeoJSON 失败: %w", err)
	}
	if col.Type != "FeatureCollection" {
		return nil, fmt.Errorf("maplens: 期望 FeatureCollection，得到 %q", col.Type)
	}
	feats := make([]Feature, 0, len(col.Features))
	for i := range col.Features {
		gf := &col.Features[i]
		id, err := featureID(gf)
		if err != nil {
			return nil, fmt.Errorf("maplens: 第 %d 个要素: %w", i, err)
		}
		geom, err := parseGeometry(&gf.Geometry)
		if err != nil {
			return nil, fmt.Errorf("maplens: 要素 %q: %w", id, err)
		}
		feats = append(feats, Feature{ID: id, Geometry: geom})
	}
	return feats, nil
}

func featureID(gf *gjFeature) (string, error) {
	if len(gf.ID) > 0 && string(gf.ID) != "null" {
		var s string
		if err := json.Unmarshal(gf.ID, &s); err == nil {
			return s, nil
		}
		var n json.Number
		if err := json.Unmarshal(gf.ID, &n); err == nil {
			return n.String(), nil
		}
		return "", fmt.Errorf("无法解析 id")
	}
	if v, ok := gf.Properties["id"]; ok {
		switch t := v.(type) {
		case string:
			return t, nil
		case float64:
			return json.Number(fmt.Sprintf("%v", t)).String(), nil
		}
	}
	return "", fmt.Errorf("缺少唯一标识 id")
}

func parseGeometry(g *gjGeometry) (Geometry, error) {
	switch g.Type {
	case "Polygon":
		var raw [][][2]float64
		if err := json.Unmarshal(g.Coordinates, &raw); err != nil {
			return nil, fmt.Errorf("解析 Polygon 坐标失败: %w", err)
		}
		p := make(Polygon, len(raw))
		for i, r := range raw {
			p[i] = toRing(r)
		}
		return p, nil
	case "MultiPolygon":
		var raw [][][][2]float64
		if err := json.Unmarshal(g.Coordinates, &raw); err != nil {
			return nil, fmt.Errorf("解析 MultiPolygon 坐标失败: %w", err)
		}
		mp := make(MultiPolygon, len(raw))
		for i, p := range raw {
			poly := make(Polygon, len(p))
			for j, r := range p {
				poly[j] = toRing(r)
			}
			mp[i] = poly
		}
		return mp, nil
	default:
		return nil, fmt.Errorf("不支持的几何类型 %q", g.Type)
	}
}

func toRing(raw [][2]float64) Ring {
	r := make(Ring, len(raw))
	for i, pt := range raw {
		r[i] = Point{Lon: pt[0], Lat: pt[1]}
	}
	return r
}
