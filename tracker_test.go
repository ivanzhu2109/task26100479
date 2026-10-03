package watchstats

import "testing"

func mustTracker(t *testing.T, durMs int64) *Tracker {
	t.Helper()
	tr, err := NewTracker(durMs)
	if err != nil {
		t.Fatalf("NewTracker(%d): %v", durMs, err)
	}
	return tr
}

// 验收场景：100 秒视频，先播放到头再暂停，覆盖 100 秒；
// 补入 10 秒处跳到 90 秒继续播放的事件后，覆盖降为 20 秒，里程碑归零。
func TestAcceptanceSeekRetractsCoverage(t *testing.T) {
	tr := mustTracker(t, 100000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 100000, PosMs: 100000, Playing: false})
	if got := tr.WatchedMs("v1"); got != 100000 {
		t.Fatalf("after play+pause: watched = %d, want 100000", got)
	}
	sum := tr.Summary()
	if sum.Viewers != 1 || sum.Milestone25 != 1 || sum.Milestone50 != 1 || sum.Milestone75 != 1 || sum.Milestone100 != 1 {
		t.Fatalf("after full watch: summary = %+v", sum)
	}
	if sum.AvgRatio != 1.0 {
		t.Fatalf("after full watch: avgRatio = %v, want 1.0", sum.AvgRatio)
	}

	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "c", TimeMs: 10000, PosMs: 90000, Playing: true})
	if got := tr.WatchedMs("v1"); got != 20000 {
		t.Fatalf("after late seek event: watched = %d, want 20000", got)
	}
	sum = tr.Summary()
	if sum.Viewers != 1 {
		t.Fatalf("viewers = %d, want 1", sum.Viewers)
	}
	if sum.Milestone25 != 0 || sum.Milestone50 != 0 || sum.Milestone75 != 0 || sum.Milestone100 != 0 {
		t.Fatalf("milestones after retraction = %+v, want all zero", sum)
	}
	if want := 0.2; sum.AvgRatio < want-1e-12 || sum.AvgRatio > want+1e-12 {
		t.Fatalf("avgRatio = %v, want 0.2", sum.AvgRatio)
	}
}

// 同一观众多个会话的区间重叠时，按内容覆盖去重。
func TestMultiSessionOverlapDeduplicated(t *testing.T) {
	tr := mustTracker(t, 100000)
	// 会话 s1：0s 从头播放，50s 暂停 -> [0, 50000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 50000, PosMs: 50000, Playing: false})
	// 会话 s2：0s 从 20s 处播放，40s 暂停 -> [20000, 60000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s2", EventID: "a", TimeMs: 0, PosMs: 20000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s2", EventID: "b", TimeMs: 40000, PosMs: 60000, Playing: false})
	// 并集 [0, 60000) = 60s，而不是 50s+40s=90s。
	if got := tr.WatchedMs("v1"); got != 60000 {
		t.Fatalf("watched = %d, want 60000", got)
	}
	// 同一段内容被同一会话重复播放也只计一次：
	// 会话 s3：0s 从 10s 播放，10s 拖回 10s 继续播放，20s 暂停 -> [10000,20000) 两次。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s3", EventID: "a", TimeMs: 0, PosMs: 10000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s3", EventID: "b", TimeMs: 10000, PosMs: 10000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s3", EventID: "c", TimeMs: 20000, PosMs: 20000, Playing: false})
	if got := tr.WatchedMs("v1"); got != 60000 {
		t.Fatalf("after replay session: watched = %d, want 60000", got)
	}
}

// 重复投递同一事件不改变结果。
func TestDuplicateDeliveryIgnored(t *testing.T) {
	tr := mustTracker(t, 100000)
	ev1 := Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true}
	ev2 := Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 30000, PosMs: 30000, Playing: false}
	tr.Ingest(ev1)
	tr.Ingest(ev2)
	tr.Ingest(ev1)
	tr.Ingest(ev2)
	tr.Ingest(ev1)
	if got := tr.WatchedMs("v1"); got != 30000 {
		t.Fatalf("watched = %d, want 30000", got)
	}
	if sum := tr.Summary(); sum.Viewers != 1 {
		t.Fatalf("viewers = %d, want 1", sum.Viewers)
	}
}

// 拖动跳过的内容不计入观看；最后一项事件无后继不贡献区间。
func TestSeekSkipsContentAndTailEventContributesNothing(t *testing.T) {
	tr := mustTracker(t, 100000)
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true})
	// 10s 处拖动到 60s 继续播放。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 10000, PosMs: 60000, Playing: true})
	// 20s 处暂停于 70s。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "c", TimeMs: 20000, PosMs: 70000, Playing: false})
	// 覆盖 [0,10000) + [60000,70000) = 20s；[10000,60000) 被跳过不计。
	if got := tr.WatchedMs("v1"); got != 20000 {
		t.Fatalf("watched = %d, want 20000", got)
	}
}

// 播放区间不超过视频末尾；汇总阈值包含等号；无有效观众时各项为 0。
func TestBoundariesAndMilestones(t *testing.T) {
	tr := mustTracker(t, 100000)
	if sum := tr.Summary(); sum != (Summary{}) {
		t.Fatalf("empty summary = %+v, want zero", sum)
	}
	// 播放中事件的时间差超出视频剩余时长时，按剩余时长截断。
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "a", TimeMs: 0, PosMs: 90000, Playing: true})
	tr.Ingest(Event{ViewerID: "v1", SessionID: "s1", EventID: "b", TimeMs: 50000, PosMs: 100000, Playing: false})
	if got := tr.WatchedMs("v1"); got != 10000 {
		t.Fatalf("watched = %d, want 10000 (clamped to video end)", got)
	}
	// 恰好 25%/50%/75%/100% 的四名观众，阈值包含等号。
	for _, tc := range []struct {
		id    string
		watch int64
	}{{"w25", 25000}, {"w50", 50000}, {"w75", 75000}, {"w100", 100000}} {
		tr.Ingest(Event{ViewerID: tc.id, SessionID: "s", EventID: "a", TimeMs: 0, PosMs: 0, Playing: true})
		tr.Ingest(Event{ViewerID: tc.id, SessionID: "s", EventID: "b", TimeMs: tc.watch, PosMs: tc.watch, Playing: false})
	}
	sum := tr.Summary()
	if sum.Viewers != 5 {
		t.Fatalf("viewers = %d, want 5", sum.Viewers)
	}
	if sum.Milestone25 != 4 || sum.Milestone50 != 3 || sum.Milestone75 != 2 || sum.Milestone100 != 1 {
		t.Fatalf("milestones = %+v, want 4/3/2/1", sum)
	}
	// 平均比例 = (0.1 + 0.25 + 0.5 + 0.75 + 1.0) / 5 = 0.52。
	if want := 0.52; sum.AvgRatio < want-1e-9 || sum.AvgRatio > want+1e-9 {
		t.Fatalf("avgRatio = %v, want %v", sum.AvgRatio, want)
	}
}

// 时长校验。
func TestDurationValidation(t *testing.T) {
	if _, err := NewTracker(0); err == nil {
		t.Fatal("NewTracker(0) should fail")
	}
	if _, err := NewTracker(MaxDurationMs + 1); err == nil {
		t.Fatal("NewTracker(MaxDurationMs+1) should fail")
	}
	if _, err := NewTracker(MaxDurationMs); err != nil {
		t.Fatalf("NewTracker(MaxDurationMs): %v", err)
	}
}
