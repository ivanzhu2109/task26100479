package watchstats

import "sort"

// session 保存单个会话的事件时间线，按 (TimeMs, EventID) 升序排列。
type session struct {
	events []Event
	ids    map[string]struct{}
}

func newSession() *session {
	return &session{ids: make(map[string]struct{})}
}

// has 判断事件标识是否已存在（用于重复投递去重）。
func (s *session) has(eventID string) bool {
	_, ok := s.ids[eventID]
	return ok
}

// less 定义时间线顺序：先按发生时刻，同一时刻按事件标识字典序。
func less(a, b Event) bool {
	if a.TimeMs != b.TimeMs {
		return a.TimeMs < b.TimeMs
	}
	return a.EventID < b.EventID
}

// interval 计算事件 a 相对其后继 b 贡献的观看区间：
// 仅当 a 执行后为播放中，取 [a.PosMs, a.PosMs+min(时间差, 视频剩余))，起点计入、终点不计入。
func interval(a, b Event, durMs int64) (start, end int64, ok bool) {
	if !a.Playing {
		return 0, 0, false
	}
	l := b.TimeMs - a.TimeMs
	if rem := durMs - a.PosMs; rem < l {
		l = rem
	}
	if l <= 0 {
		return 0, 0, false
	}
	return a.PosMs, a.PosMs + l, true
}

// insert 将事件插入时间线，并通过 cov 增量维护覆盖：
// 先撤回被拆开的相邻区间，再添加新产生的两个相邻区间。
func (s *session) insert(ev Event, durMs int64, cov *coverage) {
	p := sort.Search(len(s.events), func(i int) bool { return !less(s.events[i], ev) })
	// 撤回前驱与后继之间原有的相邻区间。
	if p > 0 && p < len(s.events) {
		if st, en, ok := interval(s.events[p-1], s.events[p], durMs); ok {
			cov.add(st, en, -1)
		}
	}
	s.events = append(s.events, Event{})
	copy(s.events[p+1:], s.events[p:])
	s.events[p] = ev
	s.ids[ev.EventID] = struct{}{}
	// 添加 前驱->新事件 与 新事件->后继 两个相邻区间。
	if p > 0 {
		if st, en, ok := interval(s.events[p-1], ev, durMs); ok {
			cov.add(st, en, +1)
		}
	}
	if p+1 < len(s.events) {
		if st, en, ok := interval(ev, s.events[p+1], durMs); ok {
			cov.add(st, en, +1)
		}
	}
}
