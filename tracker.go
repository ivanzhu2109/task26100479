package watchstat

// Event 是上层统一上报的播放事件（play/pause/seek/completion）。
// 时间与位置均为整数毫秒，PosMs 处于视频时长范围内。
type Event struct {
	ViewerID  string // 观众标识
	SessionID string // 会话标识，同一会话始终属于同一观众
	EventID   string // 事件标识，在会话内唯一
	TimeMs    int64  // 事件发生时刻
	PosMs     int64  // 事件执行后的播放位置
	Playing   bool   // 事件执行后是否正在播放
}

// Summary 是视频维度的汇总统计。
type Summary struct {
	Viewers    int     // 已看时长大于 0 的观众数
	AvgRatio   float64 // 这些观众的平均观看比例（以完整视频时长为分母）
	Milestones [4]int  // 覆盖比例达到 25%、50%、75%、100% 的观众数（阈值含等号）
}

// Tracker 是单个固定视频的观看覆盖统计实例。非并发安全，按单线程调用。
// 汇总指标随每次接收增量维护，查询为 O(1)。
type Tracker struct {
	duration   int64
	sessions   map[string]*session
	viewers    map[string]*coverage
	active     int    // 已看时长大于 0 的观众数
	watchedSum int64  // 全部观众已看时长之和
	milestones [4]int // 达到 25%、50%、75%、100% 的观众数
}

// NewTracker 创建统计实例，durationMs 为视频时长（不超过 7200000 毫秒）。
func NewTracker(durationMs int64) *Tracker {
	if durationMs < 0 {
		durationMs = 0
	}
	return &Tracker{
		duration: durationMs,
		sessions: make(map[string]*session),
		viewers:  make(map[string]*coverage),
	}
}

// hit 报告已看时长 total 是否达到第 i 档里程碑（25%、50%、75%、100%）。
// 用整数比较避免浮点边界误差；total 为 0 时不计入。
func (t *Tracker) hit(total int64, i int) bool {
	if total <= 0 {
		return false
	}
	switch i {
	case 0:
		return total*4 >= t.duration
	case 1:
		return total*2 >= t.duration
	case 2:
		return total*4 >= t.duration*3
	default:
		return total >= t.duration
	}
}

// adjust 在某观众已看时长由 old 变为 nw 后更新汇总指标。
func (t *Tracker) adjust(old, nw int64) {
	if old == 0 && nw > 0 {
		t.active++
	} else if old > 0 && nw == 0 {
		t.active--
	}
	t.watchedSum += nw - old
	for i := 0; i < 4; i++ {
		oldHit, newHit := t.hit(old, i), t.hit(nw, i)
		if oldHit != newHit {
			if newHit {
				t.milestones[i]++
			} else {
				t.milestones[i]--
			}
		}
	}
}

// Ingest 接收一条事件；重复投递（会话内事件标识相同）不改变结果。
// 返回后查询即可反映该事件的影响。
func (t *Tracker) Ingest(e Event) {
	s, ok := t.sessions[e.SessionID]
	if !ok {
		s = newSession(e.ViewerID)
		t.sessions[e.SessionID] = s
	}
	cov, ok := t.viewers[s.viewerID]
	if !ok {
		cov = &coverage{rng: uint64(len(t.viewers))*2654435761 + 1}
		t.viewers[s.viewerID] = cov
	}
	old := cov.covered()
	if s.insert(e, cov, t.duration) {
		if nw := cov.covered(); nw != old {
			t.adjust(old, nw)
		}
	}
}

// WatchedMs 返回指定观众的去重后已看时长（毫秒）。
func (t *Tracker) WatchedMs(viewerID string) int64 {
	if cov, ok := t.viewers[viewerID]; ok {
		return cov.covered()
	}
	return 0
}

// Summary 返回视频汇总统计；没有有效观众时各项为 0。
func (t *Tracker) Summary() Summary {
	sum := Summary{Viewers: t.active, Milestones: t.milestones}
	if t.active > 0 && t.duration > 0 {
		sum.AvgRatio = float64(t.watchedSum) / float64(t.duration) / float64(t.active)
	}
	return sum
}
