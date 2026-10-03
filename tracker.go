package watchstats

import "errors"

// MaxDurationMs 是视频时长的上限。
const MaxDurationMs int64 = 7200000

// Tracker 是单个视频的观看覆盖统计实例。仅支持单线程调用。
type Tracker struct {
	durMs      int64
	viewers    map[string]*viewer
	viewersPos int     // 已看时长大于 0 的观众数
	sumRatios  float64 // 这些观众的观看比例之和
	milestones [4]int  // 覆盖比例达到 25%/50%/75%/100% 的观众数
}

// NewTracker 创建对应固定时长视频的统计实例。
func NewTracker(durMs int64) (*Tracker, error) {
	if durMs <= 0 || durMs > MaxDurationMs {
		return nil, errors.New("watchstats: duration out of range")
	}
	return &Tracker{durMs: durMs, viewers: make(map[string]*viewer)}, nil
}

// Ingest 接收一条事件；重复投递（会话内事件标识相同）不改变结果。
func (t *Tracker) Ingest(ev Event) {
	v, ok := t.viewers[ev.ViewerID]
	if !ok {
		v = newViewer()
		t.viewers[ev.ViewerID] = v
	}
	s, ok := v.sessions[ev.SessionID]
	if !ok {
		s = newSession()
		v.sessions[ev.SessionID] = s
	}
	if s.has(ev.EventID) {
		return
	}
	old := v.total
	s.insert(ev, t.durMs, &v.cov)
	v.total = v.cov.total
	t.updateSummary(old, v.total)
}

// updateSummary 在某观众已看时长由 old 变为 cur 后，增量维护汇总聚合。
func (t *Tracker) updateSummary(old, cur int64) {
	if old == cur {
		return
	}
	if old > 0 {
		t.viewersPos--
		t.sumRatios -= float64(old) / float64(t.durMs)
	}
	if cur > 0 {
		t.viewersPos++
		t.sumRatios += float64(cur) / float64(t.durMs)
	}
	for k := int64(1); k <= 4; k++ {
		// 阈值包含等号，用整数比较避免浮点误差：old/dur >= k/4。
		was := old*4 >= k*t.durMs
		is := cur*4 >= k*t.durMs
		switch {
		case is && !was:
			t.milestones[k-1]++
		case was && !is:
			t.milestones[k-1]--
		}
	}
}

// WatchedMs 返回指定观众按内容覆盖去重后的已看时长（毫秒）。
func (t *Tracker) WatchedMs(viewerID string) int64 {
	if v, ok := t.viewers[viewerID]; ok {
		return v.total
	}
	return 0
}

// Summary 返回视频维度汇总；没有有效观众时各项为 0。
func (t *Tracker) Summary() Summary {
	s := Summary{
		Viewers:      t.viewersPos,
		Milestone25:  t.milestones[0],
		Milestone50:  t.milestones[1],
		Milestone75:  t.milestones[2],
		Milestone100: t.milestones[3],
	}
	if t.viewersPos > 0 {
		s.AvgRatio = t.sumRatios / float64(t.viewersPos)
	}
	return s
}
