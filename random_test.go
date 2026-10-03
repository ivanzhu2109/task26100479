package watchstats

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// bruteForce 从完整事件集独立重算：按会话分组排序求区间，再对每个观众求并集。
func bruteForce(durMs int64, events []Event) (map[string]int64, Summary) {
	type sessKey struct{ viewer, session string }
	sessions := map[sessKey]map[string]Event{}
	for _, ev := range events {
		k := sessKey{ev.ViewerID, ev.SessionID}
		m, ok := sessions[k]
		if !ok {
			m = map[string]Event{}
			sessions[k] = m
		}
		if _, dup := m[ev.EventID]; dup {
			continue // 重复投递，内容一致，忽略
		}
		m[ev.EventID] = ev
	}
	intervals := map[string][][2]int64{}
	for k, m := range sessions {
		evs := make([]Event, 0, len(m))
		for _, ev := range m {
			evs = append(evs, ev)
		}
		sort.Slice(evs, func(i, j int) bool { return less(evs[i], evs[j]) })
		for i := 0; i+1 < len(evs); i++ {
			if s, e, ok := interval(evs[i], evs[i+1], durMs); ok {
				intervals[k.viewer] = append(intervals[k.viewer], [2]int64{s, e})
			}
		}
	}
	watched := map[string]int64{}
	sum := Summary{}
	var sumRatios float64
	for viewer, ivals := range intervals {
		sort.Slice(ivals, func(i, j int) bool { return ivals[i][0] < ivals[j][0] })
		var total, curS, curE int64
		started := false
		for _, iv := range ivals {
			if !started || iv[0] > curE {
				if started {
					total += curE - curS
				}
				curS, curE = iv[0], iv[1]
				started = true
			} else if iv[1] > curE {
				curE = iv[1]
			}
		}
		if started {
			total += curE - curS
		}
		watched[viewer] = total
		if total > 0 {
			sum.Viewers++
			sumRatios += float64(total) / float64(durMs)
			ths := []int64{25, 50, 75, 100}
			for k, th := range ths {
				if total*100 >= th*durMs {
					switch k {
					case 0:
						sum.Milestone25++
					case 1:
						sum.Milestone50++
					case 2:
						sum.Milestone75++
					case 3:
						sum.Milestone100++
					}
				}
			}
		}
	}
	if sum.Viewers > 0 {
		sum.AvgRatio = sumRatios / float64(sum.Viewers)
	}
	return watched, sum
}

func almostEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

// 乱序随机事件测试：每次接收后，结果与从完整事件独立重算一致。
func TestRandomOutOfOrderMatchesRecompute(t *testing.T) {
	const durMs = 300000
	rng := rand.New(rand.NewSource(20261003))
	tr := mustTracker(t, durMs)
	var all []Event
	viewers := []string{"u1", "u2", "u3", "u4", "u5"}
	for i := 0; i < 2000; i++ {
		ev := Event{
			ViewerID:  viewers[rng.Intn(len(viewers))],
			SessionID: string(rune('A' + rng.Intn(6))),
			EventID:   fmt.Sprintf("e%04d", i), // 会话内唯一
			TimeMs:    int64(rng.Intn(durMs)),
			PosMs:     int64(rng.Intn(durMs + 1)),
			Playing:   rng.Intn(2) == 0,
		}
		// 10% 概率重复投递一条已发送的事件（内容一致）。
		if len(all) > 0 && rng.Intn(10) == 0 {
			ev = all[rng.Intn(len(all))]
		}
		tr.Ingest(ev)
		all = append(all, ev)

		wantWatched, wantSum := bruteForce(durMs, all)
		for _, v := range viewers {
			if got := tr.WatchedMs(v); got != wantWatched[v] {
				t.Fatalf("event %d (%+v): watched[%s] = %d, want %d", i, ev, v, got, wantWatched[v])
			}
		}
		gotSum := tr.Summary()
		if gotSum.Viewers != wantSum.Viewers ||
			gotSum.Milestone25 != wantSum.Milestone25 ||
			gotSum.Milestone50 != wantSum.Milestone50 ||
			gotSum.Milestone75 != wantSum.Milestone75 ||
			gotSum.Milestone100 != wantSum.Milestone100 ||
			!almostEq(gotSum.AvgRatio, wantSum.AvgRatio) {
			t.Fatalf("event %d: summary = %+v, want %+v", i, gotSum, wantSum)
		}
	}
}
