// Package limit 提供游戏服务端可复用的限流工具。
package limit

import (
	"errors"
	"sync"
	"time"
)

// TokenBucket 是一个并发安全的令牌桶。
// Rate 表示每秒补充的令牌数，Burst 表示最多可以积累的令牌数。
type TokenBucket struct {
	lock       sync.Mutex
	rate       float64
	burst      float64
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucket 创建一个令牌桶。
func NewTokenBucket(rate, burst int) (*TokenBucket, error) {
	if rate <= 0 {
		return nil, errors.New("token bucket rate must be positive")
	}
	if burst <= 0 {
		return nil, errors.New("token bucket burst must be positive")
	}
	now := time.Now()
	return &TokenBucket{
		rate:       float64(rate),
		burst:      float64(burst),
		tokens:     float64(burst),
		lastRefill: now,
	}, nil
}

// Allow 尝试消耗一个令牌。
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN 尝试消耗 n 个令牌。
func (tb *TokenBucket) AllowN(count int) bool {
	if tb == nil || count <= 0 {
		return false
	}

	tb.lock.Lock()
	defer tb.lock.Unlock()
	tb.refill(time.Now())
	if float64(count) > tb.tokens {
		return false
	}
	tb.tokens -= float64(count)
	return true
}

func (tb *TokenBucket) refill(now time.Time) {
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.burst {
		tb.tokens = tb.burst
	}
	tb.lastRefill = now
}
