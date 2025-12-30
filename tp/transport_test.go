package tp

import (
	"context"
	"testing"
	"time"
)

// TestTransport_WaitFrameLimit verifies that the sender aborts after receiving too many FlowControl Wait frames.
func TestTransport_WaitFrameLimit(t *testing.T) {
	// Configure sender with MaxWaitFrame = 2
	cfg := DefaultConfig()
	cfg.MaxWaitFrame = 2

	addr1 := &Address{TxID: 0x1, RxID: 0x2}
	t1 := NewTransport(addr1, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Virtual CAN bus
	rxChan := make(chan CanMessage, 100)
	txChan := make(chan CanMessage, 100)

	go t1.Run(ctx, rxChan, txChan)

	// Start sending a multi-frame message (e.g. 20 bytes)
	payload := make([]byte, 20)
	go t1.Send(payload)

	// 1. Sender should send First Frame (FF)
	select {
	case msg := <-txChan:
		// Verify it's FF (0x1 in PCI high nibble)
		// For CAN, PCI is byte 0.
		if (msg.Data[0] & 0xF0) != 0x10 {
			t.Fatalf("Expected First Frame, got %X", msg.Data)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for First Frame")
	}

	// 2. We (Receiver) send Flow Control WAIT frames
	// Send 1st Wait
	waitFrame := CanMessage{
		ArbitrationID: 0x2,
		Data:          []byte{0x31, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // FlowStatus=1 (Wait)
	}
	rxChan <- waitFrame

	// Sender waits... Timer reset.
	// We verify sender does NOT send CF yet.
	select {
	case msg := <-txChan:
		t.Fatalf("Sender forwarded unexpected frame: %X", msg.Data)
	case <-time.After(200 * time.Millisecond):
		// Good, silence.
	}

	// Send 2nd Wait
	rxChan <- waitFrame

	select {
	case msg := <-txChan:
		t.Fatalf("Sender forwarded unexpected frame: %X", msg.Data)
	case <-time.After(200 * time.Millisecond):
		// Good
	}

	// Send 3rd Wait (Should exceed limit 2)
	rxChan <- waitFrame

	// 3. Sender should report error
	select {
	case err := <-t1.ErrorChan:
		if err.Error() != "错误：等待帧(Wait Frame)数量超出最大限制" {
			t.Errorf("Unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for error limit exceeded")
	}

	// Verify state is Idle (or stopped)
	// We can check if it accepts new sends or check internal state (not exposed easily, but Send should work again)
}

func TestTransport_Padding(t *testing.T) {
	// Configure with Padding
	cfg := DefaultConfig()
	cfg.TxDataMinLength = 8
	padByte := byte(0xCC)
	cfg.PaddingByte = &padByte

	addr := &Address{TxID: 0x1, RxID: 0x2}
	tr := NewTransport(addr, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rxChan := make(chan CanMessage, 100)
	txChan := make(chan CanMessage, 100)
	go tr.Run(ctx, rxChan, txChan)

	// Send 1 byte. Should be padded to 8 bytes.
	// 1 byte means internal SingleFrame: 1 byte PCI + 1 byte Data. Total 2 bytes used.
	// Padding needed: 6 bytes.
	tr.Send([]byte{0xAA})

	select {
	case msg := <-txChan:
		if len(msg.Data) != 8 {
			t.Errorf("Expected 8 bytes, got %d", len(msg.Data))
		}
		// Check Content
		// PCI (SF, len=1) = 0x01
		// Data = 0xAA
		// Padding = 0xCC ...
		expected := []byte{0x01, 0xAA, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC, 0xCC}
		if string(msg.Data) != string(expected) {
			t.Errorf("Padding content mismatch.\nGot: %X\nExp: %X", msg.Data, expected)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for padded frame")
	}
}
