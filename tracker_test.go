package watchstat

import "testing"

// TestAcceptanceScenario 验收场景：100 秒视频，
// 先接收 0 秒从头播放与 100 秒暂停，覆盖为 100 秒；
// 再补入 10 秒跳到 90 秒并继续播放的事件，覆盖降为 20 秒，里程碑归零。
func TestAcceptanceScenario(t *testing.T) {
	tr := NewTracker(100_000) // 100 秒
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e1", TimeMs: 0, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e2", TimeMs: 100_000, PosMs: 100_000, Playing: false})

	if got := tr.WatchedMs("v1"); got != 100_000 {
		t.Fatalf("after play+pause: watched=%d want 100000", got)
	}
	sum := tr.Summary()
	if sum.Viewers != 1 || sum.Milestones != [4]int{1, 1, 1, 1} {
		t.Fatalf("after play+pause: summary=%+v", sum)
	}
	if sum.AvgRatio != 1.0 {
		t.Fatalf("after play+pause: avgRatio=%f want 1.0", sum.AvgRatio)
	}

	// 迟到事件：10 秒时拖动到 90 秒并继续播放。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e3", TimeMs: 10_000, PosMs: 90_000, Playing: true})

	if got := tr.WatchedMs("v1"); got != 20_000 {
		t.Fatalf("after late seek: watched=%d want 20000", got)
	}
	sum = tr.Summary()
	if sum.Viewers != 1 {
		t.Fatalf("after late seek: viewers=%d want 1", sum.Viewers)
	}
	if sum.Milestones != [4]int{0, 0, 0, 0} {
		t.Fatalf("after late seek: milestones=%v want all zero", sum.Milestones)
	}
	if want := 0.2; sum.AvgRatio != want {
		t.Fatalf("after late seek: avgRatio=%f want %f", sum.AvgRatio, want)
	}
}

// TestMultiSessionOverlap 同一观众多会话重叠覆盖只计一次。
func TestMultiSessionOverlap(t *testing.T) {
	tr := NewTracker(100_000)
	// 会话 s1：0s 起播，60s 暂停 → [0, 60000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 60_000, PosMs: 60_000, Playing: false})
	// 会话 s2：30s 从 30s 起播，90s 暂停 → [30000, 90000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s2", EventID: "a", TimeMs: 30_000, PosMs: 30_000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s2", EventID: "b", TimeMs: 90_000, PosMs: 90_000, Playing: false})

	if got := tr.WatchedMs("v1"); got != 90_000 {
		t.Fatalf("watched=%d want 90000", got)
	}
}

// TestDuplicateDelivery 重复投递不改变结果。
func TestDuplicateDelivery(t *testing.T) {
	tr := NewTracker(100_000)
	e1 := Event{ViewerID: "v1", SessionID: "s1", EventID: "e1", TimeMs: 0, PosMs: 0, Playing: true}
	e2 := Event{ViewerID: "v1", SessionID: "s1", EventID: "e2", TimeMs: 50_000, PosMs: 50_000, Playing: false}
	tr.Ingest(e1)
	tr.Ingest(e2)
	before := tr.WatchedMs("v1")
	for i := 0; i < 5; i++ {
		tr.Ingest(e1)
		tr.Ingest(e2)
	}
	if got := tr.WatchedMs("v1"); got != before || got != 50_000 {
		t.Fatalf("watched=%d want 50000 (before=%d)", got, before)
	}
	if sum := tr.Summary(); sum.Viewers != 1 {
		t.Fatalf("viewers=%d want 1", sum.Viewers)
	}
}

// TestSeekSkipsContent 拖动跳过的内容不计入观看。
func TestSeekSkipsContent(t *testing.T) {
	tr := NewTracker(100_000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e1", TimeMs: 0, PosMs: 0, Playing: true})
	// 10s 时拖动到 80s 并继续播放。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e2", TimeMs: 10_000, PosMs: 80_000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e3", TimeMs: 30_000, PosMs: 100_000, Playing: false})
	// 观看区间：[0,10000) 与 [80000,100000)，共 30s。
	if got := tr.WatchedMs("v1"); got != 30_000 {
		t.Fatalf("watched=%d want 30000", got)
	}
}

// TestRepeatedPlaybackCountsOnce 同一段内容重复播放只计一次。
func TestRepeatedPlaybackCountsOnce(t *testing.T) {
	tr := NewTracker(100_000)
	// 第一次看 [0, 50s)。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e1", TimeMs: 0, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e2", TimeMs: 50_000, PosMs: 50_000, Playing: false})
	// 拖回开头重看 [0, 50s)。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e3", TimeMs: 60_000, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e4", TimeMs: 110_000, PosMs: 50_000, Playing: false})
	if got := tr.WatchedMs("v1"); got != 50_000 {
		t.Fatalf("watched=%d want 50000", got)
	}
}

// TestTrailingEventNoContribution 最后一项事件无后继，不贡献区间。
func TestTrailingEventNoContribution(t *testing.T) {
	tr := NewTracker(100_000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "e1", TimeMs: 0, PosMs: 0, Playing: true})
	if got := tr.WatchedMs("v1"); got != 0 {
		t.Fatalf("watched=%d want 0", got)
	}
	if sum := tr.Summary(); sum.Viewers != 0 || sum.AvgRatio != 0 || sum.Milestones != [4]int{} {
		t.Fatalf("summary=%+v want all zero", sum)
	}
}

// TestMilestoneThresholds 阈值含等号。
func TestMilestoneThresholds(t *testing.T) {
	tr := NewTracker(100_000)
	addViewer := func(id string, watchedMs int64) {
		tr.Ingest(Event{ViewerID: id, SessionID: "s-" + id, EventID: "p", TimeMs: 0, PosMs: 0, Playing: true})
		tr.Ingest(Event{ViewerID: id, SessionID: "s-" + id, EventID: "q", TimeMs: watchedMs, PosMs: watchedMs, Playing: false})
	}
	addViewer("v25", 25_000)
	addViewer("v50", 50_000)
	addViewer("v75", 75_000)
	addViewer("v100", 100_000)
	addViewer("v10", 10_000)
	sum := tr.Summary()
	if sum.Viewers != 5 {
		t.Fatalf("viewers=%d want 5", sum.Viewers)
	}
	if sum.Milestones != [4]int{4, 3, 2, 1} {
		t.Fatalf("milestones=%v want [4 3 2 1]", sum.Milestones)
	}
	wantAvg := (0.25 + 0.5 + 0.75 + 1.0 + 0.1) / 5
	if diff := sum.AvgRatio - wantAvg; diff > 1e-12 || diff < -1e-12 {
		t.Fatalf("avgRatio=%f want %f", sum.AvgRatio, wantAvg)
	}
}

// TestOutOfOrderSessions 不同会话接收顺序不影响结果。
func TestOutOfOrderSessions(t *testing.T) {
	build := func(order []Event) int64 {
		tr := NewTracker(100_000)
		for _, e := range order {
			tr.Ingest(e)
		}
		return tr.WatchedMs("v1")
	}
	e1 := Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true}
	e2 := Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 40_000, PosMs: 40_000, Playing: false}
	e3 := Event{ViewerID: "v1", SessionID: "s2", EventID: "a", TimeMs: 0, PosMs: 60_000, Playing: true}
	e4 := Event{ViewerID: "v1", SessionID: "s2", EventID: "b", TimeMs: 30_000, PosMs: 90_000, Playing: false}
	want := int64(70_000) // [0,40000) ∪ [60000,90000)
	if got := build([]Event{e1, e2, e3, e4}); got != want {
		t.Fatalf("forward order: watched=%d want %d", got, want)
	}
	if got := build([]Event{e4, e3, e2, e1}); got != want {
		t.Fatalf("reverse order: watched=%d want %d", got, want)
	}
	if got := build([]Event{e3, e1, e4, e2}); got != want {
		t.Fatalf("mixed order: watched=%d want %d", got, want)
	}
}
