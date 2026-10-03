package maplens

import "math"

// unwrapRing 将环展开到连续经度区间：从首点出发，
// 相邻点经度差取绝对值小于 180 的唯一走向（输入保证无恰好 180 的边）。
// 输入可为闭合环（首点==末点），输出保持相同的点数与闭合性。
func unwrapRing(r Ring) Ring {
	if len(r) == 0 {
		return r
	}
	out := make(Ring, len(r))
	out[0] = r[0]
	for i := 1; i < len(r); i++ {
		d := r[i].X - r[i-1].X
		// 将差值折到 [-180, 180)：等价于 ±180 同一条经线的唯一短走向。
		d = math.Mod(d+540, 360) - 180
		out[i] = Point{X: out[i-1].X + d, Y: r[i].Y}
	}
	return out
}

// alignRing 将环整体平移 360 的整数倍，使其首点落在 ref 的 ±180 范围内。
// 用于把洞环对齐到外环的展开坐标系。
func alignRing(r Ring, ref Point) Ring {
	if len(r) == 0 {
		return r
	}
	k := math.Round((ref.X - r[0].X) / 360)
	if k == 0 {
		return r
	}
	out := make(Ring, len(r))
	for i, p := range r {
		out[i] = Point{X: p.X + 360*k, Y: p.Y}
	}
	return out
}

// openRing 去掉闭合环末尾与首点重复的点，转为内部的不闭合存储。
func openRing(r Ring) Ring {
	n := len(r)
	for n > 1 && nearEq(r[n-1], r[0]) {
		n--
	}
	return r[:n]
}

// reverseRing 返回反向的环。
func reverseRing(r Ring) Ring {
	out := make(Ring, len(r))
	for i, p := range r {
		out[len(r)-1-i] = p
	}
	return out
}
