package watchstat

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// TestPerformance 规模测试：累计 20 万事件、1 万名观众（其中一名占 10 万条），
// 随后 1 万条插入既有时间线中间的事件，每条之后查询该观众与视频汇总。
// 该阶段应小于 10 秒，峰值堆内存远低于 512 MiB。
func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in -short mode")
	}
	const duration = 7_200_000 // 7200000 毫秒上限
	tr := NewTracker(duration)
	rng := rand.New(rand.NewSource(1))

	// 重度观众：单会话 10 万条事件，时间线铺满 [0, 6e6) 毫秒。
	const heavyEvents = 100_000
	for i := 0; i < heavyEvents; i++ {
		tr.Ingest(Event{
			ViewerID:  "heavy",
			SessionID: "hs",
			EventID:   fmt.Sprintf("h%06d", i),
			TimeMs:    int64(i) * 60,
			PosMs:     rng.Int63n(duration + 1),
			Playing:   rng.Intn(10) < 7,
		})
	}
	// 其余 9999 名观众，每人约 10 条事件，合计约 10 万条。
	for v := 0; v < 9_999; v++ {
		viewer := fmt.Sprintf("v%05d", v)
		for j := 0; j < 10; j++ {
			tr.Ingest(Event{
				ViewerID:  viewer,
				SessionID: fmt.Sprintf("s%05d", v),
				EventID:   fmt.Sprintf("e%02d", j),
				TimeMs:    int64(j+1) * 1000,
				PosMs:     rng.Int63n(duration + 1),
				Playing:   rng.Intn(10) < 7,
			})
		}
	}

	// 第二阶段：1 万条插入重度观众既有时间线中间的事件，
	// 每条之后查询该观众与视频汇总。
	start := time.Now()
	for i := 0; i < 10_000; i++ {
		tr.Ingest(Event{
			ViewerID:  "heavy",
			SessionID: "hs",
			EventID:   fmt.Sprintf("x%06d", i),
			TimeMs:    1 + rng.Int63n(6_000_000),
			PosMs:     rng.Int63n(duration + 1),
			Playing:   rng.Intn(10) < 7,
		})
		_ = tr.WatchedMs("heavy")
		_ = tr.Summary()
	}
	elapsed := time.Since(start)
	t.Logf("10k mid-timeline inserts with queries: %v", elapsed)
	if elapsed > 10*time.Second {
		t.Fatalf("phase 2 took %v, want < 10s", elapsed)
	}

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("HeapAlloc=%.1f MiB HeapInuse=%.1f MiB", float64(ms.HeapAlloc)/1048576, float64(ms.HeapInuse)/1048576)
	if ms.HeapInuse > 512<<20 {
		t.Fatalf("HeapInuse=%d bytes exceeds 512 MiB", ms.HeapInuse)
	}
	// 防止编译器在统计内存前将 tr 判定为死对象。
	runtime.KeepAlive(tr)
	t.Logf("heavy viewer watched=%d ms, summary=%+v", tr.WatchedMs("heavy"), tr.Summary())
}
