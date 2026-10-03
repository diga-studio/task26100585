package maplens

import (
	"fmt"
	"os"
)

// Lens 保存已装载的区域集合，计算状态全部在内存中。
// 每次 Clip 都针对原始几何，互不影响。
type Lens struct {
	features []Feature
}

// LoadFile 从本地 GeoJSON 文件装载区域集合。
// 输入为 FeatureCollection，每个区域需有唯一 id（或 properties.id），
// 几何仅支持 Polygon 与 MultiPolygon。
func LoadFile(path string) (*Lens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("maplens: 读取文件失败: %w", err)
	}
	feats, err := parseFeatureCollection(data)
	if err != nil {
		return nil, err
	}
	return &Lens{features: feats}, nil
}

// Features 返回已装载区域的原始拷贝（未裁剪）。
func (l *Lens) Features() []Feature {
	out := make([]Feature, len(l.features))
	copy(out, l.features)
	return out
}

// Clip 按视窗裁剪，返回各区域与视窗的几何交集（仅正面积部分）。
// west < east 为普通视窗；west > east 为跨日界线视窗（如 170, -170 表示宽 20 度）；
// west=-180, east=180 覆盖全部经度。south 必须小于 north。
// 结果经度保持在 [-180,180]，跨日界线的面会在该经线处分成两侧。
func (l *Lens) Clip(west, south, east, north float64) []Feature {
	if south >= north || west == east {
		return nil
	}
	vp := newViewport(west, south, east, north)
	var out []Feature
	for _, f := range l.features {
		if r := clipFeature(f, vp); r != nil {
			out = append(out, *r)
		}
	}
	return out
}
