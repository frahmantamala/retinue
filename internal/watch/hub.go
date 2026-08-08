package watch

import (
	"encoding/json"
	"sync"
)

// subBuffer is generous enough to absorb the initial burst when a long
// transcript is replayed, without letting one stalled browser pin it in memory.
const subBuffer = 512

// Hub fans graph deltas out to connected pages. A subscriber that cannot keep
// up loses messages rather than stalling the tailer — the page reconnects and
// takes a fresh snapshot, so dropping is recoverable and blocking is not.
type Hub struct {
	mu      sync.Mutex
	subs    map[chan []byte]struct{}
	dropped int
}

func NewHub() *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}}
}

func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *Hub) Publish(msgs []any) {
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		h.send(b)
	}
}

func (h *Hub) send(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
			h.dropped++
		}
	}
}

func (h *Hub) Dropped() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dropped
}
