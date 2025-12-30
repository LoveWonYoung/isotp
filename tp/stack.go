package tp

import (
	"context"
	"fmt"
	"time"
)

// Transport 是ISOTP协议栈的核心结构
type Transport struct {
	address       *Address
	IsFD          bool
	MaxDataLength int
	rxState       State
	txState       State
	rxBuffer      []byte
	txBuffer      []byte

	// Channels replaced queues
	rxDataChan chan []byte
	txDataChan chan txRequest

	rxFrameLen           int
	txFrameLen           int
	rxSeqNum             int
	txSeqNum             int
	rxBlockCounter       int
	txBlockCounter       int
	remoteBlocksize      int
	remoteStmin          time.Duration
	lastFlowControlFrame *FlowControlFrame
	pendingFlowControlTx bool
	currentTxAddrType    AddressType

	// Native timers
	timerRxCF    *time.Timer
	timerRxFC    *time.Timer
	timerTxSTmin *time.Timer

	// Configuration
	config Config

	// Runtime State
	wftCounter int

	// Error Channel
	ErrorChan chan error
}

type txRequest struct {
	payload  []byte
	addrType AddressType
}

func NewTransport(address *Address, cfg Config) *Transport {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		// Since NewTransport doesn't return error, we should probably panic or log?
		// For now, let's just proceed as we can't change signature easily without breaking API.
		// In a major version bump, we should return (*Transport, error).
		fmt.Println("Warning: Invalid ISOTP Config:", err)
	}

	t := &Transport{
		address:       address,
		rxDataChan:    make(chan []byte, 10), // Buffer size can be tuned
		txDataChan:    make(chan txRequest, 10),
		IsFD:          false,
		MaxDataLength: 8,
		// Initialize timers with config values, but stopped
		timerRxCF:    time.NewTimer(time.Hour),
		timerRxFC:    time.NewTimer(time.Hour),
		timerTxSTmin: time.NewTimer(time.Hour),
		config:       cfg,
		ErrorChan:    make(chan error, 10),
	}
	t.timerRxCF.Stop()
	t.timerRxFC.Stop()
	t.timerTxSTmin.Stop()

	t.stopReceiving()
	t.stopSending()
	return t
}

func (t *Transport) SetFDMode(isFD bool) {
	t.IsFD = isFD
	if isFD {
		t.MaxDataLength = 64
	} else {
		t.MaxDataLength = 8
	}
}

// Send sends data with physical addressing. It might block if the send buffer is full.
func (t *Transport) Send(data []byte) {
	t.txDataChan <- txRequest{payload: data, addrType: Physical}
}

// SendWithAddressType sends data with the specified addressing type (physical/functional).
// Functional addressing is typically used for broadcast requests like 0x7DF.
func (t *Transport) SendWithAddressType(data []byte, addrType AddressType) {
	t.txDataChan <- txRequest{payload: data, addrType: addrType}
}

// Recv receives data. It matches the old signature but now pulls from channel.
func (t *Transport) Recv() ([]byte, bool) {
	select {
	case data := <-t.rxDataChan:
		return data, true
	default:
		return nil, false
	}
}

// Run starts the protocol stack event loop.
func (t *Transport) Run(ctx context.Context, rxChan <-chan CanMessage, txChan chan<- CanMessage) {
	// Delegate to the optimized loop that gates tx reads on state and guards timers.
	t.RunEventLoop(ctx, rxChan, txChan)
}

// RunOptimized is the loop with proper state handling
func (t *Transport) RunEventLoop(ctx context.Context, rxChan <-chan CanMessage, txChan chan<- CanMessage) {
	defer t.cleanup()

	for {
		var txDataEnable <-chan txRequest
		if t.txState == StateIdle {
			txDataEnable = t.txDataChan
		}

		select {
		case <-ctx.Done():
			return

		case msg := <-rxChan:
			t.ProcessRx(msg, txChan)

		case req := <-txDataEnable:
			t.startTransmission(req, txChan)

		case <-t.timerRxCF.C:
			fmt.Println("接收连续帧超时，重置接收状态。")
			t.stopReceiving()

		case <-t.timerRxFC.C:
			fmt.Println("等待流控帧超时，停止发送。")
			t.stopSending()

		case <-t.timerTxSTmin.C:
			if t.txState == StateTransmit {
				t.handleTxTransmit(txChan)
			}
		}
	}
}

func (t *Transport) cleanup() {
	t.timerRxCF.Stop()
	t.timerRxFC.Stop()
	t.timerTxSTmin.Stop()
}

func (t *Transport) startTransmission(req txRequest, txChan chan<- CanMessage) {
	// ... Logic from handleTxIdle ...
	// Requires refactoring handleTxIdle to direct action instead of returning msg
	t.initiateTx(req, txChan)
}

// Internal helpers
func (t *Transport) stopReceiving() {
	t.rxState = StateIdle
	t.rxBuffer = nil
	t.rxFrameLen = 0
	t.rxSeqNum = 0
	t.rxBlockCounter = 0
	if !t.timerRxCF.Stop() {
		// drain channel if needed
		select {
		case <-t.timerRxCF.C:
		default:
		}
	}
}

func (t *Transport) stopSending() {
	t.txState = StateIdle
	t.txBuffer = nil
	t.txFrameLen = 0
	t.txSeqNum = 0
	t.txBlockCounter = 0
	t.remoteBlocksize = 0
	t.remoteStmin = 0
	t.wftCounter = 0
	if !t.timerRxFC.Stop() {
		select {
		case <-t.timerRxFC.C:
		default:
		}
	}
	if !t.timerTxSTmin.Stop() {
		select {
		case <-t.timerTxSTmin.C:
		default:
		}
	}
}

func (t *Transport) makeTxMsg(data []byte, addrType AddressType) CanMessage {
	arbitrationID := t.address.GetTxArbitrationID(addrType)
	fullPayload := append(t.address.TxPayloadPrefix, data...)

	// Padding
	if t.config.PaddingByte != nil || t.config.TxDataMinLength > 0 {
		targetLen := 0

		if t.config.TxDataMinLength > 0 {
			targetLen = t.config.TxDataMinLength
		}

		// Backward compatibility / Standard behavior: if PaddingByte is set but TxDataMinLength is 0
		// we likely want to pad to 8 (Classic CAN) or maybe DLC-based (CAN FD).
		// Currently in Python-CAN-ISOTP:
		// If tx_data_min_length is set, it pads to that.
		// If tx_padding is set, it pads to tx_data_length (which defaults to 8).
		// So if TxDataMinLength is not set, but PaddingByte is, implementation implies we pad to 'MaxDataLength' or just 8?
		// Original logic was: if PaddingByte != nil, pad to 8 (or DLC for FD).
		if targetLen == 0 && t.config.PaddingByte != nil {
			if t.IsFD {
				// FD handling logic
				if len(fullPayload) < 8 {
					targetLen = 8
				} else {
					targetLen = len(fullPayload)
				}
			} else {
				// Classic CAN
				targetLen = 8
			}
		}

		if len(fullPayload) < targetLen {
			paddingVal := byte(0xCC) // Default padding if not specified? No, code says *PaddingByte
			if t.config.PaddingByte != nil {
				paddingVal = *t.config.PaddingByte
			} else {
				// If TxDataMinLength is set but PaddingByte is nil, Python implementation uses 0xCC or 0x00?
				// Actually Python says "tx_padding: Integer to use for padding. If None, no padding is used."
				// But if tx_data_min_length is set, we MUST pad.
				// If PaddingByte is nil here, we should probably default to something (0xCC is common in automotive).
				paddingVal = 0xCC
			}

			padding := make([]byte, targetLen-len(fullPayload))
			for i := range padding {
				padding[i] = paddingVal
			}
			fullPayload = append(fullPayload, padding...)
		}
	}

	return CanMessage{
		ArbitrationID: arbitrationID,
		Data:          fullPayload,
		IsExtendedID:  t.address.Is29Bit(),
		IsFD:          t.IsFD,
	}
}

// fireError sends an error to the ErrorChan. Non-blocking.
func (t *Transport) fireError(err error) {
	select {
	case t.ErrorChan <- err:
	default:
		// Channel full, drop error or maybe log to stdlib log?
		// For now we just drop to prevent blocking the stack.
		fmt.Println("ISOTP Error (Chan Full):", err)
	}
}
