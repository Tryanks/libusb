// Copyright (c) 2015-2025 The libusb developers. All rights reserved.
// Project site: https://github.com/Tryanks/libusb
// Use of this source code is governed by a MIT-style license that
// can be found in the LICENSE.txt file for the project.

package libusb

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
// #include <sys/time.h>
// static int libusb_handle_events_timeout_completed_wrapper(
// 	libusb_context *ctx, int timeout_ms, int *completed) {
//	struct timeval tv;
//	tv.tv_sec = timeout_ms / 1000;
//	tv.tv_usec = (timeout_ms % 1000) * 1000;
//	return libusb_handle_events_timeout_completed(ctx, &tv, completed);
// }
import "C"
import (
	"log"
	"sync"
	"unsafe"
)

const eventLoopTimeoutMs = 50

type contextEventLoop struct {
	ctx     *Context
	mu      sync.Mutex
	running bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

func newContextEventLoop(ctx *Context) *contextEventLoop {
	return &contextEventLoop{ctx: ctx}
}

func (loop *contextEventLoop) ensureRunning() {
	if loop == nil || loop.ctx == nil {
		return
	}
	loop.ctx.mu.Lock()
	defer loop.ctx.mu.Unlock()
	if loop.ctx.libusbContext == nil {
		return
	}

	loop.mu.Lock()
	if loop.running {
		loop.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	loop.stop = stop
	loop.running = true
	loop.wg.Add(1)
	loop.mu.Unlock()

	go loop.run(stop)
}

func (loop *contextEventLoop) stopLoop() {
	if loop == nil {
		return
	}

	loop.mu.Lock()
	if !loop.running {
		loop.mu.Unlock()
		return
	}
	stop := loop.stop
	loop.stop = nil
	loop.running = false
	loop.mu.Unlock()

	close(stop)
	loop.wg.Wait()
}

func (loop *contextEventLoop) run(stop <-chan struct{}) {
	defer loop.wg.Done()

	for {
		select {
		case <-stop:
			return
		default:
		}

		var completed C.int
		errno := C.libusb_handle_events_timeout_completed_wrapper(
			loop.ctx.libusbContext,
			C.int(eventLoopTimeoutMs),
			&completed,
		)
		if errno < 0 && ErrorCode(errno) != errorInterrupted {
			log.Printf("handle_events error: %s", ErrorCode(errno))
		}
	}
}

var (
	contextRegistryMu sync.RWMutex
	contextRegistry   = make(map[uintptr]*Context)
)

func registerContext(ctx *Context) {
	if ctx == nil || ctx.libusbContext == nil {
		return
	}

	contextRegistryMu.Lock()
	contextRegistry[uintptr(unsafe.Pointer(ctx.libusbContext))] = ctx
	contextRegistryMu.Unlock()
}

func unregisterContext(ctx *Context) {
	if ctx == nil || ctx.libusbContext == nil {
		return
	}

	contextRegistryMu.Lock()
	delete(contextRegistry, uintptr(unsafe.Pointer(ctx.libusbContext)))
	contextRegistryMu.Unlock()
}

func contextByLibusbContext(libCtx *C.libusb_context) *Context {
	if libCtx == nil {
		return nil
	}

	contextRegistryMu.RLock()
	defer contextRegistryMu.RUnlock()
	return contextRegistry[uintptr(unsafe.Pointer(libCtx))]
}
