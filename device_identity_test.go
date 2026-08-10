package libusb

import "testing"

func TestDeviceIdentityBusID(t *testing.T) {
	tests := []struct {
		name     string
		identity DeviceIdentity
		want     string
	}{
		{
			name: "ports",
			identity: DeviceIdentity{
				BusNumber:   1,
				PortNumbers: []int{1, 4},
			},
			want: "1-1.4",
		},
		{
			name: "no ports",
			identity: DeviceIdentity{
				BusNumber: 2,
			},
			want: "2-0",
		},
		{
			name: "zero bus",
			identity: DeviceIdentity{
				PortNumbers: []int{3},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		if got := tc.identity.BusID(); got != tc.want {
			t.Fatalf("%s: BusID() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTransferStatusErrorCode(t *testing.T) {
	if err := TransferCompleted.ErrorCode(); err != nil {
		t.Fatalf("TransferCompleted.ErrorCode() = %v, want nil", err)
	}
	if err := TransferCanceled.ErrorCode(); err != errorTransferCanceled {
		t.Fatalf("TransferCanceled.ErrorCode() = %v, want %v", err, errorTransferCanceled)
	}
}
