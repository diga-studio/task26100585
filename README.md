# MapLens 地图视窗裁剪库

MapLens 是一个纯 Go 库：从本地 GeoJSON 文件装载行政区域边界，
按经纬度视窗计算几何交集，结果直接返回调用方用于绘制。
计算状态保存在内存，无外部服务依赖。

## 能力

- `LoadFile(path)`：装载 GeoJSON FeatureCollection，返回内存态 `Store`。
  - 每个要素为一个区域，须含唯一标识（`feature.id`，或 `properties` 中的 `id` / `name`）。
  - 几何仅支持 `Polygon` / `MultiPolygon`；环的顺逆时针方向与起点任意，输入环须闭合。
  - 装载时把每个面展开到宽度小于 180° 的连续经度区间，跨日界线的几何无需特殊处理。
- `store.Clip(viewport)`：返回各区域与视窗的几何交集 `[]Result`。
  - 只保留有正面积的部分；仅接触视窗边/角、或视窗完全落在洞内时不返回。
  - 同一区域的全部分量归在一个 `Result` 中，保留原区域标识。
  - 查询始终针对原始几何，可反复调用，互不影响。

## 视窗约定

```go
type Viewport struct{ West, South, East, North float64 }
```

- 南界须严格小于北界；不支持东西界相等的视窗。
- `West < East`：普通视窗；`West > East`：跨日界线视窗（如西 170、东 -170 表示宽 20°）。
- `West = -180, East = 180`：覆盖全部经度。

## 输出约定

- `Result.Components` 为该区域交集的各连通分量（`[]Polygon`，每个 `Polygon` 含外环与洞环），
  保留真实的连通关系和洞：裁剪后洞被切开时成为外边界的缺口，不再计为洞。
- `Result.Geometry()` 在单分量时返回 `Polygon`，多分量时返回 `MultiPolygon`。
- 输出经度保持在 [-180, 180]；跨日界线的结果在日界线处分成两侧的面，
  不会出现横跨地图的长边；分割产生的公共边界可同时出现在两侧。
- 环按不闭合方式存储（首尾点不重复），外环逆时针、洞环顺时针；
  需要闭合形式（如输出 GeoJSON）时调用 `ring.Closed()`。
- 面积（平方度）：`Polygon.Area()`、`Result.Area()`。

## 使用示例

```go
store, err := maplens.LoadFile("regions.geojson")
if err != nil { log.Fatal(err) }

results, err := store.Clip(maplens.Viewport{West: 175, South: -5, East: -175, North: 15})
if err != nil { log.Fatal(err) }

for _, r := range results {
    fmt.Println(r.ID, "分量数:", len(r.Components), "面积:", r.Area())
    for _, comp := range r.Components {
        draw(comp.Outer.Closed())        // 外环
        for _, hole := range comp.Holes {
            draw(hole.Closed())          // 洞环
        }
    }
}
```

## 实现说明

裁剪在展开经度坐标系中进行：视窗换算为轴对齐矩形（跨线视窗东界加 360），
每个面平移 360° 的整数倍逐一求交。单个面与矩形求交时，
先对各环做 Sutherland-Hodgman 裁剪，再把所有有向边（实体恒在左侧）
在矩形边界上做共线拆分与反向抵消（消除零宽桥接与洞/外环在视窗边上的重叠），
最后在半边图上追踪连通面并归并洞。结果在 ±180+360k 经线处切开并平移回 [-180, 180]。

## 测试

```sh
go test ./...
```

覆盖：凹区域裁成多分量且洞被切开、带洞区域跨日界线（跨线视窗与全经度视窗）、
视窗落在洞内、仅接触视窗边、重复查询一致性、非法视窗校验等场景，
断言区域标识、连通分量数、洞数与面积。
