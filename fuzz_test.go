package watchstat

import (
	"math/rand"
	"sort"
	"testing"
)

// reference 是独立参照实现：从完整事件集重新计算每个观众的已看时长。
// 按会话分组、去重、排序后生成观看区间，再按观众合并去重。
func reference(duration int64, events []Event) map[string]int64 {
	type sessKey struct{ viewer, session string }
	sessions := map[sessKey]map[string]Event{}
	for _, e := range events {
		k := sessKey{e.ViewerID, e.SessionID}
		m, ok := sessions[k]
		if !ok {
			m = map[string]Event{}
			sessions[k] = m
		}
		m[e.EventID] = e // 重复投递内容一致，覆盖即可
	}
	viewerIvs := map[string][][2]int64{}
	for k, m := range sessions {
		evs := make([]Event, 0, len(m))
		for _, e := range m {
			evs = append(evs, e)
		}
		sort.Slice(evs, func(i, j int) bool { return less(evs[i], evs[j]) })
		for i := 0; i+1 < len(evs); i++ {
			if l, r, ok := interval(evs[i], evs[i+1], duration); ok {
				viewerIvs[k.viewer] = append(viewerIvs[k.viewer], [2]int64{l, r})
			}
		}
	}
	watched := map[string]int64{}
	for viewer, ivs := range viewerIvs {
		watched[viewer] = mergeLen(ivs)
	}
	return watched
}

// mergeLen 计算区间列表的覆盖总长度（去重）。
func mergeLen(ivs [][2]int64) int64 {
	sort.Slice(ivs, func(i, j int) bool { return ivs[i][0] < ivs[j][0] })
	var total, curL, curR int64
	started := false
	for _, iv := range ivs {
		if !started || iv[0] > curR {
			if started {
				total += curR - curL
			}
			curL, curR = iv[0], iv[1]
			started = true
		} else if iv[1] > curR {
			curR = iv[1]
		}
	}
	if started {
		total += curR - curL
	}
	return total
}

func refSummary(duration int64, watched map[string]int64) Summary {
	var sum Summary
	var ratioSum float64
	for _, total := range watched {
		if total <= 0 {
			continue
		}
		sum.Viewers++
		ratioSum += float64(total) / float64(duration)
		if total*4 >= duration {
			sum.Milestones[0]++
		}
		if total*2 >= duration {
			sum.Milestones[1]++
		}
		if total*4 >= duration*3 {
			sum.Milestones[2]++
		}
		if total >= duration {
			sum.Milestones[3]++
		}
	}
	if sum.Viewers > 0 {
		sum.AvgRatio = ratioSum / float64(sum.Viewers)
	}
	return sum
}

// TestRandomOutOfOrder 乱序随机事件（含重复投递与迟到事件），
// 每次接收后都与从完整事件集独立重算的结果比对。
func TestRandomOutOfOrder(t *testing.T) {
	const duration = 120_000
	rng := rand.New(rand.NewSource(7))

	for trial := 0; trial < 30; trial++ {
		// 为每个会话预生成一条事件时间线，再打乱投递顺序。
		var all []Event
		nViewers := 1 + rng.Intn(4)
		for v := 0; v < nViewers; v++ {
			viewer := string(rune('A' + v))
			nSessions := 1 + rng.Intn(3)
			for s := 0; s < nSessions; s++ {
				session := string(rune('a'+s)) + viewer
				nEvents := 1 + rng.Intn(12)
				t := int64(0)
				for i := 0; i < nEvents; i++ {
					t += int64(rng.Intn(15_000))
					all = append(all, Event{
						ViewerID:  viewer,
						SessionID: session,
						EventID:   session + "-" + string(rune('0'+i)),
						TimeMs:    t,
						PosMs:     int64(rng.Intn(int(duration) + 1)),
						Playing:   rng.Intn(3) > 0,
					})
				}
			}
		}
		// 打乱顺序，并随机插入重复投递。
		var stream []Event
		for _, e := range all {
			stream = append(stream, e)
			if rng.Intn(4) == 0 {
				stream = append(stream, e) // 重复投递
			}
		}
		rng.Shuffle(len(stream), func(i, j int) { stream[i], stream[j] = stream[j], stream[i] })

		tr := NewTracker(duration)
		var received []Event
		for i, e := range stream {
			tr.Ingest(e)
			received = append(received, e)
			want := reference(duration, received)
			for viewer, w := range want {
				if got := tr.WatchedMs(viewer); got != w {
					t.Fatalf("trial %d step %d: viewer %s watched=%d want %d", trial, i, viewer, got, w)
				}
			}
			wantSum := refSummary(duration, want)
			gotSum := tr.Summary()
			if gotSum.Viewers != wantSum.Viewers || gotSum.Milestones != wantSum.Milestones {
				t.Fatalf("trial %d step %d: summary=%+v want %+v", trial, i, gotSum, wantSum)
			}
			if diff := gotSum.AvgRatio - wantSum.AvgRatio; diff > 1e-12 || diff < -1e-12 {
				t.Fatalf("trial %d step %d: avgRatio=%f want %f", trial, i, gotSum.AvgRatio, wantSum.AvgRatio)
			}
		}
	}
}
