package tp

import (
	"testing"
)

func TestPDUParseSingleFrame(t *testing.T) {
	msg := CanMessage{Data: []byte{0x05, 1, 2, 3, 4, 5}}
	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("unexpected error parsing PDU: %v", err)
	}
	if pdu.Type != PDUSingleFrame {
		t.Fatalf("expected SINGLE_FRAME type, got %v", pdu.Type)
	}
	if pdu.Length == nil || *pdu.Length != 5 {
		t.Fatalf("expected length 5, got %v", pdu.Length)
	}
	if got := string(pdu.Data); got != string([]byte{1, 2, 3, 4, 5}) {
		t.Fatalf("unexpected data: %v", pdu.Data)
	}
}

func TestPDUParseFirstFrameWithEscapeLength(t *testing.T) {
	// Length encoded on bytes 2..5 (0x00000100 = 256 bytes)
	msg := CanMessage{Data: []byte{0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0xAA, 0xBB}}
	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("unexpected error parsing first frame: %v", err)
	}
	if pdu.Type != PDUFirstFrame {
		t.Fatalf("expected FIRST_FRAME type, got %v", pdu.Type)
	}
	if pdu.Length == nil || *pdu.Length != 256 {
		t.Fatalf("expected length 256, got %v", pdu.Length)
	}
	if len(pdu.Data) != 2 { // Only 2 payload bytes available after 6-byte header
		t.Fatalf("expected 2 bytes of data, got %d", len(pdu.Data))
	}
	if pdu.Data[0] != 0xAA || pdu.Data[1] != 0xBB {
		t.Fatalf("unexpected payload bytes: %v", pdu.Data)
	}
}

func TestTransportLayerLogicSendSingleFrame(t *testing.T) {
	addr := NewAddress(Normal11bits, 0x123, 0x321, 0x44, 0x55, 0, 0, 0, false, false)

	var sent []CanMessage
	txfn := func(msg *CanMessage) {
		sent = append(sent, *msg)
	}
	rxfn := func(timeout float64) *CanMessage { return nil }

	tll := NewTransportLayerLogic(rxfn, txfn, addr, nil, nil, nil)
	if err := tll.Send([]byte{0xAA, 0xBB, 0xCC}, Physical, 0); err != nil {
		t.Fatalf("send returned error: %v", err)
	}

	stats := tll.Process(0, true, true)
	if stats.Sent != 1 {
		t.Fatalf("expected 1 CAN message sent, got %d", stats.Sent)
	}

	if len(sent) != 1 {
		t.Fatalf("expected exactly one message captured, got %d", len(sent))
	}
	msg := sent[0]
	if msg.ArbitrationId != addr.GetTxArbitrationId(Physical) {
		t.Fatalf("unexpected arbitration id: %X", msg.ArbitrationId)
	}
	if len(msg.Data) != 4 {
		t.Fatalf("expected 4 data bytes (PCI + payload), got %d", len(msg.Data))
	}
	if msg.Data[0] != 0x03 { // Single Frame, length on low nibble
		t.Fatalf("unexpected PCI byte: 0x%02X", msg.Data[0])
	}
	if got := msg.Data[1:]; string(got) != string([]byte{0xAA, 0xBB, 0xCC}) {
		t.Fatalf("unexpected payload bytes: %v", got)
	}
	if msg.Dlc != len(msg.Data) {
		t.Fatalf("expected DLC %d, got %d", len(msg.Data), msg.Dlc)
	}
}
