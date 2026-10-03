package maplens

// Point 为经纬度平面上的点，Lon 经度、Lat 纬度。
type Point struct {
	Lon float64
	Lat float64
}

// Ring 为闭合环，首尾点相同。
type Ring []Point

// Polygon 为带洞多边形：第一个环为外环，其余为洞。
type Polygon []Ring

// MultiPolygon 为多个互不重面的多边形。
type MultiPolygon []Polygon

// Geometry 为 Polygon 或 MultiPolygon。
type Geometry interface {
	isGeometry()
}

func (Polygon) isGeometry()      {}
func (MultiPolygon) isGeometry() {}

// Feature 为一个区域：唯一标识 + 几何。
type Feature struct {
	ID       string
	Geometry Geometry
}
