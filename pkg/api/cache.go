package api

import (
	"context"
	"errors"
	"time"
)

var (
	errCacheDisabled = errors.New("cache disabled")
	errCacheStopped  = errors.New("cache stopped")
	errNoLoader      = errors.New("no loader")
)

// cacheRequest models a single cache lookup or population attempt.
// Using a struct keeps the channel signature compact so the goroutine
// that owns the cache can reason about a single message type.
type cacheRequest struct {
	ctx    context.Context
	key    string
	loader func(context.Context) ([]byte, error)
	reply  chan cacheResponse
}

// cacheResponse carries either the cached bytes or an error back to
// the handler goroutine.
type cacheResponse struct {
	data []byte
	err  error
}

// cacheEntry records cached JSON along with its expiry timestamp.
// The goroutine trims stale entries lazily when they are accessed so we
// avoid extra timers.
type cacheEntry struct {
	data    []byte
	expires time.Time
}

// ResponseCache keeps expensive API responses in memory so identical
// requests within the TTL avoid hitting the database. We implement it
// with a dedicated goroutine and channels to honour the constraint of
// coordinating state without mutexes.
type ResponseCache struct {
	ttl      time.Duration
	requests chan cacheRequest
	quit     chan struct{}
	now      func() time.Time
}

// NewResponseCache starts the caching goroutine immediately. Callers
// may pass nil to disable caching entirely. The clock is injectable for
// tests; in production we default to time.Now.
func NewResponseCache(ttl time.Duration) *ResponseCache {
	if ttl <= 0 {
		return nil
	}
	cache := &ResponseCache{
		ttl:      ttl,
		requests: make(chan cacheRequest),
		quit:     make(chan struct{}),
		now:      time.Now,
	}
	go cache.loop()
	return cache
}

// Close stops the cache goroutine. The method is safe to call multiple
// times; subsequent calls have no effect.
func (c *ResponseCache) Close() {
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

// Get returns cached bytes for the provided key or invokes loader to
// produce them. We copy the stored slice before returning so callers
// can safely modify the result without affecting future hits.
func (c *ResponseCache) Get(ctx context.Context, key string, loader func(context.Context) ([]byte, error)) ([]byte, error) {
	if c == nil {
		return nil, errCacheDisabled
	}
	req := cacheRequest{
		ctx:    ctx,
		key:    key,
		loader: loader,
		reply:  make(chan cacheResponse, 1),
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.quit:
		return nil, errCacheStopped
	case c.requests <- req:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.quit:
		return nil, errCacheStopped
	case resp := <-req.reply:
		if resp.err != nil {
			return nil, resp.err
		}
		if resp.data == nil {
			return nil, nil
		}
		copyBuf := make([]byte, len(resp.data))
		copy(copyBuf, resp.data)
		return copyBuf, nil
	}
}

// loop serialises all cache access inside a single goroutine so we can
// use plain maps without additional locking constructs.
func (c *ResponseCache) loop() {
	store := make(map[string]cacheEntry)
	for {
		select {
		case <-c.quit:
			return
		case req := <-c.requests:
			now := c.now()
			if entry, ok := store[req.key]; ok && now.Before(entry.expires) {
				req.reply <- cacheResponse{data: entry.data}
				continue
			}
			if req.loader == nil {
				req.reply <- cacheResponse{err: errNoLoader}
				continue
			}
			data, err := req.loader(req.ctx)
			if err == nil && data != nil {
				buf := make([]byte, len(data))
				copy(buf, data)
				store[req.key] = cacheEntry{data: buf, expires: now.Add(c.ttl)}
			} else if err != nil {
				delete(store, req.key)
			}
			req.reply <- cacheResponse{data: data, err: err}
		}
	}
}
