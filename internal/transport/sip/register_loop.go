package sip

import (
	"context"
	"log"
	"math/rand"
	"time"
)

// RegisterLoop 维持注册在线,直到 ctx 取消:
//   - 首注册失败:指数退避 + 抖动后重试,不让单个设备拖垮进程
//   - 注册成功:在 Expires 过期前自动续订(re-REGISTER)
//   - 续订失败:回到退避重试
//
// 大规模并发时,抖动把重试/续订时刻打散,避免所有设备同一秒重连形成惊群。
func (c *Client) RegisterLoop(ctx context.Context) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 30 * time.Second
	)
	attempt := 0
	for {
		if err := c.Register(ctx); err != nil {
			c.registered.Store(false)
			attempt++
			d := backoffDelay(attempt, baseDelay, maxDelay)
			log.Printf("[sip] %s register failed (attempt %d): %v; retry in %s",
				c.cfg.Username, attempt, err, d.Round(time.Millisecond))
			if !sleepCtx(ctx, d) {
				return
			}
			continue
		}
		c.registered.Store(true)
		attempt = 0
		renew := renewInterval(c.cfg.Expiry)
		log.Printf("[sip] %s registered; renew in %s", c.cfg.Username, renew)
		if !sleepCtx(ctx, renew) {
			return
		}
	}
}

// Registered 供上层(Device 遥测)读取当前注册态。
func (c *Client) Registered() bool { return c.registered.Load() }

// backoffDelay 指数退避(封顶)+ ±20% 抖动。attempt 从 1 开始。
func backoffDelay(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	span := int64(d) / 5 // 20%
	if span <= 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(2*span+1)-span)
}

// renewInterval 续订时机:留余量,赶在 Expires 之前。Expires=60 → 45s。
func renewInterval(expirySec int) time.Duration {
	if expirySec <= 0 {
		expirySec = 60
	}
	margin := expirySec / 4
	if margin < 5 {
		margin = 5
	}
	renew := expirySec - margin
	if renew < 1 {
		renew = expirySec
	}
	return time.Duration(renew) * time.Second
}

// sleepCtx 睡 d,或 ctx 取消提前返回;返回 false 表示被取消。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
