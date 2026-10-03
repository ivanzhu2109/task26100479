package watchstats

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// 性能测试：累计 20 万条事件、1 万名观众（其中一人 10 万条），
// 再补 1 万条插入既有时间线中间的事件，每条之后查询该观众与视频汇总。
// 要求补入阶段在普通开发机上 10 秒内完成，峰值内存不超过 512 MiB。
func TestPerformanceIngestAndQuery(t *testing.T) {
	const durMs = MaxDurationMs
	tr := mustTracker(t, durMs)

	// 重度观众：单会话 10 万条事件，偶数时刻，位置随时间推进（到片尾后回绕）。
	const heavyEvents = 100000
	for i := 0; i < heavyEvents; i++ {
		ts := int64(2 * i)
		tr.Ingest(Event{
			ViewerID:  "heavy",
			SessionID: "s0",
			EventID:   fmt.Sprintf("h%06d", i),
			TimeMs:    ts,
			PosMs:     ts % durMs,
			Playing:   i%10 != 9, // 90% 播放中
		})
	}
	// 其余 9999 名观众各 10 条事件，合计约 10 万条。
	for u := 0; u < 9999; u++ {
		viewer := fmt.Sprintf("u%05d", u)
		for j := 0; j < 10; j++ {
			ts := int64(j * 1000)
			tr.Ingest(Event{
				ViewerID:  viewer,
				SessionID: "s",
				EventID:   fmt.Sprintf("e%02d", j),
				TimeMs:    ts,
				PosMs:     ts,
				Playing:   j < 9,
			})
		}
	}

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// 补入 1 万条落在重度观众时间线中间的事件（奇数时刻），
	// 每条之后查询该观众与视频汇总。
	const lateEvents = 10000
	start := time.Now()
	var sink int64
	var sumSink Summary
	for i := 0; i < lateEvents; i++ {
		ts := int64(2*(40000+i) + 1) // 80001..99998，位于既有时间线中间
		tr.Ingest(Event{
			ViewerID:  "heavy",
			SessionID: "s0",
			EventID:   fmt.Sprintf("x%06d", i),
			TimeMs:    ts,
			PosMs:     ts % durMs,
			Playing:   i%2 == 0, // 既造成覆盖新增，也造成撤回
		})
		sink += tr.WatchedMs("heavy")
		sumSink = tr.Summary()
	}
	elapsed := time.Since(start)

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	heapMiB := float64(memAfter.HeapAlloc) / (1 << 20)
	t.Logf("late phase: %d ingests + queries in %v (%.1f us/op)", lateEvents, elapsed, float64(elapsed.Microseconds())/lateEvents)
	t.Logf("heap after: %.1f MiB, heavy watched: %d ms, viewers: %d", heapMiB, sink/int64(lateEvents)*0+tr.WatchedMs("heavy"), sumSink.Viewers)
	if elapsed > 10*time.Second {
		t.Fatalf("late phase took %v, exceeds 10s budget", elapsed)
	}
	if heapMiB > 512 {
		t.Fatalf("heap %.1f MiB exceeds 512 MiB budget", heapMiB)
	}
}
