package service

import (
	"fmt"
	"sync"
	"time"
)

type limiterCounter struct {
	count int
	start time.Time
}

type Limiter struct {
	mu            sync.Mutex
	buckets       map[string]*limiterCounter
	lastSendAt    map[string]time.Time
	domainLastAt  map[string]time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{
		buckets:      make(map[string]*limiterCounter),
		lastSendAt:   make(map[string]time.Time),
		domainLastAt: make(map[string]time.Time),
	}
}

func (l *Limiter) CheckAndInc(userID int64, domain string, perSec, perMin, perHour, perDay, throttleMS, domainPerHour, domainThrottleMS int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	if throttleMS > 0 {
		if last, ok := l.lastSendAt[fmt.Sprintf("u:%d", userID)]; ok {
			if now.Sub(last) < time.Duration(throttleMS)*time.Millisecond {
				return fmt.Errorf("user throttled: wait %dms", throttleMS)
			}
		}
	}

	if domainThrottleMS > 0 {
		dk := fmt.Sprintf("u:%d:d:%s", userID, domain)
		if last, ok := l.domainLastAt[dk]; ok {
			if now.Sub(last) < time.Duration(domainThrottleMS)*time.Millisecond {
				return fmt.Errorf("domain throttled: wait %dms", domainThrottleMS)
			}
		}
	}

	checks := []struct {
		key   string
		limit int
		win   time.Duration
	}{
		{fmt.Sprintf("u:%d:sec", userID), perSec, time.Second},
		{fmt.Sprintf("u:%d:min", userID), perMin, time.Minute},
		{fmt.Sprintf("u:%d:hour", userID), perHour, time.Hour},
		{fmt.Sprintf("u:%d:day", userID), perDay, 24 * time.Hour},
		{fmt.Sprintf("u:%d:domain:%s:hour", userID, domain), domainPerHour, time.Hour},
	}

	for _, c := range checks {
		if c.limit <= 0 {
			continue
		}
		b := l.buckets[c.key]
		if b == nil || now.Sub(b.start) >= c.win {
			l.buckets[c.key] = &limiterCounter{count: 1, start: now}
			continue
		}
		if b.count+1 > c.limit {
			return fmt.Errorf("rate limit exceeded: %s", c.key)
		}
		b.count++
	}

	l.lastSendAt[fmt.Sprintf("u:%d", userID)] = now
	l.domainLastAt[fmt.Sprintf("u:%d:d:%s", userID, domain)] = now
	return nil
}