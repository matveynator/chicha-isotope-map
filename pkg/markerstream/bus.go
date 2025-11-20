package markerstream

import (
	"context"

	"chicha-isotope-map/pkg/database"
)

// Bus fan-outs new markers to subscribed listeners without locks.
// Using channels keeps producers and consumers decoupled so long imports do not block map streaming.
type Bus struct {
	publish     chan database.Marker
	subscribe   chan subscription
	unsubscribe chan subscription
}

type subscription struct {
	zoom int
	ch   chan database.Marker
}

// NewBus constructs a broadcaster dedicated to marker fan-out.
// The goroutine never stops because it is tied to the process lifetime and relies on caller contexts to prune subscribers.
func NewBus(buffer int) *Bus {
	b := &Bus{
		publish:     make(chan database.Marker, buffer),
		subscribe:   make(chan subscription),
		unsubscribe: make(chan subscription),
	}

	go b.run()
	return b
}

// Publish forwards a marker to listeners interested in the same zoom.
// Non-blocking sends avoid stalling imports when clients are slow or absent.
func (b *Bus) Publish(m database.Marker) {
	select {
	case b.publish <- m:
	default:
	}
}

// Subscribe registers interest in markers for the requested zoom level.
// The returned channel closes when the provided context ends.
func (b *Bus) Subscribe(ctx context.Context, zoom int, buffer int) <-chan database.Marker {
	ch := make(chan database.Marker, buffer)
	req := subscription{zoom: zoom, ch: ch}

	b.subscribe <- req

	go func() {
		<-ctx.Done()
		b.unsubscribe <- req
		close(ch)
	}()

	return ch
}

func (b *Bus) run() {
	listeners := make(map[int][]chan database.Marker)

	for {
		select {
		case req := <-b.subscribe:
			listeners[req.zoom] = append(listeners[req.zoom], req.ch)
		case req := <-b.unsubscribe:
			chans := listeners[req.zoom]
			filtered := chans[:0]
			for _, existing := range chans {
				if existing != req.ch {
					filtered = append(filtered, existing)
				}
			}
			if len(filtered) == 0 {
				delete(listeners, req.zoom)
			} else {
				listeners[req.zoom] = filtered
			}
		case m := <-b.publish:
			subs := listeners[m.Zoom]
			for _, ch := range subs {
				select {
				case ch <- m:
				default:
				}
			}
		}
	}
}
