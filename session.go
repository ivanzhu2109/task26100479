package watchstat

// session 保存单个会话已接收的事件，按 (TimeMs, EventID) 升序排列。
type session struct {
	viewerID string
	events   []Event
	seen     map[string]struct{}
}

func newSession(viewerID string) *session {
	return &session{viewerID: viewerID, seen: make(map[string]struct{})}
}

// less 按发生时刻升序、同一时刻按事件标识字典序排列。
func less(a, b Event) bool {
	if a.TimeMs != b.TimeMs {
		return a.TimeMs < b.TimeMs
	}
	return a.EventID < b.EventID
}

// interval 计算相邻事件对（prev, next）贡献的观看区间。
// 仅当 prev 执行后为播放中时贡献 [prev.PosMs, prev.PosMs+length)，
// length 取两项时间差与视频剩余时长的较小值；无贡献时 ok=false。
func interval(prev, next Event, duration int64) (l, r int64, ok bool) {
	if !prev.Playing {
		return 0, 0, false
	}
	length := next.TimeMs - prev.TimeMs
	if remain := duration - prev.PosMs; length > remain {
		length = remain
	}
	if length <= 0 {
		return 0, 0, false
	}
	return prev.PosMs, prev.PosMs + length, true
}

// insert 向会话中插入事件；重复投递（事件标识已存在）返回 false。
// 否则局部更新观众覆盖集合：撤销被拆开的相邻事件对贡献，加入新的事件对贡献。
func (s *session) insert(e Event, cov *coverage, duration int64) bool {
	if _, dup := s.seen[e.EventID]; dup {
		return false
	}
	s.seen[e.EventID] = struct{}{}

	// 二分查找插入位置。
	lo, hi := 0, len(s.events)
	for lo < hi {
		mid := (lo + hi) / 2
		if less(s.events[mid], e) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	idx := lo
	s.events = append(s.events, Event{})
	copy(s.events[idx+1:], s.events[idx:])
	s.events[idx] = e

	var pred, succ *Event
	if idx > 0 {
		pred = &s.events[idx-1]
	}
	if idx+1 < len(s.events) {
		succ = &s.events[idx+1]
	}
	// 撤销原先 (pred, succ) 事件对的贡献。
	if pred != nil && succ != nil {
		if l, r, ok := interval(*pred, *succ, duration); ok {
			cov.add(l, r, -1)
		}
	}
	// 加入 (pred, e) 与 (e, succ) 事件对的贡献。
	if pred != nil {
		if l, r, ok := interval(*pred, e, duration); ok {
			cov.add(l, r, +1)
		}
	}
	if succ != nil {
		if l, r, ok := interval(e, *succ, duration); ok {
			cov.add(l, r, +1)
		}
	}
	return true
}
