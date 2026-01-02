package tp

import (
	"bytes"
	"testing"
	"time"
)

// --- PDU (Unpacking/Deframing) Tests ---

func TestPDU_SingleFrame_Standard(t *testing.T) {
	// SF with 3 bytes of data: [0x03, 0x11, 0x22, 0x33]
	data := []byte{0x03, 0x11, 0x22, 0x33}
	msg := CanMessage{Data: data}
	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse standard SF: %v", err)
	}
	if pdu.Type != PDUSingleFrame {
		t.Errorf("Expected Type %d, got %d", PDUSingleFrame, pdu.Type)
	}
	if *pdu.Length != 3 {
		t.Errorf("Expected Length 3, got %d", *pdu.Length)
	}
	if !bytes.Equal(pdu.Data, []byte{0x11, 0x22, 0x33}) {
		t.Errorf("Unexpected Data: %x", pdu.Data)
	}
}

func TestPDU_SingleFrame_EscapeSequence(t *testing.T) {
	// SF for CAN FD with >8 bytes (e.g. 10 chars).
	// Byte 0: 0x00 (SF with Escape)
	// Byte 1: 0x0A (Length = 10)
	// Bytes 2..11: Data
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A}
	data := append([]byte{0x00, 0x0A}, payload...)
	// Pad to emulate CAN FD frame size if needed, though NewPDU mainly checks provided data
	msg := CanMessage{Data: data}

	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse SF with escape: %v", err)
	}
	if pdu.Type != PDUSingleFrame {
		t.Errorf("Expected Type %d, got %d", PDUSingleFrame, pdu.Type)
	}
	if !pdu.EscapeSequence {
		t.Error("Expected EscapeSequence to be true")
	}
	if *pdu.Length != 10 {
		t.Errorf("Expected Length 10, got %d", *pdu.Length)
	}
	if !bytes.Equal(pdu.Data, payload) {
		t.Errorf("Unexpected Data: %x", pdu.Data)
	}
}

func TestPDU_FirstFrame_Standard(t *testing.T) {
	// FF with length 100 (0x064).
	// Byte 0: 0x10 | (0x064 >> 8) -> 0x10
	// Byte 1: 0x64
	// Bytes 2..7: First 6 bytes of payload
	payloadStart := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	data := append([]byte{0x10, 0x64}, payloadStart...)
	msg := CanMessage{Data: data}

	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse FF: %v", err)
	}
	if pdu.Type != PDUFirstFrame {
		t.Errorf("Expected Type %d, got %d", PDUFirstFrame, pdu.Type)
	}
	if *pdu.Length != 100 {
		t.Errorf("Expected Length 100, got %d", *pdu.Length)
	}
	if !bytes.Equal(pdu.Data, payloadStart) {
		t.Errorf("Unexpected Data: %x", pdu.Data)
	}
}

func TestPDU_FirstFrame_EscapeSequence(t *testing.T) {
	// FF with length 5000 (0x1388) > 4095.
	// Byte 0: 0x10
	// Byte 1: 0x00 (0)
	// Byte 2..5: 0x00001388 (Length)
	// Bytes 6..: Payload
	lengthBytes := []byte{0x00, 0x00, 0x13, 0x88}
	payloadStart := []byte{0x11, 0x22}
	data := append([]byte{0x10, 0x00}, lengthBytes...)
	data = append(data, payloadStart...)

	msg := CanMessage{Data: data}
	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse FF with escape: %v", err)
	}
	if pdu.Type != PDUFirstFrame {
		t.Errorf("Expected Type %d, got %d", PDUFirstFrame, pdu.Type)
	}
	if !pdu.EscapeSequence {
		t.Error("Expected EscapeSequence to be true")
	}
	if *pdu.Length != 5000 {
		t.Errorf("Expected Length 5000, got %d", *pdu.Length)
	}
	if !bytes.Equal(pdu.Data, payloadStart) {
		t.Errorf("Unexpected Data: %x", pdu.Data)
	}
}

func TestPDU_ConsecutiveFrame(t *testing.T) {
	// CF with Sequence Number 5.
	// Byte 0: 0x25 (0x20 | 0x05)
	// Bytes 1..7: Payload
	payload := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	data := append([]byte{0x25}, payload...)
	msg := CanMessage{Data: data}

	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse CF: %v", err)
	}
	if pdu.Type != PDUConsecutiveFrame {
		t.Errorf("Expected Type %d, got %d", PDUConsecutiveFrame, pdu.Type)
	}
	if *pdu.SeqNum != 5 {
		t.Errorf("Expected SeqNum 5, got %d", *pdu.SeqNum)
	}
	if !bytes.Equal(pdu.Data, payload) {
		t.Errorf("Unexpected Data: %x", pdu.Data)
	}
}

func TestPDU_FlowControl(t *testing.T) {
	// FC: ContinueToSend (0), BlockSize 10, StMin 5ms.
	// Byte 0: 0x30 | 0 = 0x30
	// Byte 1: 0x0A (10)
	// Byte 2: 0x05 (5)
	data := []byte{0x30, 0x0A, 0x05}
	msg := CanMessage{Data: data}

	pdu, err := NewPDU(msg, 0)
	if err != nil {
		t.Fatalf("Failed to parse FC: %v", err)
	}
	if pdu.Type != PDUFlowControl {
		t.Errorf("Expected Type %d, got %d", PDUFlowControl, pdu.Type)
	}
	if *pdu.FlowStatus != FlowStatusContinueToSend {
		t.Errorf("Expected FlowStatus %d, got %d", FlowStatusContinueToSend, *pdu.FlowStatus)
	}
	if *pdu.BlockSize != 10 {
		t.Errorf("Expected BlockSize 10, got %d", *pdu.BlockSize)
	}
	if *pdu.StMin != 5 {
		t.Errorf("Expected StMin 5, got %d", *pdu.StMin)
	}
	// StMinSeconds for 0x05 is 5ms -> 0.005s
	if *pdu.StMinSeconds != 0.005 {
		t.Errorf("Expected StMinSeconds 0.005, got %f", *pdu.StMinSeconds)
	}
}

// --- Fragmentation (Assembly) Tests ---

func TestTransportLayer_Fragmentation_MultiFrame(t *testing.T) {
	// We want to send 15 bytes.
	// Address config: Standard 8-byte frames.
	// 15 bytes > 7 bytes -> FF + CFs.
	// First Frame: 6 bytes payload (2 overhead).
	// Remaining: 9 bytes.
	// CF1: 7 bytes payload (1 overhead).
	// CF2: 2 bytes payload (1 overhead) + padding (if enabled? defaults seem to be no padding unless forced).

	addr := NewAddress(Normal11bits, 0x123, 0x321, 0x44, 0x55, 0, 0, 0, false, false)

	// Capture sent messages
	var sentMsgs []*CanMessage
	txfn := func(msg *CanMessage) {
		sentMsgs = append(sentMsgs, msg)
	}

	// Mock Rx to provide FlowControl when FF is sent
	// We need a channel to verify flow control is read
	fcProvided := false
	rxfn := func(timeout float64) *CanMessage {
		// In the first process loop where we sent FF, the state machine will switch to TxWaitFC.
		// The next call to Process (doRx=true) should pick up our FC.
		// Detailed synchronization might be tricky with pure function mocks,
		// but since Process() calls rxfn in a loop, we can just return it once.
		if !fcProvided && len(sentMsgs) == 1 { // FF sent
			fcProvided = true
			// Send FC: Continue, BS=0 (unlimited), StMin=0
			data := CraftFlowControlData(FlowStatusContinueToSend, 0, 0)
			// Add valid address header if necessary (address.GetRxArbitrationId)
			// For specific address matching, we need to correct ID.
			// Helper: addr.GetRxArbitrationId(Physical)
			return &CanMessage{
				ArbitrationId: addr.GetRxArbitrationId(Physical),
				Data:          data,
				Dlc:           8,
			}
		}
		return nil
	}

	params := NewParams()
	// No padding by default
	tll := NewTransportLayerLogic(rxfn, txfn, addr, nil, &params, nil)

	dataToSend := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15}
	if err := tll.Send(dataToSend, Physical, 0); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Step 1: Process -> Should generate FF
	// Loop until transmission is done
	maxLoops := 100
	for i := 0; i < maxLoops; i++ {
		tll.Process(0.001, true, true)
		if !tll.Transmitting() {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	if len(sentMsgs) != 3 {
		t.Fatalf("Expected 3 frames (FF, CF1, CF2), got %d", len(sentMsgs))
	}

	// Verify FF
	ff := sentMsgs[0]
	if ff.Data[0]&0xF0 != 0x10 {
		t.Errorf("Frame 0 should be FF (0x10), got %x", ff.Data[0])
	}
	// Length should be 15 (0x00F)
	if ff.Data[0]&0x0F != 0x00 || ff.Data[1] != 0x0F {
		t.Errorf("Frame 0 length incorrect")
	}

	// Verify CF1
	cf1 := sentMsgs[1]
	if cf1.Data[0] != 0x21 {
		t.Errorf("Frame 1 should be CF with Seq 1 (0x21), got %x", cf1.Data[0])
	}

	// Verify CF2
	cf2 := sentMsgs[2]
	if cf2.Data[0] != 0x22 {
		t.Errorf("Frame 2 should be CF with Seq 2 (0x22), got %x", cf2.Data[0])
	}
}

// --- Reassembly (Unpacking) Tests ---

func TestTransportLayer_Reassembly_MultiFrame(t *testing.T) {
	// Receive 15 bytes.
	// FF: Length 15. Bytes 1..6.
	// Logic should send FC.
	// CF1: Bytes 7..13.
	// CF2: Bytes 14..15.

	addr := NewAddress(Normal11bits, 0x123, 0x321, 0x00, 0x00, 0, 0, 0, false, false)

	rxMsgs := []*CanMessage{
		// FF: Len 15. Payload: 01 02 03 04 05 06
		{ArbitrationId: 0x321, Data: []byte{0x10, 0x0F, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}},
		// CF1: Seq 1. Payload: 07 08 09 10 11 12 13
		{ArbitrationId: 0x321, Data: []byte{0x21, 0x07, 0x08, 0x09, 0x10, 0x11, 0x12, 0x13}},
		// CF2: Seq 2. Payload: 14 15
		{ArbitrationId: 0x321, Data: []byte{0x22, 0x14, 0x15}},
	}

	msgIdx := 0
	rxfn := func(timeout float64) *CanMessage {
		if msgIdx < len(rxMsgs) {
			m := rxMsgs[msgIdx]
			msgIdx++
			return m
		}
		return nil
	}

	var sentFC []*CanMessage
	txfn := func(msg *CanMessage) {
		sentFC = append(sentFC, msg)
	}

	tll := NewTransportLayerLogic(rxfn, txfn, addr, nil, nil, nil)

	// Pump the process loop enough times to consume messages
	for i := 0; i < 10; i++ {
		tll.Process(0.001, true, true)
	}

	if len(sentFC) < 1 {
		t.Errorf("Expected at least 1 Flow Control sent, got %d", len(sentFC))
	} else {
		// Valid FC check
		if sentFC[0].Data[0]&0xF0 != 0x30 {
			t.Errorf("Expected FC frame, got %x", sentFC[0].Data)
		}
	}

	// Check received data
	data, ok := tll.Recv(false, 0)
	if !ok {
		t.Fatalf("Did not receive reassembled message")
	}
	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15}
	if !bytes.Equal(data, expected) {
		t.Errorf("Reassembled data mismatch.\nGot: %x\nWant: %x", data, expected)
	}
}
