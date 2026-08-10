// Copyright (c) 2015-2025 The libusb developers. All rights reserved.
// Project site: https://github.com/Tryanks/libusb
// Use of this source code is governed by a MIT-style license that
// can be found in the LICENSE.txt file for the project.

package libusb

import (
	"sync"
	"testing"
	"time"
)

func TestTransferConcurrentRegistryCancelClose(t *testing.T) {
	transfer := &Transfer{}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				registerTransfer(transfer)
				_ = loadTransfer(nil)
				if err := transfer.Cancel(); err != nil {
					t.Errorf("Cancel() = %v, want nil", err)
				}
				if err := transfer.Close(); err != nil {
					t.Errorf("Close() = %v, want nil", err)
				}
				unregisterTransfer(nil)
			}
		}()
	}
	wg.Wait()
}

func TestTransferStatusString(t *testing.T) {
	if got := TransferCompleted.String(); got != "Completed" {
		t.Fatalf("TransferCompleted.String() = %q, want %q", got, "Completed")
	}
}

func TestTransferCancelIdempotent(t *testing.T) {
	transfer := &Transfer{}
	if err := transfer.Cancel(); err != nil {
		t.Fatalf("Cancel() on zero-value transfer = %v, want nil", err)
	}
	if err := transfer.Cancel(); err != nil {
		t.Fatalf("second Cancel() on zero-value transfer = %v, want nil", err)
	}
}

func TestTransferCloseBusy(t *testing.T) {
	transfer := &Transfer{submitted: true}
	if err := transfer.Close(); err != ErrorCode(errorBusy) {
		t.Fatalf("Close() = %v, want errorBusy", err)
	}
}

func TestTransferCompleteWithPayloadCopiesBulkData(t *testing.T) {
	transfer := &Transfer{
		TransferType:    BulkTransfer,
		Buffer:          make([]byte, 4),
		RequestedLength: 4,
	}

	transfer.completeWithPayload([]byte{1, 2, 3, 4}, TransferCompleted, 4, nil)

	if !transfer.completed {
		t.Fatal("completed = false, want true")
	}
	if transfer.Status != TransferCompleted {
		t.Fatalf("Status = %v, want %v", transfer.Status, TransferCompleted)
	}
	if got := string(transfer.Buffer); got != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("Buffer = %v, want %v", transfer.Buffer, []byte{1, 2, 3, 4})
	}
}

func TestTransferCompleteWithPayloadCopiesControlData(t *testing.T) {
	transfer := &Transfer{
		controlData:     true,
		TransferType:    ControlTransfer,
		Buffer:          make([]byte, 2),
		RequestedLength: 2,
	}

	transfer.completeWithPayload([]byte{0x12, 0x34}, TransferCompleted, 2, nil)

	if got := string(transfer.Buffer); got != string([]byte{0x12, 0x34}) {
		t.Fatalf("Buffer = %v, want %v", transfer.Buffer, []byte{0x12, 0x34})
	}
}

func TestTransferCompleteWithPayloadSetsTerminalError(t *testing.T) {
	transfer := &Transfer{Buffer: make([]byte, 1)}
	transfer.completeWithPayload([]byte{0x99}, TransferTimedOut, 1, nil)
	if err := transfer.Status.ErrorCode(); err != errorTransferTimedOut {
		t.Fatalf("Status.ErrorCode() = %v, want %v", err, errorTransferTimedOut)
	}
}

func TestContextEventLoopLifecycle(t *testing.T) {
	ctx, err := NewContext()
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}

	ctx.eventLoop.ensureRunning()
	time.Sleep(10 * time.Millisecond)

	ctx.eventLoop.mu.Lock()
	running := ctx.eventLoop.running
	ctx.eventLoop.mu.Unlock()
	if !running {
		_ = ctx.Close()
		t.Fatal("event loop is not running after ensureRunning()")
	}

	if err := ctx.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ctx.eventLoop.mu.Lock()
	stopped := !ctx.eventLoop.running
	ctx.eventLoop.mu.Unlock()
	if !stopped {
		t.Fatal("event loop is still running after Close()")
	}
}
