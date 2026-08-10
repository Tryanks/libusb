// Copyright (c) 2015-2025 The libusb developers. All rights reserved.
// Project site: https://github.com/Tryanks/libusb
// Use of this source code is governed by a MIT-style license that
// can be found in the LICENSE.txt file for the project.

package libusb

// #cgo pkg-config: libusb-1.0
// #include <libusb.h>
// #include <stdlib.h>
// void libusbTransferCallback(struct libusb_transfer *transfer);
import "C"
import (
	"sync"
	"unsafe"
)

// TransferStatus mirrors libusb transfer completion states.
type TransferStatus int

// TransferFlag controls optional libusb transfer behaviors.
type TransferFlag uint8

// Transfer completion states.
const (
	TransferCompleted TransferStatus = C.LIBUSB_TRANSFER_COMPLETED
	TransferError     TransferStatus = C.LIBUSB_TRANSFER_ERROR
	TransferTimedOut  TransferStatus = C.LIBUSB_TRANSFER_TIMED_OUT
	TransferCanceled  TransferStatus = C.LIBUSB_TRANSFER_CANCELLED
	TransferStall     TransferStatus = C.LIBUSB_TRANSFER_STALL
	TransferNoDevice  TransferStatus = C.LIBUSB_TRANSFER_NO_DEVICE
	TransferOverflow  TransferStatus = C.LIBUSB_TRANSFER_OVERFLOW
)

const (
	TransferShortNotOK    TransferFlag = C.LIBUSB_TRANSFER_SHORT_NOT_OK
	TransferAddZeroPacket TransferFlag = C.LIBUSB_TRANSFER_ADD_ZERO_PACKET
)

var transferStatuses = map[TransferStatus]string{
	TransferCompleted: "Completed",
	TransferError:     "Error",
	TransferTimedOut:  "Timed out",
	TransferCanceled:  "Canceled",
	TransferStall:     "Stall",
	TransferNoDevice:  "No device",
	TransferOverflow:  "Overflow",
}

// String implements fmt.Stringer for transfer statuses.
func (status TransferStatus) String() string {
	return transferStatuses[status]
}

// ErrorCode returns the terminal error value associated with the transfer
// status. Completed transfers return nil.
func (status TransferStatus) ErrorCode() error {
	return transferCompletionError(status)
}

// TransferCallback receives transfer completion notifications.
type TransferCallback func(*Transfer)

// Transfer is the asynchronous libusb transfer wrapper.
type Transfer struct {
	mu sync.RWMutex

	ctx         *Context
	handle      *DeviceHandle
	transfer    *C.struct_libusb_transfer
	cBuffer     unsafe.Pointer
	controlData bool

	TransferType    TransferType
	Endpoint        EndpointAddress
	Buffer          []byte
	RequestedLength int
	ActualLength    int
	Status          TransferStatus
	UserData        any

	callback  TransferCallback
	submitted bool
	completed bool
}

var (
	transferRegistryMu sync.RWMutex
	transferRegistry   = make(map[uintptr]*Transfer)
)

func registerTransfer(t *Transfer) {
	if t == nil {
		return
	}
	t.mu.RLock()
	transfer := t.transfer
	t.mu.RUnlock()
	registerTransferPointer(transfer, t)
}

func registerTransferPointer(cTransfer *C.struct_libusb_transfer, t *Transfer) {
	transferRegistryMu.Lock()
	transferRegistry[uintptr(unsafe.Pointer(cTransfer))] = t
	transferRegistryMu.Unlock()
}

func loadTransfer(cTransfer *C.struct_libusb_transfer) *Transfer {
	transferRegistryMu.RLock()
	defer transferRegistryMu.RUnlock()
	return transferRegistry[uintptr(unsafe.Pointer(cTransfer))]
}

func unregisterTransfer(cTransfer *C.struct_libusb_transfer) {
	transferRegistryMu.Lock()
	delete(transferRegistry, uintptr(unsafe.Pointer(cTransfer)))
	transferRegistryMu.Unlock()
}

// NewBulkTransfer creates a bulk transfer that can later be submitted.
func (dh *DeviceHandle) NewBulkTransfer(
	endpoint EndpointAddress,
	data []byte,
	length int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.newDataTransfer(BulkTransfer, endpoint, data, length, timeout, callback)
}

// NewBulkTransferOut creates a bulk OUT transfer.
func (dh *DeviceHandle) NewBulkTransferOut(
	endpoint EndpointAddress,
	data []byte,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.NewBulkTransfer(endpoint, data, len(data), timeout, callback)
}

// NewBulkTransferIn creates a bulk IN transfer with a preallocated receive buffer.
func (dh *DeviceHandle) NewBulkTransferIn(
	endpoint EndpointAddress,
	maxReceiveBytes int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.NewBulkTransfer(endpoint, make([]byte, maxReceiveBytes), maxReceiveBytes, timeout, callback)
}

// NewInterruptTransfer creates an interrupt transfer.
func (dh *DeviceHandle) NewInterruptTransfer(
	endpoint EndpointAddress,
	data []byte,
	length int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.newDataTransfer(InterruptTransfer, endpoint, data, length, timeout, callback)
}

// NewInterruptTransferOut creates an interrupt OUT transfer.
func (dh *DeviceHandle) NewInterruptTransferOut(
	endpoint EndpointAddress,
	data []byte,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.NewInterruptTransfer(endpoint, data, len(data), timeout, callback)
}

// NewInterruptTransferIn creates an interrupt IN transfer.
func (dh *DeviceHandle) NewInterruptTransferIn(
	endpoint EndpointAddress,
	maxReceiveBytes int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	return dh.NewInterruptTransfer(endpoint, make([]byte, maxReceiveBytes), maxReceiveBytes, timeout, callback)
}

// NewControlTransfer creates an asynchronous control transfer.
func (dh *DeviceHandle) NewControlTransfer(
	requestType byte,
	request byte,
	value uint16,
	index uint16,
	data []byte,
	length int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	if dh == nil || dh.libusbDeviceHandle == nil || dh.ctx == nil {
		return nil, ErrorCode(errorInvalidParam)
	}
	if length < 0 {
		return nil, ErrorCode(errorInvalidParam)
	}

	visible := make([]byte, length)
	copy(visible, data)

	totalLength := int(C.LIBUSB_CONTROL_SETUP_SIZE) + length
	buffer := C.malloc(C.size_t(totalLength))
	if buffer == nil {
		return nil, ErrorCode(errorNoMem)
	}

	cBuffer := (*C.uchar)(buffer)
	C.libusb_fill_control_setup(
		cBuffer,
		C.uint8_t(requestType),
		C.uint8_t(request),
		C.uint16_t(value),
		C.uint16_t(index),
		C.uint16_t(length),
	)

	if length > 0 {
		payload := unsafe.Slice(
			(*byte)(unsafe.Pointer(uintptr(buffer)+uintptr(C.LIBUSB_CONTROL_SETUP_SIZE))),
			length,
		)
		copy(payload, data)
	}

	transfer := C.libusb_alloc_transfer(0)
	if transfer == nil {
		C.free(buffer)
		return nil, ErrorCode(errorNoMem)
	}

	C.libusb_fill_control_transfer(
		transfer,
		dh.libusbDeviceHandle,
		cBuffer,
		C.libusb_transfer_cb_fn(unsafe.Pointer(C.libusbTransferCallback)),
		nil,
		C.uint(timeout),
	)

	return &Transfer{
		ctx:             dh.ctx,
		handle:          dh,
		transfer:        transfer,
		cBuffer:         buffer,
		controlData:     true,
		TransferType:    ControlTransfer,
		Endpoint:        0,
		Buffer:          visible,
		RequestedLength: length,
		callback:        callback,
	}, nil
}

func (dh *DeviceHandle) newDataTransfer(
	transferType TransferType,
	endpoint EndpointAddress,
	data []byte,
	length int,
	timeout int,
	callback TransferCallback,
) (*Transfer, error) {
	if dh == nil || dh.libusbDeviceHandle == nil || dh.ctx == nil {
		return nil, ErrorCode(errorInvalidParam)
	}
	if length < 0 {
		return nil, ErrorCode(errorInvalidParam)
	}

	visible := make([]byte, length)
	copy(visible, data)

	var cBuffer unsafe.Pointer
	if length > 0 {
		cBuffer = C.malloc(C.size_t(length))
		if cBuffer == nil {
			return nil, ErrorCode(errorNoMem)
		}
		payload := unsafe.Slice((*byte)(cBuffer), length)
		copy(payload, data)
	}

	transfer := C.libusb_alloc_transfer(0)
	if transfer == nil {
		if cBuffer != nil {
			C.free(cBuffer)
		}
		return nil, ErrorCode(errorNoMem)
	}

	switch transferType {
	case BulkTransfer:
		C.libusb_fill_bulk_transfer(
			transfer,
			dh.libusbDeviceHandle,
			C.uchar(endpoint),
			(*C.uchar)(cBuffer),
			C.int(length),
			C.libusb_transfer_cb_fn(unsafe.Pointer(C.libusbTransferCallback)),
			nil,
			C.uint(timeout),
		)
	case InterruptTransfer:
		C.libusb_fill_interrupt_transfer(
			transfer,
			dh.libusbDeviceHandle,
			C.uchar(endpoint),
			(*C.uchar)(cBuffer),
			C.int(length),
			C.libusb_transfer_cb_fn(unsafe.Pointer(C.libusbTransferCallback)),
			nil,
			C.uint(timeout),
		)
	default:
		if cBuffer != nil {
			C.free(cBuffer)
		}
		C.libusb_free_transfer(transfer)
		return nil, ErrorCode(errorInvalidParam)
	}

	return &Transfer{
		ctx:             dh.ctx,
		handle:          dh,
		transfer:        transfer,
		cBuffer:         cBuffer,
		TransferType:    transferType,
		Endpoint:        endpoint,
		Buffer:          visible,
		RequestedLength: length,
		callback:        callback,
	}, nil
}

// Submit submits the asynchronous transfer to libusb.
func (t *Transfer) Submit() error {
	if t == nil {
		return ErrorCode(errorInvalidParam)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.transfer == nil {
		return ErrorCode(errorInvalidParam)
	}
	if t.submitted {
		return ErrorCode(errorBusy)
	}
	if t.completed {
		return ErrorCode(errorBusy)
	}
	if t.ctx == nil || t.ctx.eventLoop == nil {
		return ErrorCode(errorInvalidParam)
	}
	t.submitted = true

	t.ctx.eventLoop.ensureRunning()
	registerTransferPointer(t.transfer, t)

	err := C.libusb_submit_transfer(t.transfer)
	if err != 0 {
		unregisterTransfer(t.transfer)
		t.submitted = false
		return ErrorCode(err)
	}
	return nil
}

// Cancel attempts to cancel a submitted transfer.
func (t *Transfer) Cancel() error {
	if t == nil {
		return ErrorCode(errorInvalidParam)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	transfer := t.transfer
	submitted := t.submitted
	completed := t.completed

	if transfer == nil || completed || !submitted {
		return nil
	}

	err := C.libusb_cancel_transfer(transfer)
	if err == 0 || ErrorCode(err) == errorNotFound {
		return nil
	}
	return ErrorCode(err)
}

// Close releases a transfer that is not currently in flight.
func (t *Transfer) Close() error {
	if t == nil {
		return ErrorCode(errorInvalidParam)
	}

	t.mu.Lock()
	if t.submitted && !t.completed {
		t.mu.Unlock()
		return ErrorCode(errorBusy)
	}
	transfer := t.transfer
	buffer := t.cBuffer
	t.transfer = nil
	t.cBuffer = nil
	t.mu.Unlock()

	if transfer != nil {
		unregisterTransfer(transfer)
		C.libusb_free_transfer(transfer)
	}
	if buffer != nil {
		C.free(buffer)
	}
	return nil
}

// SetFlags applies libusb transfer behavior flags before submission.
func (t *Transfer) SetFlags(flags uint8) error {
	if t == nil {
		return ErrorCode(errorInvalidParam)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.transfer == nil {
		return ErrorCode(errorInvalidParam)
	}
	if t.submitted || t.completed {
		return ErrorCode(errorBusy)
	}

	t.transfer.flags |= C.uint8_t(flags)
	return nil
}

func (t *Transfer) finish(cTransfer *C.struct_libusb_transfer) {
	actualLength := int(cTransfer.actual_length)
	if actualLength < 0 {
		actualLength = 0
	}

	var payload []byte
	if actualLength > 0 {
		if t.controlData {
			payload = unsafe.Slice(
				(*byte)(unsafe.Pointer(C.libusb_control_transfer_get_data(cTransfer))),
				actualLength,
			)
		} else {
			payload = unsafe.Slice(
				(*byte)(unsafe.Pointer(cTransfer.buffer)),
				actualLength,
			)
		}
	}

	t.completeWithPayload(payload, TransferStatus(cTransfer.status), actualLength, cTransfer)
}

func (t *Transfer) completeWithPayload(
	payload []byte,
	status TransferStatus,
	actualLength int,
	cTransfer *C.struct_libusb_transfer,
) {
	t.mu.Lock()
	if t.completed {
		t.mu.Unlock()
		return
	}

	t.Status = status
	t.ActualLength = actualLength
	if t.ActualLength > len(t.Buffer) {
		t.ActualLength = len(t.Buffer)
	}
	if t.ActualLength > 0 && len(payload) > 0 {
		copy(t.Buffer, payload[:t.ActualLength])
	}

	t.completed = true
	t.submitted = false

	transfer := t.transfer
	buffer := t.cBuffer
	callback := t.callback
	t.transfer = nil
	t.cBuffer = nil
	t.mu.Unlock()

	if cTransfer != nil {
		unregisterTransfer(cTransfer)
	}
	if transfer != nil {
		C.libusb_free_transfer(transfer)
	}
	if buffer != nil {
		C.free(buffer)
	}
	if callback != nil {
		callback(t)
	}
}

//export libusbTransferCallback
func libusbTransferCallback(cTransfer *C.struct_libusb_transfer) {
	transfer := loadTransfer(cTransfer)
	if transfer == nil {
		return
	}
	transfer.finish(cTransfer)
}
