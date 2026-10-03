package watchstat

import (
	"math/rand"
	"testing"
)

// TestCoverageAgainstBitmap 用逐毫秒位图作为参照，随机增删区间验证一致性。
func TestCoverageAgainstBitmap(t *testing.T) {
	const span = 500
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 50; trial++ {
		cov := &coverage{rng: uint64(trial) + 1}
		bitmap := make([]int, span)
		var live []struct{ l, r int }
		for step := 0; step < 2000; step++ {
			if len(live) > 0 && rng.Intn(2) == 0 {
				i := rng.Intn(len(live))
				iv := live[i]
				live = append(live[:i], live[i+1:]...)
				cov.add(int64(iv.l), int64(iv.r), -1)
				for p := iv.l; p < iv.r; p++ {
					bitmap[p]--
				}
			} else {
				l := int64(rng.Intn(span))
				r := l + int64(rng.Intn(span-int(l))) + 1
				live = append(live, struct{ l, r int }{int(l), int(r)})
				cov.add(l, r, +1)
				for p := l; p < r; p++ {
					bitmap[p]++
				}
			}
			var want int64
			for _, v := range bitmap {
				if v > 0 {
					want++
				}
			}
			if got := cov.covered(); got != want {
				t.Fatalf("trial %d step %d: covered=%d want %d", trial, step, got, want)
			}
		}
	}
}
