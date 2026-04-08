package bridge

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type WebEvent struct {
	Event string `json:"event"`
	Args  []any  `json:"args"`
}

type eventHandler func(...any)

type eventBus struct {
	mu       sync.RWMutex
	handlers map[string][]eventHandler
	clients  map[uint64]chan WebEvent
}

var webEventBus = &eventBus{
	handlers: make(map[string][]eventHandler),
	clients:  make(map[uint64]chan WebEvent),
}

var eventClientCounter uint64

func (b *eventBus) On(event string, handler eventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[event] = append(b.handlers[event], handler)
}

func (b *eventBus) Off(event string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.handlers, event)
}

func (b *eventBus) Emit(event string, args ...any) {
	b.emit(event, args, true)
}

func (b *eventBus) EmitLocal(event string, args ...any) {
	b.emit(event, args, false)
}

func (b *eventBus) emit(event string, args []any, broadcast bool) {
	b.mu.RLock()
	handlers := append([]eventHandler(nil), b.handlers[event]...)
	clients := make([]chan WebEvent, 0, len(b.clients))
	if broadcast {
		for _, ch := range b.clients {
			clients = append(clients, ch)
		}
	}
	b.mu.RUnlock()

	for _, handler := range handlers {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("event handler panic on %q: %v", event, recovered)
				}
			}()
			handler(args...)
		}()
	}

	if !broadcast {
		return
	}

	webEvent := WebEvent{
		Event: event,
		Args:  append([]any(nil), args...),
	}

	for _, ch := range clients {
		select {
		case ch <- webEvent:
		default:
		}
	}
}

func (b *eventBus) AddClient() (uint64, <-chan WebEvent, func()) {
	id := atomic.AddUint64(&eventClientCounter, 1)
	ch := make(chan WebEvent, 64)

	b.mu.Lock()
	b.clients[id] = ch
	b.mu.Unlock()

	remove := func() {
		b.mu.Lock()
		client, ok := b.clients[id]
		if ok {
			delete(b.clients, id)
		}
		b.mu.Unlock()

		if ok {
			close(client)
		}
	}

	return id, ch, remove
}

func emitRuntimeEvent(a *App, event string, args ...any) {
	if Env.IsWeb {
		webEventBus.Emit(event, args...)
		return
	}
	runtime.EventsEmit(a.Ctx, event, args...)
}

func onRuntimeEvent(a *App, event string, handler eventHandler) {
	if Env.IsWeb {
		webEventBus.On(event, handler)
		return
	}
	runtime.EventsOn(a.Ctx, event, handler)
}

func offRuntimeEvent(a *App, event string, additionalEvents ...string) {
	if Env.IsWeb {
		webEventBus.Off(event)
		for _, additionalEvent := range additionalEvents {
			webEventBus.Off(additionalEvent)
		}
		return
	}
	runtime.EventsOff(a.Ctx, event, additionalEvents...)
}
