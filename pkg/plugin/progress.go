// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package plugin

import (
	"sync"
	"time"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Publisher is a thread-safe, non-blocking sink for plugin-side progress
// events. The plugin's SCCloudPluginServer impl calls Push from any RPC
// handler; SC core's LogStream RPC drains events into its logger. Backs a
// bounded ring buffer so Push never blocks even if SC core isn't draining
// yet (startup race) or has gone away (shutdown). Dropped events are
// counted so a plugin's own health probe can surface the loss.
//
// Lifetime: one per plugin process. The plugin's main() constructs one and
// passes it to the server impl; the impl exposes it on the LogStream RPC.
// It survives across RPCs. Drain blocks until Close is called OR the stream
// context cancels.
type Publisher struct {
	mu       sync.Mutex
	cond     *sync.Cond
	buf      []*scpluginv1.PluginLogEvent
	capacity int
	dropped  uint64
	closed   bool
}

// NewPublisher creates a Publisher with the given ring-buffer capacity.
// A capacity of 0 defaults to 1024, which is enough for even a chatty
// multi-record provision without back-pressure.
func NewPublisher(capacity int) *Publisher {
	if capacity <= 0 {
		capacity = 1024
	}
	p := &Publisher{
		buf:      make([]*scpluginv1.PluginLogEvent, 0, capacity),
		capacity: capacity,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Push enqueues one event. Non-blocking. When the buffer is full, the
// OLDEST event is dropped (ring semantics) — the newest progress is
// almost always what the operator wants to see.
func (p *Publisher) Push(sev scpluginv1.LogSeverity, phase, message string) {
	ev := &scpluginv1.PluginLogEvent{
		Severity:    sev,
		Message:     message,
		Phase:       phase,
		TimestampMs: time.Now().UnixMilli(),
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if len(p.buf) >= p.capacity {
		copy(p.buf, p.buf[1:])
		p.buf = p.buf[:len(p.buf)-1]
		p.dropped++
	}
	p.buf = append(p.buf, ev)
	p.cond.Signal()
}

// Info / Debug / Warn / Error are tiny convenience wrappers.
func (p *Publisher) Info(phase, msg string) {
	p.Push(scpluginv1.LogSeverity_LOG_SEVERITY_INFO, phase, msg)
}

func (p *Publisher) Debug(phase, msg string) {
	p.Push(scpluginv1.LogSeverity_LOG_SEVERITY_DEBUG, phase, msg)
}

func (p *Publisher) Warn(phase, msg string) {
	p.Push(scpluginv1.LogSeverity_LOG_SEVERITY_WARN, phase, msg)
}

func (p *Publisher) Error(phase, msg string) {
	p.Push(scpluginv1.LogSeverity_LOG_SEVERITY_ERROR, phase, msg)
}

// Drain blocks until an event is available (or the publisher is closed)
// and returns the next batch. Caller is expected to loop: drain → send
// over the gRPC stream → drain again. Returns an empty slice + false
// after Close.
func (p *Publisher) Drain() ([]*scpluginv1.PluginLogEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.buf) == 0 && !p.closed {
		p.cond.Wait()
	}
	if len(p.buf) == 0 && p.closed {
		return nil, false
	}
	out := p.buf
	p.buf = make([]*scpluginv1.PluginLogEvent, 0, p.capacity)
	return out, true
}

// Close wakes every blocked Drain so the gRPC stream handler can return.
// Idempotent.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.cond.Broadcast()
}

// Dropped returns the number of events dropped since start due to ring
// overflow. Observational only; a plugin can expose it on a healthz
// endpoint if it has one.
func (p *Publisher) Dropped() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}
