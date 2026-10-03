package watchstats

import "sort"

// coverage 以引用计数维护一组区间的并集，支持区间的添加与撤回，
// 并增量维护被覆盖的总时长。
type coverage struct {
	points []int64 // 有序边界点，len = 段数 + 1
	cnt    []int   // cnt[i] 为段 [points[i], points[i+1]) 的引用计数
	total  int64   // 计数大于 0 的段的总长度
}

// indexOf 返回 x 在边界点中的下标；ok 为 false 时 x 不存在，i 为插入位置。
func (c *coverage) indexOf(x int64) (i int, ok bool) {
	i = sort.Search(len(c.points), func(j int) bool { return c.points[j] >= x })
	return i, i < len(c.points) && c.points[i] == x
}

// ensurePoint 保证 x 是边界点：若不存在则插入，并把被拆开的段的计数复制给两段。
func (c *coverage) ensurePoint(x int64) int {
	i, ok := c.indexOf(x)
	if ok {
		return i
	}
	c.points = append(c.points, 0)
	copy(c.points[i+1:], c.points[i:])
	c.points[i] = x
	// 被拆开的段是 i-1（若存在），其计数复制给新段 i。
	var cntAt int
	if i > 0 {
		cntAt = c.cnt[i-1]
	}
	c.cnt = append(c.cnt, 0)
	copy(c.cnt[i+1:], c.cnt[i:])
	c.cnt[i] = cntAt
	return i
}

// add 将区间 [s, e) 的引用计数调整 delta（+1 添加，-1 撤回）。
func (c *coverage) add(s, e int64, delta int) {
	if s >= e {
		return
	}
	from := c.ensurePoint(s)
	to := c.ensurePoint(e)
	for i := from; i < to; i++ {
		old := c.cnt[i]
		next := old + delta
		if next < 0 {
			panic("watchstats: coverage count underflow")
		}
		segLen := c.points[i+1] - c.points[i]
		if old == 0 && next > 0 {
			c.total += segLen
		} else if old > 0 && next == 0 {
			c.total -= segLen
		}
		c.cnt[i] = next
	}
}
