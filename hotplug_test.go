// Copyright (c) 2015-2025 The libusb developers. All rights reserved.
// Project site: https://github.com/Tryanks/libusb
// Use of this source code is governed by a MIT-style license that
// can be found in the LICENSE.txt file for the project.

package libusb

import (
	"testing"
)

func TestVidPidToUint32(t *testing.T) {
	testCases := []struct {
		vid      uint16
		pid      uint16
		expected uint32
	}{
		{0x0000, 0x0000, 0x00000000},
		{0x1234, 0x5678, 0x12345678},
		{0xFFFF, 0xFFFF, 0xFFFFFFFF},
		{0x04B8, 0x0202, 0x04B80202},
		{0x0001, 0x0000, 0x00010000},
		{0x0000, 0x0001, 0x00000001},
	}
	for _, tc := range testCases {
		result := vidPidToUint32(tc.vid, tc.pid)
		if result != tc.expected {
			t.Errorf(
				"vidPidToUint32(0x%04X, 0x%04X) = 0x%08X, want 0x%08X",
				tc.vid, tc.pid, result, tc.expected,
			)
		}
	}
}

func TestHotPlugEventTypeConstants(t *testing.T) {
	if HotplugUndefined != 0 {
		t.Errorf("HotplugUndefined = %d, want 0", HotplugUndefined)
	}
	if HotplugArrived != 1 {
		t.Errorf("HotplugArrived = %d, want 1", HotplugArrived)
	}
	if HotplugLeft != 2 {
		t.Errorf("HotplugLeft = %d, want 2", HotplugLeft)
	}
}

func TestHotPlugEventStruct(t *testing.T) {
	event := HotPlugEvent{
		VendorID:  0x1234,
		ProductID: 0x5678,
		Event:     HotplugArrived,
		Identity: DeviceIdentity{
			BusNumber:     1,
			DeviceAddress: 2,
			PortNumbers:   []int{1, 4},
		},
	}
	if event.VendorID != 0x1234 {
		t.Errorf("VendorID = 0x%04X, want 0x1234", event.VendorID)
	}
	if event.ProductID != 0x5678 {
		t.Errorf("ProductID = 0x%04X, want 0x5678", event.ProductID)
	}
	if event.Event != HotplugArrived {
		t.Errorf("Event = %d, want HotplugArrived", event.Event)
	}
	if event.Identity.BusNumber != 1 {
		t.Errorf("Identity.BusNumber = %d, want 1", event.Identity.BusNumber)
	}
}

func TestHotplugCallbackStorageZeroValue(t *testing.T) {
	storage := &HotplugCallbackStorage{}
	if storage.callbackMap != nil {
		t.Error("zero-value HotplugCallbackStorage should have nil callbackMap")
	}
}

func TestContextRegistryRoundTrip(t *testing.T) {
	if got := contextByLibusbContext(nil); got != nil {
		t.Fatalf("contextByLibusbContext(nil) = %#v, want nil", got)
	}
}
