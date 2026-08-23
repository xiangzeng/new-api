package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Allowed reports whether key is still under its limit without recording an
// event. Limiters that only meter failures need to check the verdict before
// the handler runs and record long after it, which Request cannot express
// because it always consumes budget.
func (l *InMemoryRateLimiter) Allowed(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	queue, ok := l.store[key]
	if !ok || len(*queue) < maxRequestNum {
		return true
	}
	return time.Now().Unix()-(*queue)[0] >= duration
}

// Record appends one event to key's window, dropping entries that already fell
// out of it so an idle key cannot stay full forever. Parameter duration's unit
// is seconds.
func (l *InMemoryRateLimiter) Record(key string, maxRequestNum int, duration int64) {
	if maxRequestNum <= 0 {
		return
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now().Unix()
	queue, ok := l.store[key]
	if !ok {
		s := make([]int64, 0, maxRequestNum)
		s = append(s, now)
		l.store[key] = &s
		return
	}
	for len(*queue) > 0 && now-(*queue)[0] >= duration {
		*queue = (*queue)[1:]
	}
	if len(*queue) >= maxRequestNum {
		*queue = (*queue)[len(*queue)-maxRequestNum+1:]
	}
	*queue = append(*queue, now)
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}
