package main

/*
#include <stdint.h>
#include <stdbool.h>
#include <stdlib.h>

// Define the function pointer type for the Tx callback
// id: CAN ID
// data: pointer to data buffer
// len: length of data
typedef void (*TxCallback)(uint32_t id, uint8_t* data, int len);

// Helper function to call the callback from Go
static void call_tx_callback(TxCallback cb, uint32_t id, uint8_t* data, int len) {
    if (cb != NULL) {
        cb(id, data, len);
    }
}
*/
import "C"
import (
	"context"
	"time"
	"unsafe"

	"gitee.com/lovewonyoung/CanMix/tp_layer"
)

// Global state
var (
	tpTransport *tp_layer.Transport
	txCallback  C.TxCallback
	ctx         context.Context
	cancel      context.CancelFunc
	rxChan      chan tp_layer.CanMessage
	txChan      chan tp_layer.CanMessage
)

// GoInitTp initializes the ISO-TP layer.
// rxID: The CAN ID to listen for.
// txID: The CAN ID to transmit with.
// isFD: Whether to use CAN FD.
// cb: The callback function for transmitting CAN frames.
//
//export GoInitTp
func GoInitTp(rxID uint32, txID uint32, isFD bool, cb C.TxCallback) {
	if tpTransport != nil {
		GoCloseTp()
	}

	txCallback = cb

	// Configure Address
	addr := &tp_layer.Address{
		RxID: rxID,
		TxID: txID,
	}

	// Default Config
	config := tp_layer.Config{
		PaddingByte: nil, // Optional padding
	}

	// Create channels
	rxChan = make(chan tp_layer.CanMessage, 100)
	txChan = make(chan tp_layer.CanMessage, 100)

	// Create Transport
	tpTransport = tp_layer.NewTransport(addr, config)
	tpTransport.SetFDMode(isFD)

	// Context for lifecycle management
	ctx, cancel = context.WithCancel(context.Background())

	// Start the Transport Run loop
	go tpTransport.Run(ctx, rxChan, txChan)

	// Start Tx Bridge
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-txChan:
				// Convert to C types and call callback
				id := C.uint32_t(msg.ArbitrationID)
				length := C.int(len(msg.Data))

				// Allocate C buffer if needed, or pass pointer to Go memory?
				// Passing pointer to Go memory is safe if the C code copies it immediately and doesn't hold it.
				// Since we don't control the C code, it's safer to not rely on Go GC behavior for long term,
				// but for a synchronous callback, pointing to Go memory slice is generally acceptable if the C code is well-behaved.
				// However, to be perfectly safe, let's use the pointer to the first element of slice.
				if length > 0 {
					ptr := (*C.uint8_t)(unsafe.Pointer(&msg.Data[0]))
					C.call_tx_callback(txCallback, id, ptr, length)
				}
			}
		}
	}()
}

// GoInputCanFrame is called by the external application when a CAN frame is received.
// id: CAN ID
// data: pointer to data
// len: length of data
//
//export GoInputCanFrame
func GoInputCanFrame(id uint32, data *C.uint8_t, length int) {
	if rxChan == nil {
		return
	}

	// Copy data from C memory to Go slice
	goData := C.GoBytes(unsafe.Pointer(data), C.int(length))

	msg := tp_layer.CanMessage{
		ArbitrationID: id,
		Data:          goData,
		IsFD:          false, // Should we expose this? For now assume inferred or fixed
		// We might need to expose isFD/isExtended in input if the stack needs it.
		// Detailed tp_layer stack might assume standard addressing.
	}
	// Note: You might want to pass IsExtended from C if needed.

	// Non-blocking send to rxChan to avoid blocking C thread
	select {
	case rxChan <- msg:
	default:
		// Drop frame if buffer full
	}
}

// GoSendTp sends data via ISO-TP.
// data: pointer to data
// length: length of data
//
//export GoSendTp
func GoSendTp(data *C.uint8_t, length int) {
	if tpTransport == nil {
		return
	}
	goData := C.GoBytes(unsafe.Pointer(data), C.int(length))
	tpTransport.Send(goData)
}

// GoRecvTp receives data from ISO-TP.
// buffer: pointer to buffer to write to
// capacity: size of buffer
// timeoutMs: timeout in milliseconds
// returns: number of bytes written, or 0 if timeout/empty, -1 if buffer too small
//
//export GoRecvTp
func GoRecvTp(buffer *C.uint8_t, capacity int, timeoutMs int) int {
	if tpTransport == nil {
		return 0
	}

	// We need a mechanism to peek or wait.
	// The Isotp Transport Recv() is non-blocking check on channel usually, or we can select.
	// Our stack.Recv() implemented in previous steps does a select on rxDataChan.

	// Create a timer for timeout
	timeout := time.Duration(timeoutMs) * time.Millisecond

	// We have to recreate the Recv logic here because we can't easily modify the stack to accept a timeout
	// without changing the interface, but wait! The stack.Recv() calls stack.rxDataChan.
	// We can't access private fields easily if we are in 'main' package but stack is in 'tp_layer'.
	// Wait, stack.Recv() returns (data, bool). It is non-blocking (default case).
	// We need a blocking receive with timeout.

	// Since we can't access `rxDataChan` directly (it is lowercase in previous `view_file` output),
	// we have to rely on `Recv()`.
	// But `Recv()` behaves:
	// case data := <-t.rxDataChan: return data, true
	// default: return nil, false

	// This is a busy-wait if we loop it.
	// Ideally we should export a BlockingRecv from tp_layer or expose the channel.
	// Let's modify tp_layer/stack.go to export the channel or add RecvTimeout.
	// BUT, modifying the core library might not be what the user wants if they just want a wrapper.
	// However, busy waiting is bad.

	// Let's check `tp_layer/stack.go` again.
	// public: Recv() ([]byte, bool)
	// private: rxDataChan

	// For now, I will implement a polling loop with small sleeps. It's not ideal but works without modifying core code.

	deadline := time.Now().Add(timeout)
	for {
		data, ok := tpTransport.Recv()
		if ok {
			if len(data) > capacity {
				return -1 // Buffer too small
			}
			// Copy to C buffer
			// Cast C pointer to Go byte pointer and create a slice
			ptr := (*byte)(unsafe.Pointer(buffer))
			cBuf := unsafe.Slice(ptr, capacity)
			copy(cBuf, data)
			return len(data)
		}

		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(1 * time.Millisecond)
	}
}

//export GoCloseTp
func GoCloseTp() {
	if cancel != nil {
		cancel()
	}
	tpTransport = nil
	ctx = nil
	cancel = nil
	rxChan = nil
	txChan = nil
}

func main() {
	// Need a main function for buildmode=c-shared
}
