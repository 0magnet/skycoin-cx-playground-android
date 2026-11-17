package main

import (
	"image"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// host is the cxrun.Host of the running program. The UI goroutine feeds it
// events and the canvas size and draws the frames it receives.
type host struct {
	invalidate func()

	size   atomic.Value // image.Point
	closed atomic.Bool

	mu       sync.Mutex
	events   []cxrun.Event
	frame    []cxrun.Cmd
	framed   bool
	keyboard *bool
	shown    chan struct{}
}

func newHost(invalidate func()) *host {
	h := &host{invalidate: invalidate, shown: make(chan struct{}, 1)}
	h.size.Store(image.Point{})
	return h
}

func (h *host) Size() (int, int) {
	p := h.size.Load().(image.Point)
	return p.X, p.Y
}

func (h *host) setSize(p image.Point) {
	if old := h.size.Swap(p).(image.Point); old != p && old != (image.Point{}) {
		h.push(cxrun.Event{Kind: cxrun.EvResize, X: float32(p.X), Y: float32(p.Y)})
	}
}

// Frame hands the display list to the UI and waits until it was drawn, so
// the program runs at the display rate. The timeout keeps a program going
// while the window is not drawing.
func (h *host) Frame(cmds []cxrun.Cmd) {
	h.mu.Lock()
	h.frame, h.framed = cmds, true
	h.mu.Unlock()
	select {
	case <-h.shown:
	default:
	}
	h.invalidate()
	select {
	case <-h.shown:
	case <-time.After(250 * time.Millisecond):
	}
}

// takeFrame returns the latest display list, and whether the program has
// drawn at all.
func (h *host) takeFrame() ([]cxrun.Cmd, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.frame, h.framed
}

func (h *host) frameShown() {
	select {
	case h.shown <- struct{}{}:
	default:
	}
}

func (h *host) push(e cxrun.Event) {
	h.mu.Lock()
	h.events = append(h.events, e)
	h.mu.Unlock()
}

func (h *host) PollEvent() (cxrun.Event, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) == 0 {
		return cxrun.Event{}, false
	}
	e := h.events[0]
	h.events = h.events[1:]
	return e, true
}

func (h *host) Keyboard(show bool) {
	h.mu.Lock()
	h.keyboard = &show
	h.mu.Unlock()
	h.invalidate()
}

// takeKeyboard returns a pending keyboard request.
func (h *host) takeKeyboard() (show, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.keyboard == nil {
		return false, false
	}
	show = *h.keyboard
	h.keyboard = nil
	return show, true
}

func (h *host) ShouldClose() bool { return h.closed.Load() }

// console collects program output for display.
type console struct {
	invalidate func()
	mu         sync.Mutex
	b          strings.Builder
}

func (c *console) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.b.Write(p)
	c.mu.Unlock()
	c.invalidate()
	return len(p), nil
}

func (c *console) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}
