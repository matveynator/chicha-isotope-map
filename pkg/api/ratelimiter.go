package api

import (
	"context"
	"time"
)

// ==========================
// Per-IP rate limiting logic
// ==========================

// RequestKind distinguishes between lightweight metadata calls and heavy
// responses that stream larger payloads. This keeps the limiter expressive while
// staying simple to reason about.
type RequestKind int

const (
	// RequestGeneral marks inexpensive metadata lookups that still benefit
	// from the per-IP queue so clients cannot overwhelm the server with
	// concurrent requests.
	RequestGeneral RequestKind = iota
	// RequestHeavy marks endpoints that stream large responses. We enforce a
	// cooldown after each heavy call to prevent repeated downloads from a
	// single IP.
	RequestHeavy
)

// RateLimiter coordinates per-IP request sequencing without relying on mutexes.
// Each IP gets its own goroutine so the design follows "Do not communicate by
// sharing memory; share memory by communicating".
type RateLimiter struct {
	heavyCooldown time.Duration
	heavyBurst    int // allow a burst of heavy requests before throttling so normal browsing stays fast
	requests      chan keyedRequest
	now           func() time.Time
}

// completionNotice flows back to the per-IP worker once a handler finishes its
// work. We keep it lean to avoid extra allocations while still carrying the
// signal the worker needs to update its rolling heavy request history.
type completionNotice struct {
	kind        RequestKind
	completedAt time.Time
}

type keyedRequest struct {
	ip  string
	req ipRequest
}

type ipRequest struct {
	ctx      context.Context
	kind     RequestKind
	arrived  time.Time
	response chan acquireResponse
}

type acquireResponse struct {
	release      chan struct{}
	wait         bool
	waitDuration time.Duration
	err          error
}

// Permit represents an acquired slot for a particular request. Call Release
// when the handler finished processing so the next queued request can proceed.
type Permit struct {
	release      chan struct{}
	WaitNotice   bool
	WaitDuration time.Duration
}

// Release signals the associated limiter goroutine that the request is done.
// We set the channel to nil so double releases are harmless, following the Go
// proverb "A little copying is better than a little dependency".
func (p *Permit) Release() {
	if p == nil || p.release == nil {
		return
	}
	close(p.release)
	p.release = nil
}

// NewRateLimiter constructs a limiter with the provided cooldown for heavy
// endpoints. The limiter immediately starts its coordination goroutine so the
// caller can use it without additional plumbing.
func NewRateLimiter(heavyCooldown time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		heavyCooldown: heavyCooldown,
		heavyBurst:    8,
		requests:      make(chan keyedRequest),
		now:           time.Now,
	}

	go limiter.loop()

	return limiter
}

// Acquire reserves a slot for the given IP and request kind. The returned
// Permit must be released once the handler is done. If the context is cancelled
// before the permit becomes available an error is returned.
func (l *RateLimiter) Acquire(ctx context.Context, ip string, kind RequestKind) (*Permit, error) {
	if l == nil {
		return nil, nil
	}

	respCh := make(chan acquireResponse, 1)
	req := ipRequest{
		ctx:      ctx,
		kind:     kind,
		arrived:  l.now(),
		response: respCh,
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.requests <- keyedRequest{ip: ip, req: req}:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.err != nil {
			return nil, resp.err
		}
		permit := &Permit{
			release:      resp.release,
			WaitNotice:   resp.wait,
			WaitDuration: resp.waitDuration,
		}
		return permit, nil
	}
}

func (l *RateLimiter) loop() {
	workers := make(map[string]chan ipRequest)

	for keyed := range l.requests {
		ch, ok := workers[keyed.ip]
		if !ok {
			ch = make(chan ipRequest)
			workers[keyed.ip] = ch
			go l.runIPWorker(keyed.ip, ch)
		}

		select {
		case ch <- keyed.req:
		case <-keyed.req.ctx.Done():
			keyed.req.response <- acquireResponse{err: keyed.req.ctx.Err()}
		}
	}
}

func (l *RateLimiter) runIPWorker(ip string, requests <-chan ipRequest) {
	// We keep heavyHistory small by aggressively trimming timestamps from
	// completed requests. activeHeavy tracks in-flight downloads so we can
	// allow short bursts without waiting for earlier downloads to finish.
	var heavyHistory []time.Time
	activeHeavy := 0

	// doneCh collects completion notices from handlers. Using a dedicated
	// channel lets the worker keep processing new arrivals instead of
	// blocking on long downloads, aligning with Go's preference to share
	// memory by communicating.
	doneCh := make(chan completionNotice)

	for {
		select {
		case req, ok := <-requests:
			if !ok {
				return
			}

			select {
			case <-req.ctx.Done():
				req.response <- acquireResponse{err: req.ctx.Err()}
				continue
			default:
			}

			now := l.now()
			queueWait := now.Sub(req.arrived)
			if queueWait < 0 {
				queueWait = 0
			}
			totalWait := queueWait

			if req.kind == RequestHeavy && l.heavyCooldown > 0 && l.heavyBurst > 0 {
				cutoff := now.Add(-l.heavyCooldown)
				dst := heavyHistory[:0]
				for _, ts := range heavyHistory {
					if ts.After(cutoff) {
						dst = append(dst, ts)
					}
				}
				heavyHistory = dst

				// Count both in-flight heavy downloads and recently
				// completed ones so only sustained floods trigger
				// throttling. This keeps normal browsing snappy
				// while still dampening abusive bursts.
				heavySeen := activeHeavy + len(heavyHistory)
				if heavySeen >= l.heavyBurst {
					readyAt := heavyHistory[0].Add(l.heavyCooldown)
					now = l.now()
					if now.Before(readyAt) {
						cooldownWait := readyAt.Sub(now)
						timer := time.NewTimer(cooldownWait)
						select {
						case <-req.ctx.Done():
							if !timer.Stop() {
								<-timer.C
							}
							req.response <- acquireResponse{err: req.ctx.Err()}
							continue
						case <-timer.C:
							totalWait += cooldownWait
						}
					}
				}
			}

			release := make(chan struct{})
			resp := acquireResponse{
				release:      release,
				wait:         totalWait > 0,
				waitDuration: totalWait,
			}

			select {
			case <-req.ctx.Done():
				req.response <- acquireResponse{err: req.ctx.Err()}
				continue
			case req.response <- resp:
			}

			if req.kind == RequestHeavy {
				activeHeavy++
			}

			go l.waitForRelease(req, release, doneCh)

		case done := <-doneCh:
			if done.kind == RequestHeavy {
				if activeHeavy > 0 {
					activeHeavy--
				}
				heavyHistory = append(heavyHistory, done.completedAt)
				// Trim stale timestamps promptly so a single burst
				// cannot penalize a client forever. Storing the
				// timestamps lets us detect real overloads instead
				// of throttling casual browsing where the browser
				// issues only a handful of downloads.
				cutoff := l.now().Add(-l.heavyCooldown)
				dst := heavyHistory[:0]
				for _, ts := range heavyHistory {
					if ts.After(cutoff) {
						dst = append(dst, ts)
					}
				}
				heavyHistory = dst
			}
		}
	}
}

// waitForRelease listens for handler completion signals and notifies the per-IP
// worker once the request finishes. Keeping this separate keeps the hot path in
// runIPWorker non-blocking even when handlers stream large payloads.
func (l *RateLimiter) waitForRelease(req ipRequest, release <-chan struct{}, done chan<- completionNotice) {
	select {
	case <-release:
	case <-req.ctx.Done():
		// Ensure the worker still sees the release event even when the
		// request context is cancelled. This mirrors the original
		// behaviour where the worker always drained the release channel.
		<-release
	}

	done <- completionNotice{kind: req.kind, completedAt: l.now()}
}
