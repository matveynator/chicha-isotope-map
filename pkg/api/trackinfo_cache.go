package api

import (
	"context"
	"errors"
	"time"

	"chicha-isotope-map/pkg/database"
)

// ======================
// Track info cache logic
// ======================

// errTrackInfoStopped reports that the cache goroutine has exited. Callers fall back to direct queries if this happens.
var errTrackInfoStopped = errors.New("track info cache stopped")

// trackInfoRequest models a single lookup. We send requests through a dedicated channel so the loop remains in charge of all state.
type trackInfoRequest struct {
	ctx   context.Context
	reply chan trackInfoResponse
}

// trackInfoResponse contains either cached data or an error explaining why it is unavailable.
type trackInfoResponse struct {
	totalTracks int64
	latestID    string
	err         error
}

// trackInfoSnapshot represents the most recently computed numbers. We keep the timestamp to decide when background refreshes are due.
type trackInfoSnapshot struct {
	totalTracks int64
	latestID    string
	fetchedAt   time.Time
	ready       bool
}

// refreshOutcome is sent back from the loader goroutine. A nil error means the snapshot now contains fresh data.
type refreshOutcome struct {
	snapshot trackInfoSnapshot
	err      error
}

// TrackInfoCache shields handlers from heavy COUNT(DISTINCT) queries by caching the result and refreshing it in the background.
// Following the proverb "Don't communicate by sharing memory; share memory by communicating", we coordinate exclusively via channels.
type TrackInfoCache struct {
	db      *database.Database
	dbType  string
	ttl     time.Duration
	timeout time.Duration
	retry   time.Duration
	logf    func(string, ...any)
	now     func() time.Time

	requests chan trackInfoRequest
	quit     chan struct{}
}

// NewTrackInfoCache starts the cache goroutine. ttl controls how long results stay fresh, timeout bounds database calls and retry
// decides how quickly we attempt to recover after failures. Passing nil db disables the cache so handlers can fall back to direct queries.
func NewTrackInfoCache(db *database.Database, dbType string, ttl, timeout, retry time.Duration, logf func(string, ...any)) *TrackInfoCache {
	if db == nil || db.DB == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if retry <= 0 {
		retry = time.Minute
	}
	cache := &TrackInfoCache{
		db:       db,
		dbType:   dbType,
		ttl:      ttl,
		timeout:  timeout,
		retry:    retry,
		logf:     logf,
		now:      time.Now,
		requests: make(chan trackInfoRequest),
		quit:     make(chan struct{}),
	}
	go cache.loop()
	return cache
}

// Close stops the goroutine. The method is idempotent so shutdown paths stay simple.
func (c *TrackInfoCache) Close() {
	if c == nil {
		return
	}
	select {
	case <-c.quit:
		return
	default:
	}
	close(c.quit)
}

// Get returns the cached track count and latest ID. The call blocks until the first snapshot is ready or the context is cancelled.
func (c *TrackInfoCache) Get(ctx context.Context) (int64, string, error) {
	if c == nil {
		return 0, "", errors.New("track info cache disabled")
	}
	req := trackInfoRequest{ctx: ctx, reply: make(chan trackInfoResponse, 1)}
	select {
	case <-ctx.Done():
		return 0, "", ctx.Err()
	case <-c.quit:
		return 0, "", errTrackInfoStopped
	case c.requests <- req:
	}
	select {
	case <-ctx.Done():
		return 0, "", ctx.Err()
	case <-c.quit:
		return 0, "", errTrackInfoStopped
	case resp := <-req.reply:
		return resp.totalTracks, resp.latestID, resp.err
	}
}

// loop serialises all cache access. A single goroutine owns the state so we avoid mutexes entirely.
func (c *TrackInfoCache) loop() {
	if c == nil {
		return
	}
	snapshot := trackInfoSnapshot{}
	var refreshCh <-chan refreshOutcome
	refreshing := false
	var timer *time.Timer

	triggerRefresh := func() {
		if refreshing {
			return
		}
		refreshing = true
		refreshCh = c.startRefresh(snapshot)
	}

	armTimer := func(d time.Duration) {
		if d <= 0 {
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer = nil
			}
			return
		}
		if timer == nil {
			timer = time.NewTimer(d)
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)
	}

	triggerRefresh()

	for {
		var timerC <-chan time.Time
		if timer != nil {
			timerC = timer.C
		}
		select {
		case <-c.quit:
			if timer != nil {
				timer.Stop()
			}
			return
		case req := <-c.requests:
			if snapshot.ready {
				// Serve cached data immediately and kick off a background refresh when it expired.
				if c.ttl > 0 && c.now().Sub(snapshot.fetchedAt) >= c.ttl {
					triggerRefresh()
				}
				req.reply <- trackInfoResponse{totalTracks: snapshot.totalTracks, latestID: snapshot.latestID}
				continue
			}
			if !refreshing {
				triggerRefresh()
			}
			delivered := false
			for !delivered {
				select {
				case <-c.quit:
					req.reply <- trackInfoResponse{err: errTrackInfoStopped}
					delivered = true
				case <-req.ctx.Done():
					req.reply <- trackInfoResponse{err: req.ctx.Err()}
					delivered = true
				case outcome := <-refreshCh:
					refreshing = false
					refreshCh = nil
					if outcome.err != nil {
						if c.logf != nil {
							c.logf("track info refresh failed: %v", outcome.err)
						}
						if !outcome.snapshot.ready {
							armTimer(c.retry)
							req.reply <- trackInfoResponse{err: outcome.err}
							delivered = true
							break
						}
					}
					snapshot = outcome.snapshot
					if snapshot.ready {
						req.reply <- trackInfoResponse{totalTracks: snapshot.totalTracks, latestID: snapshot.latestID}
						armTimer(c.ttl)
					} else {
						req.reply <- trackInfoResponse{err: errors.New("track info unavailable")}
						armTimer(c.retry)
					}
					delivered = true
				}
			}
		case outcome := <-refreshCh:
			refreshing = false
			refreshCh = nil
			if outcome.err != nil {
				if c.logf != nil {
					c.logf("track info refresh failed: %v", outcome.err)
				}
				if !outcome.snapshot.ready {
					armTimer(c.retry)
					continue
				}
			}
			snapshot = outcome.snapshot
			if snapshot.ready {
				armTimer(c.ttl)
			} else {
				armTimer(c.retry)
			}
		case <-timerC:
			timer = nil
			triggerRefresh()
		}
	}
}

// startRefresh spawns a goroutine that computes the latest totals. We reuse the previous snapshot when the loader fails
// so callers can keep seeing stale-but-useful data.
func (c *TrackInfoCache) startRefresh(prev trackInfoSnapshot) <-chan refreshOutcome {
	ch := make(chan refreshOutcome, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()
		total, latest, err := c.loadTrackInfo(ctx)
		if err != nil {
			ch <- refreshOutcome{snapshot: prev, err: err}
			return
		}
		ch <- refreshOutcome{snapshot: trackInfoSnapshot{
			totalTracks: total,
			latestID:    latest,
			fetchedAt:   c.now(),
			ready:       true,
		}}
	}()
	return ch
}

// loadTrackInfo calls into the database package. Keeping the logic isolated makes it easy to extend with alternative strategies later.
func (c *TrackInfoCache) loadTrackInfo(ctx context.Context) (int64, string, error) {
	if c.db == nil {
		return 0, "", errors.New("database unavailable")
	}
	total, err := c.db.CountTracks(ctx)
	if err != nil {
		return 0, "", err
	}
	if total == 0 {
		return 0, "", nil
	}
	latest, err := c.db.GetTrackIDByIndex(ctx, total, c.dbType)
	if err != nil {
		return 0, "", err
	}
	return total, latest, nil
}
