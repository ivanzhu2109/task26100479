package watchstat

// inode 是覆盖区间集合（treap）中的节点，表示一个互不重叠的区间
// [start, end) 及其引用计数 cnt（被多少条观看区间覆盖）。
// 不变式：cnt >= 1；相邻区间的 cnt 不同（相同者已合并）。
type inode struct {
	start, end  int64
	cnt         int
	pri         uint32
	left, right *inode
}

// coverage 维护单个观众的观看区间多重集合，支持区间增减与已覆盖总时长查询。
type coverage struct {
	root  *inode
	total int64
	rng   uint64
}

// covered 返回当前已覆盖的总时长（毫秒）。
func (c *coverage) covered() int64 {
	return c.total
}

func (c *coverage) nextPri() uint32 {
	// xorshift64*
	c.rng ^= c.rng << 13
	c.rng ^= c.rng >> 7
	c.rng ^= c.rng << 17
	return uint32(c.rng>>11) | 1
}

// split 按 start 键将树分为 < key 与 >= key 两棵。
func split(n *inode, key int64) (l, r *inode) {
	if n == nil {
		return nil, nil
	}
	if n.start < key {
		rl, rr := split(n.right, key)
		n.right = rl
		return n, rr
	}
	ll, lr := split(n.left, key)
	n.left = lr
	return ll, n
}

// merge 合并两棵树，要求 l 中所有键小于 r 中所有键。
func merge(l, r *inode) *inode {
	if l == nil {
		return r
	}
	if r == nil {
		return l
	}
	if l.pri < r.pri {
		l.right = merge(l.right, r)
		return l
	}
	r.left = merge(l, r.left)
	return r
}

// findContaining 返回满足 start <= p < end 的节点，不存在时返回 nil。
func findContaining(n *inode, p int64) *inode {
	for n != nil {
		switch {
		case p < n.start:
			n = n.left
		case p >= n.end:
			n = n.right
		default:
			return n
		}
	}
	return nil
}

// deleteKey 删除 start == key 的节点（必须存在）。
func deleteKey(n *inode, key int64) *inode {
	if n.start == key {
		return merge(n.left, n.right)
	}
	if key < n.start {
		n.left = deleteKey(n.left, key)
	} else {
		n.right = deleteKey(n.right, key)
	}
	return n
}

func insertNode(root, n *inode) *inode {
	a, b := split(root, n.start)
	return merge(merge(a, n), b)
}

// ensureBoundary 保证 p 是某个节点的边界（或落在空白处）：
// 若 p 严格落在某节点内部，则把该节点一分为二。
func (c *coverage) ensureBoundary(p int64) {
	n := findContaining(c.root, p)
	if n == nil || n.start == p {
		return
	}
	c.root = deleteKey(c.root, n.start)
	c.root = insertNode(c.root, &inode{start: n.start, end: p, cnt: n.cnt, pri: c.nextPri()})
	c.root = insertNode(c.root, &inode{start: p, end: n.end, cnt: n.cnt, pri: c.nextPri()})
}

// flatten 中序展开为有序节点切片。
func flatten(n *inode, out []*inode) []*inode {
	if n == nil {
		return out
	}
	out = flatten(n.left, out)
	out = append(out, n)
	return flatten(n.right, out)
}

// rightmost 返回树中键最大的节点。
func rightmost(n *inode) *inode {
	for n != nil && n.right != nil {
		n = n.right
	}
	return n
}

// leftmost 返回树中键最小的节点。
func leftmost(n *inode) *inode {
	for n != nil && n.left != nil {
		n = n.left
	}
	return n
}

// buildFromSorted 由键递增的节点序列重建 treap。
func buildFromSorted(nodes []*inode) *inode {
	var root *inode
	for _, n := range nodes {
		n.left, n.right = nil, nil
		root = merge(root, n)
	}
	return root
}

// add 将区间 [l, r) 的引用计数调整 delta（+1 或 -1），并维护 total。
func (c *coverage) add(l, r int64, delta int) {
	if l >= r {
		return
	}
	c.ensureBoundary(l)
	c.ensureBoundary(r)

	a, bc := split(c.root, l)
	b, cc := split(bc, r)

	nodes := flatten(b, nil)
	// 补齐空白段，使节点序列完整覆盖 [l, r)。
	full := make([]*inode, 0, len(nodes)+1)
	pos := l
	for _, n := range nodes {
		if pos < n.start {
			full = append(full, &inode{start: pos, end: n.start, cnt: 0, pri: c.nextPri()})
		}
		full = append(full, n)
		pos = n.end
	}
	if pos < r {
		full = append(full, &inode{start: pos, end: r, cnt: 0, pri: c.nextPri()})
	}
	kept := make([]*inode, 0, len(full)+1)
	for _, n := range full {
		if n.cnt > 0 {
			c.total -= n.end - n.start
		}
		n.cnt += delta
		if n.cnt > 0 {
			c.total += n.end - n.start
			kept = append(kept, n)
		}
	}

	// 与左侧相邻节点合并（cnt 相同）。
	if la := rightmost(a); la != nil && len(kept) > 0 && la.end == kept[0].start && la.cnt == kept[0].cnt {
		a = deleteKey(a, la.start)
		kept[0].start = la.start
	}
	// 与右侧相邻节点合并（cnt 相同）。
	if len(kept) > 0 {
		if rc := leftmost(cc); rc != nil && kept[len(kept)-1].end == rc.start && rc.cnt == kept[len(kept)-1].cnt {
			cc = deleteKey(cc, rc.start)
			kept[len(kept)-1].end = rc.end
		}
	}
	// kept 内部相邻同计数节点合并。
	merged := kept[:0]
	for _, n := range kept {
		if len(merged) > 0 && merged[len(merged)-1].end == n.start && merged[len(merged)-1].cnt == n.cnt {
			merged[len(merged)-1].end = n.end
			continue
		}
		merged = append(merged, n)
	}

	c.root = merge(a, merge(buildFromSorted(merged), cc))
}
