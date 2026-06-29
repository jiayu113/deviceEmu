package sip

import (
	"testing"
	"time"
)

// 测试碰壁退避时间
func TestBackoffDelay(t *testing.T) {
	base, max := time.Second, 30*time.Second
	cases := []struct {
		attempt int
		lo, hi  time.Duration
	}{
		{1, 800 * time.Millisecond, 1200 * time.Millisecond},
		{2, 1600 * time.Millisecond, 2400 * time.Millisecond},
		{100, 24 * time.Second, 36 * time.Second}, // 封顶 30s ±20%
	}
	for _, c := range cases {
		for i := 0; i < 300; i++ {
			d := backoffDelay(c.attempt, base, max)
			if d < c.lo || d > c.hi {
				t.Fatalf("attempt=%d got %s want [%s,%s]", c.attempt, d, c.lo, c.hi)
			}
		}
	}
}

// 测试提前续签时间
func TestRenewInterval(t *testing.T) {
	if got := renewInterval(60); got != 45*time.Second {
		t.Fatalf("renew(60)=%s want 45s", got)
	}
	if got := renewInterval(0); got != 45*time.Second {
		t.Fatalf("renew(0)=%s want 45s", got)
	}
	if got := renewInterval(8); got != 3*time.Second { // margin=max(2,5)=5 → 8-5=3
		t.Fatalf("renew(8)=%s want 3s", got)
	}
}
