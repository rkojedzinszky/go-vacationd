package ratelimiter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/rkojedzinszky/go-vacationd/pkg/interfaces"
)

// New returns a new ratelimiter, possibly delaying to pass the specified duration
func New(d time.Duration) interfaces.RateLimiter {
	return &ratelimiter{
		d:     d,
		inch:  make(chan string),
		track: make(map[string]bool),
	}
}

type expireelem struct {
	when time.Time
	key  string
	next *expireelem
}

type ratelimiter struct {
	d time.Duration

	inch chan string

	lock       sync.Mutex
	track      map[string]bool
	expirehead *expireelem
	expiretail *expireelem
}

func (r *ratelimiter) Ratelimit(from, to string) bool {
	key := mapKey(from, to)

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.track[key] {
		return false
	}

	r.inch <- key

	return true
}

func (r *ratelimiter) Run(ctx context.Context) {
	var tm *time.Timer

	for {
		if tm == nil {
			select {
			case <-ctx.Done():
				return
			case key := <-r.inch:
				r.add(key)
				tm = time.NewTimer(time.Until(r.expirehead.when))
			}
		} else {
			select {
			case <-ctx.Done():
				return
			case key := <-r.inch:
				r.add(key)
			case <-tm.C:
				n := time.Now()
				for e := r.expirehead; e != nil && e.when.Before(n); e = r.expirehead {
					r.pop()
				}
				if r.expirehead != nil {
					tm = time.NewTimer(time.Until(r.expirehead.when))
				} else {
					tm = nil
				}
			}
		}
	}
}

func (r *ratelimiter) add(key string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.track[key] {
		return
	}

	r.track[key] = true

	e := &expireelem{
		when: time.Now().Add(r.d),
		key:  key,
	}

	if r.expiretail != nil {
		r.expiretail.next = e
	} else {
		r.expirehead = e
	}
	r.expiretail = e
}

func (r *ratelimiter) pop() {
	e := r.expirehead

	r.expirehead = e.next
	if r.expirehead == nil {
		r.expiretail = nil
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.track, e.key)
}

func mapKey(from, to string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s@@%s", from, to)))

	return string(sum[:])
}
