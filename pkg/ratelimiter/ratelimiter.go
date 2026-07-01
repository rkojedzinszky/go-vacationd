package ratelimiter

import (
	"context"
	"sync"
	"time"

	"github.com/rkojedzinszky/go-vacationd/pkg/interfaces"
)

// New returns a new ratelimiter, possibly delaying to pass the specified duration
func New(d time.Duration) interfaces.RateLimiter {
	return &ratelimiter{
		d:     d,
		inch:  make(chan *elemkey),
		track: make(map[elemkey]bool),
	}
}

type elemkey struct {
	from string
	to   string
}

type expireelem struct {
	when time.Time
	key  *elemkey
	next *expireelem
}

type ratelimiter struct {
	d time.Duration

	inch chan *elemkey

	lock       sync.Mutex
	track      map[elemkey]bool
	expirehead *expireelem
	expiretail *expireelem
}

func (r *ratelimiter) Ratelimit(from, to string) bool {
	key := elemkey{
		from: from,
		to:   to,
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.track[key] {
		return false
	}

	r.inch <- &key

	return true
}

func (r *ratelimiter) Run(ctx context.Context) {
	tm := time.NewTimer(0)

	for {
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
		}
		if r.expirehead != nil {
			tm.Reset(time.Until(r.expirehead.when))
		}
	}
}

func (r *ratelimiter) add(key *elemkey) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.track[*key] {
		return
	}

	r.track[*key] = true

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

	delete(r.track, *e.key)
}
