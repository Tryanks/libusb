// Copyright (c) 2015-2025 The libusb developers. All rights reserved.
// Project site: https://github.com/Tryanks/libusb
// Use of this source code is governed by a MIT-style license that
// can be found in the LICENSE.txt file for the project.

package libusb

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
// int libusbHotplugCallback (libusb_context *ctx, libusb_device *device, libusb_hotplug_event event, void *user_data);
// static int libusb_hotplug_register_callback_wrapper (
//	libusb_context *ctx,
//	int events, int flags,
//	int vendor_id, int product_id, int dev_class,
//	libusb_hotplug_callback_fn cb_fn, void *user_data,
//	libusb_hotplug_callback_handle *callback_handle)
//	{
// 		return libusb_hotplug_register_callback(ctx, events, flags, vendor_id, product_id, dev_class, cb_fn, user_data, callback_handle);
// }
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// HotPlugEventType represents the type of hotplug event.
type HotPlugEventType uint8

// HotPlugCbFunc is the callback function signature for hotplug events.
type HotPlugCbFunc func(event HotPlugEvent)

// HotPlug Event Types
const (
	HotplugUndefined HotPlugEventType = iota
	HotplugArrived
	HotplugLeft
)

// HotPlugEvent is the structured hotplug callback payload.
type HotPlugEvent struct {
	VendorID  uint16
	ProductID uint16
	Event     HotPlugEventType
	Identity  DeviceIdentity
}

type hotplugCallback struct {
	handler *C.libusb_hotplug_callback_handle
	fn      HotPlugCbFunc
}

// HotplugCallbackStorage holds the callback map for a single context.
type HotplugCallbackStorage struct {
	callbackMap map[uint32]hotplugCallback
	mu          sync.RWMutex
}

// HotplugRegisterCallbackEvent registers a hotplug callback for the given
// vendor/product ID pair and event type.
func (ctx *Context) HotplugRegisterCallbackEvent(
	vendorID, productID uint16,
	eventType HotPlugEventType,
	cb HotPlugCbFunc,
) error {
	if ctx == nil || ctx.libusbContext == nil {
		return ErrorCode(errorInvalidParam)
	}

	var event C.int
	switch eventType {
	case HotplugArrived:
		event = C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED
	case HotplugLeft:
		event = C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT
	default:
		event = C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED |
			C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT
	}

	var vID C.int = C.LIBUSB_HOTPLUG_MATCH_ANY
	var pID C.int = C.LIBUSB_HOTPLUG_MATCH_ANY
	if vendorID != 0 {
		vID = C.int(vendorID)
	}
	if productID != 0 {
		pID = C.int(productID)
	}

	var cbHandle C.libusb_hotplug_callback_handle
	rc := C.libusb_hotplug_register_callback_wrapper(
		ctx.libusbContext,
		event,
		C.LIBUSB_HOTPLUG_NO_FLAGS,
		vID,
		pID,
		C.LIBUSB_HOTPLUG_MATCH_ANY,
		C.libusb_hotplug_callback_fn(
			unsafe.Pointer(C.libusbHotplugCallback),
		),
		nil,
		&cbHandle,
	)
	if rc != C.LIBUSB_SUCCESS {
		return fmt.Errorf(
			"libusb_hotplug_register_callback error: %s",
			ErrorCode(rc),
		)
	}

	ctx.eventLoop.ensureRunning()

	key := vidPidToUint32(vendorID, productID)
	ctx.hotplugStorage.mu.Lock()
	ctx.hotplugStorage.callbackMap[key] = hotplugCallback{
		handler: &cbHandle,
		fn:      cb,
	}
	ctx.hotplugStorage.mu.Unlock()
	return nil
}

// HotplugDeregisterCallback deregisters the callback for a vendor/product pair.
// The context's shared event loop remains available for other async work.
func (ctx *Context) HotplugDeregisterCallback(vendorID, productID uint16) error {
	if ctx == nil || ctx.libusbContext == nil || ctx.hotplugStorage == nil {
		return nil
	}
	key := vidPidToUint32(vendorID, productID)
	ctx.hotplugStorage.mu.Lock()
	cb, ok := ctx.hotplugStorage.callbackMap[key]
	if ok {
		delete(ctx.hotplugStorage.callbackMap, key)
	}
	ctx.hotplugStorage.mu.Unlock()
	if ok {
		C.libusb_hotplug_deregister_callback(ctx.libusbContext, *cb.handler)
	}
	return nil
}

// HotplugDeregisterAllCallbacks deregisters all hotplug callbacks for this
// context.
func (ctx *Context) HotplugDeregisterAllCallbacks() error {
	if ctx == nil || ctx.libusbContext == nil || ctx.hotplugStorage == nil {
		return nil
	}

	ctx.hotplugStorage.mu.RLock()
	handlers := make(
		[]*C.libusb_hotplug_callback_handle,
		0,
		len(ctx.hotplugStorage.callbackMap),
	)
	for _, cb := range ctx.hotplugStorage.callbackMap {
		handlers = append(handlers, cb.handler)
	}
	ctx.hotplugStorage.mu.RUnlock()

	for _, handler := range handlers {
		C.libusb_hotplug_deregister_callback(ctx.libusbContext, *handler)
	}

	ctx.hotplugStorage.mu.Lock()
	ctx.hotplugStorage.callbackMap = make(map[uint32]hotplugCallback)
	ctx.hotplugStorage.mu.Unlock()
	return nil
}

//export libusbHotplugCallback
func libusbHotplugCallback(
	libCtx *C.libusb_context,
	dev *C.libusb_device,
	event C.libusb_hotplug_event,
	p unsafe.Pointer,
) C.int {
	ctx := contextByLibusbContext(libCtx)
	if ctx == nil || ctx.hotplugStorage == nil {
		return C.LIBUSB_SUCCESS
	}

	identity, err := deviceIdentityFromLibusbDevice(dev)
	if err != nil {
		return C.LIBUSB_SUCCESS
	}

	var eventType HotPlugEventType
	switch event {
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_ARRIVED:
		eventType = HotplugArrived
	case C.LIBUSB_HOTPLUG_EVENT_DEVICE_LEFT:
		eventType = HotplugLeft
	default:
		eventType = HotplugUndefined
	}

	payload := HotPlugEvent{
		VendorID:  identity.VendorID,
		ProductID: identity.ProductID,
		Event:     eventType,
		Identity:  *identity,
	}

	ctx.hotplugStorage.mu.RLock()
	cb, ok := ctx.hotplugStorage.callbackMap[vidPidToUint32(payload.VendorID, payload.ProductID)]
	var deviceCallback HotPlugCbFunc
	if ok {
		deviceCallback = cb.fn
	}
	cb, ok = ctx.hotplugStorage.callbackMap[0]
	var allCallback HotPlugCbFunc
	if ok {
		allCallback = cb.fn
	}
	ctx.hotplugStorage.mu.RUnlock()

	if deviceCallback != nil {
		deviceCallback(payload)
	}
	if allCallback != nil {
		allCallback(payload)
	}

	return C.LIBUSB_SUCCESS
}

func vidPidToUint32(vID, pID uint16) uint32 {
	return (uint32(vID) << 16) | uint32(pID)
}
