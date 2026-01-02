package tp

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// PDU represents a decoded ISO-TP protocol data unit extracted from a CAN message.
type PDU struct {
	Type           int
	Length         *int
	Data           []byte
	BlockSize      *int
	StMin          *int
	StMinSeconds   *float64
	SeqNum         *int
	FlowStatus     *int
	RxDL           int
	EscapeSequence bool
	CanDL          int
}

const (
	PDUSingleFrame = iota
	PDUFirstFrame
	PDUConsecutiveFrame
	PDUFlowControl
)

const (
	FlowStatusContinueToSend = iota
	FlowStatusWait
	FlowStatusOverflow
)

// NewPDU parses a CAN message into a PDU.
func NewPDU(msg CanMessage, startOfData int) (*PDU, error) {
	if len(msg.Data) < startOfData {
		return nil, fmt.Errorf("received message is missing data according to prefix size")
	}

	p := &PDU{
		Data:           []byte{},
		EscapeSequence: false,
		CanDL:          len(msg.Data),
		RxDL:           maxInt(8, len(msg.Data)),
	}

	msgData := msg.Data[startOfData:]
	dataLen := len(msgData)
	if dataLen == 0 {
		return nil, fmt.Errorf("empty CAN frame")
	}

	frameType := int((msgData[0] >> 4) & 0xF)
	if frameType > 3 {
		return nil, fmt.Errorf("received message with unknown frame type %d", frameType)
	}
	p.Type = frameType

	switch frameType {
	case PDUSingleFrame:
		lengthPlaceholder := int(msgData[0]) & 0xF
		if lengthPlaceholder != 0 {
			p.Length = intPtr(lengthPlaceholder)
			if *p.Length > dataLen-1 {
				return nil, fmt.Errorf("single frame length %d exceeds payload %d", *p.Length, dataLen-1)
			}
			p.Data = msgData[1 : 1+*p.Length]
		} else {
			if dataLen < 2 {
				return nil, fmt.Errorf("single frame with escape sequence must be at least %d bytes", 2+startOfData)
			}
			p.EscapeSequence = true
			length := int(msgData[1])
			if length == 0 {
				return nil, fmt.Errorf("received single frame with length of 0 bytes")
			}
			if length > dataLen-2 {
				return nil, fmt.Errorf("single frame length %d exceeds payload %d", length, dataLen-2)
			}
			p.Length = intPtr(length)
			p.Data = msgData[2 : 2+length]
		}

	case PDUFirstFrame:
		if dataLen < 2 {
			return nil, fmt.Errorf("first frame must be at least %d bytes", 2+startOfData)
		}
		lengthPlaceholder := ((int(msgData[0]) & 0xF) << 8) | int(msgData[1])
		if lengthPlaceholder != 0 {
			p.Length = intPtr(lengthPlaceholder)
			p.Data = msgData[2:minInt(dataLen, 2+*p.Length)]
		} else {
			if dataLen < 6 {
				return nil, fmt.Errorf("first frame with escape sequence must be at least %d bytes", 6+startOfData)
			}
			p.EscapeSequence = true
			length := (int(msgData[2]) << 24) | (int(msgData[3]) << 16) | (int(msgData[4]) << 8) | int(msgData[5])
			p.Length = intPtr(length)
			p.Data = msgData[6:minInt(dataLen, 6+length)]
		}

	case PDUConsecutiveFrame:
		seq := int(msgData[0]) & 0xF
		p.SeqNum = &seq
		p.Data = msgData[1:]

	case PDUFlowControl:
		if dataLen < 3 {
			return nil, fmt.Errorf("flow control frame must be at least %d bytes", 3+startOfData)
		}
		fs := int(msgData[0]) & 0xF
		if fs >= 3 {
			return nil, fmt.Errorf("unknown flow status")
		}
		p.FlowStatus = &fs
		bs := int(msgData[1])
		p.BlockSize = &bs
		stMinTemp := int(msgData[2])
		var stSec *float64
		if stMinTemp >= 0 && stMinTemp <= 0x7F {
			val := float64(stMinTemp) / 1000.0
			stSec = &val
		} else if stMinTemp >= 0xF1 && stMinTemp <= 0xF9 {
			val := float64(stMinTemp-0xF0) / 10000.0
			stSec = &val
		}
		if stSec == nil {
			return nil, fmt.Errorf("invalid StMin received in Flow Control")
		}
		p.StMinSeconds = stSec
		p.StMin = &stMinTemp
	default:
		return nil, fmt.Errorf("unsupported PDU type %d", p.Type)
	}

	return p, nil
}

func (p *PDU) Name() string {
	switch p.Type {
	case PDUSingleFrame:
		return "SINGLE_FRAME"
	case PDUFirstFrame:
		return "FIRST_FRAME"
	case PDUConsecutiveFrame:
		return "CONSECUTIVE_FRAME"
	case PDUFlowControl:
		return "FLOW_CONTROL"
	default:
		return "[None]"
	}
}

func CraftFlowControlData(flowStatus, blockSize, stMin int) []byte {
	return []byte{byte(0x30 | (flowStatus & 0xF)), byte(blockSize & 0xFF), byte(stMin & 0xFF)}
}

// RateLimiter limits outgoing bandwidth over a sliding window.
type RateLimiter struct {
	Enabled       bool
	MeanBitrate   float64
	WindowSizeSec float64
	ErrorReason   string
	burstBits     []int
	burstTime     []float64
	bitTotal      int
	windowBitMax  float64
	mu            sync.Mutex
}

func NewRateLimiter(meanBitrate float64, windowSizeSec float64) *RateLimiter {
	rl := &RateLimiter{
		MeanBitrate:   meanBitrate,
		WindowSizeSec: windowSizeSec,
	}
	rl.Reset()
	if rl.CanBeEnabled() {
		rl.Enable()
	}
	return rl
}

func (r *RateLimiter) CanBeEnabled() bool {
	if r.MeanBitrate <= 0 {
		r.ErrorReason = "mean_bitrate must be greater than 0"
		return false
	}
	if r.WindowSizeSec <= 0 {
		r.ErrorReason = "window_size_sec must be greater than 0"
		return false
	}
	return true
}

func (r *RateLimiter) Enable() {
	if !r.CanBeEnabled() {
		panic(fmt.Sprintf("cannot enable RateLimiter: %s", r.ErrorReason))
	}
	r.Enabled = true
	r.Reset()
}

func (r *RateLimiter) Disable() {
	r.Enabled = false
}

func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.burstBits = nil
	r.burstTime = nil
	r.bitTotal = 0
	r.windowBitMax = r.MeanBitrate * r.WindowSizeSec
}

func (r *RateLimiter) Update() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.Enabled {
		r.bitTotal = 0
		r.burstBits = nil
		r.burstTime = nil
		return
	}
	now := time.Since(time.Unix(0, 0)).Seconds()
	for len(r.burstTime) > 0 {
		if now-r.burstTime[0] > r.WindowSizeSec {
			removed := r.burstBits[0]
			r.burstBits = r.burstBits[1:]
			r.burstTime = r.burstTime[1:]
			r.bitTotal -= removed
		} else {
			break
		}
	}
}

func (r *RateLimiter) AllowedBytes() int {
	if !r.Enabled {
		return int(^uint32(0) >> 1)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	allowedBits := r.windowBitMax - float64(r.bitTotal)
	if allowedBits < 0 {
		allowedBits = 0
	}
	return int(math.Floor(allowedBits / 8.0))
}

func (r *RateLimiter) InformByteSent(dataLen int) {
	if !r.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Since(time.Unix(0, 0)).Seconds()
	bits := dataLen * 8
	if len(r.burstTime) == 0 || now-r.burstTime[len(r.burstTime)-1] > 0.005 {
		r.burstTime = append(r.burstTime, now)
		r.burstBits = append(r.burstBits, bits)
	} else {
		r.burstBits[len(r.burstBits)-1] += bits
	}
	r.bitTotal += bits
}

type RxState int

const (
	RxIdle RxState = iota
	RxWaitCF
)

type TxState int

const (
	TxIdle TxState = iota
	TxWaitFC
	TxTransmitCF
	TxTransmitSFStandby
	TxTransmitFFStandby
)

// Params mirrors the configurable parameters of the transport layer.
type Params struct {
	StMin                  int
	BlockSize              int
	OverrideReceiverStMin  *float64
	RxFlowControlTimeoutMs int
	RxConsecutiveTimeoutMs int
	TxPadding              *int
	WftMax                 int
	TxDataLength           int
	TxDataMinLength        *int
	MaxFrameSize           int
	CanFD                  bool
	BitrateSwitch          bool
	DefaultTargetType      uint32
	RateLimitMaxBitrate    int
	RateLimitWindowSize    float64
	RateLimitEnable        bool
	ListenMode             bool
	BlockingSend           bool
	WaitFunc               func(time.Duration)
}

func NewParams() Params {
	return Params{
		StMin:                  0,
		BlockSize:              8,
		OverrideReceiverStMin:  nil,
		RxFlowControlTimeoutMs: 1000,
		RxConsecutiveTimeoutMs: 1000,
		TxPadding:              nil,
		WftMax:                 0,
		TxDataLength:           8,
		TxDataMinLength:        nil,
		MaxFrameSize:           4095,
		CanFD:                  false,
		BitrateSwitch:          false,
		DefaultTargetType:      Physical,
		RateLimitMaxBitrate:    100000000,
		RateLimitWindowSize:    0.2,
		RateLimitEnable:        false,
		ListenMode:             false,
		BlockingSend:           false,
		WaitFunc:               func(d time.Duration) { time.Sleep(d) },
	}
}

func (p *Params) Validate() error {
	if p.RxFlowControlTimeoutMs < 0 {
		return fmt.Errorf("rx_flowcontrol_timeout must be positive integer")
	}
	if p.RxConsecutiveTimeoutMs < 0 {
		return fmt.Errorf("rx_consecutive_frame_timeout must be positive integer")
	}
	if p.TxPadding != nil {
		if *p.TxPadding < 0 || *p.TxPadding > 0xFF {
			return fmt.Errorf("tx_padding must be between 0x00 and 0xFF")
		}
	}
	if p.StMin < 0 || p.StMin > 0xFF {
		return fmt.Errorf("stmin must be between 0x00 and 0xFF")
	}
	if p.BlockSize < 0 || p.BlockSize > 0xFF {
		return fmt.Errorf("blocksize must be between 0x00 and 0xFF")
	}
	if p.OverrideReceiverStMin != nil {
		if *p.OverrideReceiverStMin < 0 || math.IsInf(*p.OverrideReceiverStMin, 0) || math.IsNaN(*p.OverrideReceiverStMin) {
			return fmt.Errorf("invalid override_receiver_stmin")
		}
	}
	validTxDataLengths := map[int]bool{8: true, 12: true, 16: true, 20: true, 24: true, 32: true, 48: true, 64: true}
	if !validTxDataLengths[p.TxDataLength] {
		return fmt.Errorf("tx_data_length must be one of 8, 12, 16, 20, 24, 32, 48, 64")
	}
	if p.TxDataMinLength != nil {
		validMin := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 12: true, 16: true, 20: true, 24: true, 32: true, 48: true, 64: true}
		if !validMin[*p.TxDataMinLength] {
			return fmt.Errorf("invalid tx_data_min_length")
		}
		if *p.TxDataMinLength > p.TxDataLength {
			return fmt.Errorf("tx_data_min_length cannot be greater than tx_data_length")
		}
	}
	if p.MaxFrameSize < 0 {
		return fmt.Errorf("max_frame_size must be positive")
	}
	if p.RateLimitMaxBitrate <= 0 {
		return fmt.Errorf("rate_limit_max_bitrate must be greater than 0")
	}
	if p.RateLimitWindowSize <= 0 {
		return fmt.Errorf("rate_limit_window_size must be greater than 0")
	}
	if float64(p.RateLimitMaxBitrate)*p.RateLimitWindowSize < float64(p.TxDataLength*8) {
		return fmt.Errorf("rate limiter too restrictive for a single frame")
	}
	return nil
}

// SendGenerator wraps a generator function with its total size.
type SendGenerator struct {
	Gen  ByteGenerator
	Size int
}

type SendRequest struct {
	Generator         *FiniteByteGenerator
	TargetAddressType uint32
	completeCh        chan bool
	Success           bool
}

func NewSendRequest(data interface{}, targetAddressType uint32) (*SendRequest, error) {
	var gen *FiniteByteGenerator
	switch v := data.(type) {
	case []byte:
		g, err := NewFiniteByteGenerator(sliceGenerator(v), len(v))
		if err != nil {
			return nil, err
		}
		gen = g
	case SendGenerator:
		g, err := NewFiniteByteGenerator(v.Gen, v.Size)
		if err != nil {
			return nil, err
		}
		gen = g
	default:
		return nil, fmt.Errorf("data must be []byte or SendGenerator")
	}

	return &SendRequest{
		Generator:         gen,
		TargetAddressType: targetAddressType,
		completeCh:        make(chan bool, 1),
	}, nil
}

func (s *SendRequest) Complete(success bool) {
	s.Success = success
	select {
	case s.completeCh <- success:
	default:
	}
}

type ProcessStats struct {
	Received          int
	ReceivedProcessed int
	Sent              int
	FrameReceived     int
}

type ProcessRxReport struct {
	ImmediateTxRequired bool
	FrameReceived       bool
}

type ProcessTxReport struct {
	Msg                 *CanMessage
	ImmediateRxRequired bool
}

type RxFn func(timeout float64) *CanMessage
type TxFn func(msg *CanMessage)
type PostSendCallback func(*SendRequest)
type ErrorHandler func(error)

// TransportLayerLogic implements the ISO-TP state machines.
type TransportLayerLogic struct {
	Params                 Params
	Logger                 *log.Logger
	RemoteBlockSize        *int
	rxfn                   RxFn
	txfn                   TxFn
	txQueue                *SafeQueue[*SendRequest]
	rxQueue                *SafeQueue[[]byte]
	txStandbyMsg           *CanMessage
	rxState                RxState
	txState                TxState
	lastRxState            RxState
	lastTxState            TxState
	rxBlockCounter         int
	lastSeqNum             int
	rxFrameLength          int
	txFrameLength          int
	lastFlowControlFrame   *PDU
	txBlockCounter         int
	txSeqNum               int
	wftCounter             int
	pendingFlowControlTx   bool
	pendingFlowControlStat int
	timerTxStMin           *Timer
	errorHandler           ErrorHandler
	actualRxDL             *int
	timings                map[[2]int]float64
	activeSendRequest      *SendRequest
	rxBuffer               []byte
	address                *Address
	timerRxFC              *Timer
	timerRxCF              *Timer
	rateLimiter            *RateLimiter
	blockingRxFn           bool
	postSendCallback       PostSendCallback
}

func NewTransportLayerLogic(rxfn RxFn, txfn TxFn, address *Address, errorHandler ErrorHandler, params *Params, postSendCb PostSendCallback) *TransportLayerLogic {
	p := NewParams()
	if params != nil {
		p = *params
	}
	if err := p.Validate(); err != nil {
		panic(err)
	}

	t := &TransportLayerLogic{
		Params:           p,
		Logger:           log.Default(),
		rxfn:             rxfn,
		txfn:             txfn,
		rxState:          RxIdle,
		txState:          TxIdle,
		lastRxState:      RxIdle,
		lastTxState:      TxIdle,
		errorHandler:     errorHandler,
		postSendCallback: postSendCb,
	}

	t.SetAddress(address)
	t.txQueue = NewSafeQueue[*SendRequest]()
	t.rxQueue = NewSafeQueue[[]byte]()
	t.rxBuffer = []byte{}
	t.timerTxStMin = NewTimer(0)
	t.loadParams()
	t.timings = map[[2]int]float64{
		{int(RxIdle), int(TxIdle)}:   0.02,
		{int(RxIdle), int(TxWaitFC)}: 0.005,
	}
	return t
}

func (t *TransportLayerLogic) loadParams() {
	t.timerRxFC = NewTimer(float64(t.Params.RxFlowControlTimeoutMs) / 1000.0)
	t.timerRxCF = NewTimer(float64(t.Params.RxConsecutiveTimeoutMs) / 1000.0)
	t.rateLimiter = NewRateLimiter(float64(t.Params.RateLimitMaxBitrate), t.Params.RateLimitWindowSize)
	if t.Params.RateLimitEnable {
		t.rateLimiter.Enable()
	} else {
		t.rateLimiter.Disable()
	}
}

// Send enqueues data for transmission. data must be []byte or SendGenerator.
func (t *TransportLayerLogic) Send(data interface{}, targetAddressType uint32, sendTimeout time.Duration) error {
	if t.Params.ListenMode {
		return errors.New("cannot transmit when listen_mode=true")
	}

	req, err := NewSendRequest(data, targetAddressType)
	if err != nil {
		return err
	}

	if targetAddressType == Functional {
		lengthBytes := 1
		if t.Params.TxDataLength != 8 {
			lengthBytes = 2
		}
		maxLen := t.Params.TxDataLength - lengthBytes - len(t.address.GetTxPayloadPrefix())
		if req.Generator.TotalLength() > maxLen {
			return fmt.Errorf("cannot send multi packet frame with Functional TargetAddressType")
		}
	}

	t.txQueue.Push(req)
	if t.postSendCallback != nil {
		t.postSendCallback(req)
	}

	if t.Params.BlockingSend {
		select {
		case <-req.completeCh:
			if !req.Success {
				return BlockingSendFailure{IsoTpError: NewIsoTpError("error while sending IsoTP frame")}
			}
		case <-time.After(sendTimeout):
			return BlockingSendTimeout{BlockingSendFailure{IsoTpError: NewIsoTpError("failed to send IsoTP frame in time")}}
		}
	}
	return nil
}

func (t *TransportLayerLogic) Recv(block bool, timeout time.Duration) ([]byte, bool) {
	if data, ok := t.rxQueue.Pop(); ok {
		return data, true
	}
	if !block {
		return nil, false
	}
	expire := time.Now().Add(timeout)
	for time.Now().Before(expire) {
		if data, ok := t.rxQueue.Pop(); ok {
			return data, true
		}
		time.Sleep(1 * time.Millisecond)
	}
	return nil, false
}

func (t *TransportLayerLogic) Available() bool {
	return t.rxQueue.Len() > 0
}

func (t *TransportLayerLogic) Transmitting() bool {
	return t.txQueue.Len() > 0 || t.txState != TxIdle
}

// Process runs one iteration of the RX/TX state machines.
func (t *TransportLayerLogic) Process(rxTimeout float64, doRx, doTx bool) ProcessStats {
	runProcess := true
	msgReceived := 0
	msgReceivedProcessed := 0
	msgSent := 0
	nbFrameReceived := 0

	for runProcess {
		runProcess = false
		var msg *CanMessage

		startWithTx := doTx && t.txQueue.Len() > 0 && t.rxState == RxIdle && t.txState == TxIdle
		if startWithTx {
			runProcess = true
		}

		if doRx && !startWithTx {
			firstLoop := true
			for msg != nil || firstLoop {
				firstLoop = false
				msg = t.rxfn(rxTimeout)
				t.checkTimeoutsRx()
				if msg != nil {
					msgReceived++
					if t.address.IsForMe(*msg) {
						msgReceivedProcessed++
						rxResult := t.processRx(*msg)
						if rxResult.FrameReceived {
							nbFrameReceived++
						}
						if rxResult.ImmediateTxRequired {
							break
						}
					}
				}
			}
		}

		startWithTx = false
		t.rateLimiter.Update()

		if doTx {
			firstLoop := true
			for msg != nil || firstLoop {
				firstLoop = false
				txResult := t.processTx()
				msg = txResult.Msg
				if msg != nil {
					msgSent++
					t.txfn(msg)
				}
				if txResult.ImmediateRxRequired {
					runProcess = true
					break
				}
			}
		}
		t.lastTxState = t.txState
		t.lastRxState = t.rxState
	}

	return ProcessStats{
		Received:          msgReceived,
		ReceivedProcessed: msgReceivedProcessed,
		Sent:              msgSent,
		FrameReceived:     nbFrameReceived,
	}
}

func (t *TransportLayerLogic) checkTimeoutsRx() {
	if t.timerRxCF.IsTimedOut() {
		t.triggerError(ConsecutiveFrameTimeoutError{})
		t.stopReceiving()
	}
}

func (t *TransportLayerLogic) processRx(msg CanMessage) ProcessRxReport {
	pdu, err := NewPDU(msg, t.address.GetRxPrefixSize())
	if err != nil {
		t.triggerError(InvalidCanDataError{IsoTpError: NewIsoTpError(err.Error())})
		t.stopReceiving()
		return ProcessRxReport{}
	}

	if pdu.Type == PDUFlowControl {
		t.lastFlowControlFrame = pdu
		return ProcessRxReport{ImmediateTxRequired: true}
	}

	if pdu.Type == PDUSingleFrame && pdu.CanDL > 8 && !pdu.EscapeSequence {
		t.triggerError(MissingEscapeSequenceError{})
		return ProcessRxReport{}
	}

	frameComplete := false
	immediateTx := false

	switch t.rxState {
	case RxIdle:
		t.rxFrameLength = 0
		t.timerRxCF.Stop()
		switch pdu.Type {
		case PDUSingleFrame:
			if pdu.Data != nil {
				frameComplete = true
				t.rxQueue.Push(append([]byte{}, pdu.Data...))
			}
		case PDUFirstFrame:
			started := t.startReceptionAfterFirstFrame(pdu)
			immediateTx = immediateTx || started
		case PDUConsecutiveFrame:
			t.triggerError(UnexpectedConsecutiveFrameError{})
		}
	case RxWaitCF:
		switch pdu.Type {
		case PDUSingleFrame:
			if pdu.Data != nil {
				frameComplete = true
				t.rxQueue.Push(append([]byte{}, pdu.Data...))
				t.rxState = RxIdle
				t.triggerError(ReceptionInterruptedWithSingleFrameError{})
			}
		case PDUFirstFrame:
			started := t.startReceptionAfterFirstFrame(pdu)
			immediateTx = immediateTx || started
			t.triggerError(ReceptionInterruptedWithFirstFrameError{})
		case PDUConsecutiveFrame:
			expected := (t.lastSeqNum + 1) & 0xF
			if pdu.SeqNum != nil && *pdu.SeqNum == expected {
				bytesToReceive := t.rxFrameLength - len(t.rxBuffer)
				if pdu.RxDL != t.getActualRxDL() && pdu.RxDL < bytesToReceive {
					t.triggerError(ChangingInvalidRXDLError{})
					return ProcessRxReport{}
				}
				t.startRxCFTimer()
				t.lastSeqNum = *pdu.SeqNum
				if bytesToReceive > len(pdu.Data) {
					t.appendRxData(pdu.Data)
				} else {
					t.appendRxData(pdu.Data[:bytesToReceive])
				}
				if len(t.rxBuffer) >= t.rxFrameLength {
					frameComplete = true
					t.rxQueue.Push(append([]byte{}, t.rxBuffer...))
					t.stopReceiving()
				} else {
					t.rxBlockCounter++
					if t.Params.BlockSize > 0 && (t.rxBlockCounter%t.Params.BlockSize) == 0 {
						t.requestTxFlowControl(FlowStatusContinueToSend)
						t.timerRxCF.Stop()
						immediateTx = true
					}
				}
			} else {
				t.stopReceiving()
				t.triggerError(WrongSequenceNumberError{})
			}
		}
	}

	if t.pendingFlowControlTx {
		immediateTx = true
	}
	return ProcessRxReport{ImmediateTxRequired: immediateTx, FrameReceived: frameComplete}
}

func (t *TransportLayerLogic) processTx() ProcessTxReport {
	var outputMsg *CanMessage
	allowedBytes := t.rateLimiter.AllowedBytes()

	if t.pendingFlowControlTx {
		t.pendingFlowControlTx = false
		if t.pendingFlowControlStat == FlowStatusContinueToSend {
			t.startRxCFTimer()
		}
		if !t.Params.ListenMode {
			msg := t.makeFlowControl(t.pendingFlowControlStat, nil, nil)
			return ProcessTxReport{Msg: msg, ImmediateRxRequired: true}
		}
	}

	flowControlFrame := t.lastFlowControlFrame
	t.lastFlowControlFrame = nil
	if flowControlFrame != nil {
		if flowControlFrame.FlowStatus != nil && *flowControlFrame.FlowStatus == FlowStatusOverflow {
			t.stopSending(false)
			t.triggerError(OverflowError{})
			return ProcessTxReport{}
		}
		if t.txState == TxIdle {
			t.triggerError(UnexpectedFlowControlError{}, true)
		} else {
			switch *flowControlFrame.FlowStatus {
			case FlowStatusWait:
				if t.Params.WftMax == 0 && !t.Params.ListenMode {
					t.triggerError(UnsupportedWaitFrameError{})
				} else if t.wftCounter >= t.Params.WftMax && !t.Params.ListenMode {
					t.triggerError(MaximumWaitFrameReachedError{})
					t.stopSending(false)
				} else {
					t.wftCounter++
					if t.txState == TxWaitFC || t.txState == TxTransmitCF {
						t.txState = TxWaitFC
						t.startRxFCTimer()
					}
				}
			case FlowStatusContinueToSend:
				if !t.timerRxFC.IsTimedOut() {
					t.wftCounter = 0
					t.timerRxFC.Stop()
					st := flowControlFrame.StMinSeconds
					if t.Params.OverrideReceiverStMin != nil {
						t.timerTxStMin.SetTimeout(*t.Params.OverrideReceiverStMin)
					} else if st != nil {
						t.timerTxStMin.SetTimeout(*st)
					}
					t.RemoteBlockSize = flowControlFrame.BlockSize
					if t.txState == TxWaitFC {
						t.txBlockCounter = 0
						t.timerTxStMin.Start()
					}
					t.txState = TxTransmitCF
				}
			default:
				panic("unhandled default case")
			}
		}
	}

	if t.timerRxFC.IsTimedOut() {
		t.triggerError(FlowControlTimeoutError{})
		t.stopSending(false)
	}

	if t.txState != TxIdle {
		if t.activeSendRequest != nil && t.activeSendRequest.Generator.Depleted() && t.txStandbyMsg == nil {
			t.stopSending(true)
		}
	}

	immediateRx := false

	switch t.txState {
	case TxIdle:
		for t.txQueue.Len() > 0 {
			req, _ := t.txQueue.Pop()
			t.activeSendRequest = req

			if req.Generator.Depleted() {
				req.Complete(true)
				continue
			}

			sizeOnFirstByte := (req.Generator.RemainingSize() + len(t.address.GetTxPayloadPrefix())) <= 7
			sizeOffset := 2
			if sizeOnFirstByte {
				sizeOffset = 1
			}
			totalSize := req.Generator.TotalLength()

			if totalSize <= t.Params.TxDataLength-sizeOffset-len(t.address.GetTxPayloadPrefix()) {
				payload, err := req.Generator.Consume(totalSize, true)
				if err != nil {
					t.triggerError(err)
					t.stopSending(false)
					break
				}
				msgData := append([]byte{}, t.address.GetTxPayloadPrefix()...)
				if sizeOnFirstByte {
					msgData = append(msgData, byte(len(payload)))
				} else {
					msgData = append(msgData, 0x0, byte(len(payload)))
				}
				msgData = append(msgData, payload...)
				arbitrationID := t.address.GetTxArbitrationId(req.TargetAddressType)
				msgTemp := t.makeTxMsg(arbitrationID, msgData)
				if len(msgData) > allowedBytes {
					t.txStandbyMsg = msgTemp
					t.txState = TxTransmitSFStandby
				} else {
					outputMsg = msgTemp
					t.stopSending(true)
				}
			} else {
				t.txFrameLength = totalSize
				encodeOnTwoBytes := t.txFrameLength <= 0xFFF
				var payload []byte
				var err error
				msgData := append([]byte{}, t.address.GetTxPayloadPrefix()...)
				if encodeOnTwoBytes {
					dataLength := t.Params.TxDataLength - 2 - len(t.address.GetTxPayloadPrefix())
					payload, err = req.Generator.Consume(dataLength, true)
					if err != nil {
						t.triggerError(err)
						t.stopSending(false)
						break
					}
					msgData = append(msgData, byte(0x10|((t.txFrameLength>>8)&0xF)), byte(t.txFrameLength&0xFF))
				} else {
					dataLength := t.Params.TxDataLength - 6 - len(t.address.GetTxPayloadPrefix())
					payload, err = req.Generator.Consume(dataLength, true)
					if err != nil {
						t.triggerError(err)
						t.stopSending(false)
						break
					}
					msgData = append(msgData, 0x10, 0x00, byte((t.txFrameLength>>24)&0xFF), byte((t.txFrameLength>>16)&0xFF), byte((t.txFrameLength>>8)&0xFF), byte(t.txFrameLength&0xFF))
				}
				msgData = append(msgData, payload...)
				arbitrationID := t.address.GetTxArbitrationId(req.TargetAddressType)
				t.txSeqNum = 1
				msgTemp := t.makeTxMsg(arbitrationID, msgData)
				if len(msgData) <= allowedBytes {
					outputMsg = msgTemp
					t.txState = TxWaitFC
					t.startRxFCTimer()
				} else {
					t.txStandbyMsg = msgTemp
					t.txState = TxTransmitFFStandby
				}
			}
			break
		}
	case TxTransmitSFStandby, TxTransmitFFStandby:
		if t.txStandbyMsg != nil && len(t.txStandbyMsg.Data) <= allowedBytes {
			outputMsg = t.txStandbyMsg
			t.txStandbyMsg = nil
			if t.txState == TxTransmitFFStandby {
				t.startRxFCTimer()
				t.txState = TxWaitFC
			} else {
				t.txState = TxIdle
			}
		}
	case TxWaitFC:
	case TxTransmitCF:
		if t.RemoteBlockSize == nil {
			break
		}
		if t.timerTxStMin.IsTimedOut() && t.activeSendRequest != nil {
			dataLength := t.Params.TxDataLength - 1 - len(t.address.GetTxPayloadPrefix())
			payloadLength := minInt(dataLength, t.activeSendRequest.Generator.RemainingSize())
			if payloadLength <= allowedBytes {
				payload, err := t.activeSendRequest.Generator.Consume(payloadLength, false)
				if err != nil {
					t.triggerError(err)
					t.stopSending(false)
					break
				}
				if len(payload) > 0 {
					msgData := append([]byte{}, t.address.GetTxPayloadPrefix()...)
					msgData = append(msgData, byte(0x20|t.txSeqNum))
					msgData = append(msgData, payload...)
					arbitrationID := t.address.GetTxArbitrationId(t.activeSendRequest.TargetAddressType)
					outputMsg = t.makeTxMsg(arbitrationID, msgData)
					t.txSeqNum = (t.txSeqNum + 1) & 0xF
					t.timerTxStMin.Start()
					t.txBlockCounter++
				}

				if t.activeSendRequest.Generator.Depleted() {
					if t.activeSendRequest.Generator.RemainingSize() > 0 {
						t.triggerError(BadGeneratorError{IsoTpError: NewIsoTpError("generator depleted before reaching specified size")})
						t.stopSending(false)
					} else {
						t.stopSending(true)
					}
				} else if *t.RemoteBlockSize != 0 && t.txBlockCounter >= *t.RemoteBlockSize {
					t.txState = TxWaitFC
					immediateRx = true
					t.startRxFCTimer()
				}
			}
		}
	}

	if outputMsg != nil {
		t.rateLimiter.InformByteSent(len(outputMsg.Data))
	}

	return ProcessTxReport{Msg: outputMsg, ImmediateRxRequired: immediateRx}
}

func (t *TransportLayerLogic) SetSleepTiming(idle, waitFC float64) {
	t.timings[[2]int{int(RxIdle), int(TxIdle)}] = idle
	t.timings[[2]int{int(RxIdle), int(TxWaitFC)}] = waitFC
}

func (t *TransportLayerLogic) SetAddress(address *Address) {
	if address == nil {
		panic("address must be provided")
	}
	t.address = address
}

func (t *TransportLayerLogic) padMessageData(msgData []byte) []byte {
	mustPad := false
	paddingByte := byte(0xCC)
	if t.Params.TxPadding != nil {
		paddingByte = byte(*t.Params.TxPadding)
	}

	var targetLength int
	if t.Params.TxDataLength == 8 {
		if t.Params.TxDataMinLength == nil {
			if t.Params.TxPadding != nil {
				mustPad = true
				targetLength = 8
			}
		} else {
			mustPad = true
			targetLength = *t.Params.TxDataMinLength
		}
	} else {
		if t.Params.TxDataMinLength == nil {
			targetLength = t.getNearestCanFdSize(len(msgData))
			mustPad = true
		} else {
			mustPad = true
			targetLength = maxInt(*t.Params.TxDataMinLength, t.getNearestCanFdSize(len(msgData)))
		}
	}

	if mustPad && len(msgData) < targetLength {
		pad := make([]byte, targetLength-len(msgData))
		for i := range pad {
			pad[i] = paddingByte
		}
		return append(msgData, pad...)
	}
	return msgData
}

func (t *TransportLayerLogic) startRxFCTimer() {
	t.timerRxFC = NewTimer(float64(t.Params.RxFlowControlTimeoutMs) / 1000.0)
	t.timerRxFC.Start()
}

func (t *TransportLayerLogic) startRxCFTimer() {
	t.timerRxCF = NewTimer(float64(t.Params.RxConsecutiveTimeoutMs) / 1000.0)
	t.timerRxCF.Start()
}

func (t *TransportLayerLogic) appendRxData(data []byte) {
	t.rxBuffer = append(t.rxBuffer, data...)
}

func (t *TransportLayerLogic) requestTxFlowControl(status int) {
	t.pendingFlowControlTx = true
	t.pendingFlowControlStat = status
}

func (t *TransportLayerLogic) stopSendingFlowControl() {
	t.pendingFlowControlTx = false
	t.lastFlowControlFrame = nil
}

func (t *TransportLayerLogic) makeTxMsg(arbitrationID int, data []byte) *CanMessage {
	data = t.padMessageData(data)
	return &CanMessage{
		ArbitrationId: arbitrationID,
		Dlc:           t.getDlc(data, true),
		Data:          data,
		ExtendedId:    t.address.IsTx29Bit(),
		IsFd:          t.Params.CanFD,
		BitrateSwitch: t.Params.BitrateSwitch,
	}
}

func (t *TransportLayerLogic) getDlc(data []byte, validateTx bool) int {
	fdlen := t.getNearestCanFdSize(len(data))
	if validateTx {
		if t.Params.TxDataLength == 8 {
			if fdlen < 2 || fdlen > 8 {
				panic(fmt.Sprintf("impossible DLC size for payload %d bytes", len(data)))
			}
		}
	}
	switch {
	case fdlen >= 2 && fdlen <= 8:
		return fdlen
	case fdlen == 12:
		return 9
	case fdlen == 16:
		return 10
	case fdlen == 20:
		return 11
	case fdlen == 24:
		return 12
	case fdlen == 32:
		return 13
	case fdlen == 48:
		return 14
	case fdlen == 64:
		return 15
	}
	panic(fmt.Sprintf("impossible DLC for payload size %d", len(data)))
}

func (t *TransportLayerLogic) getNearestCanFdSize(size int) int {
	switch {
	case size <= 8:
		return size
	case size <= 12:
		return 12
	case size <= 16:
		return 16
	case size <= 20:
		return 20
	case size <= 24:
		return 24
	case size <= 32:
		return 32
	case size <= 48:
		return 48
	case size <= 64:
		return 64
	default:
		panic(fmt.Sprintf("impossible data size for CAN FD: %d", size))
	}
}

func (t *TransportLayerLogic) makeFlowControl(flowStatus int, blockSize *int, stMin *int) *CanMessage {
	bs := t.Params.BlockSize
	if blockSize != nil {
		bs = *blockSize
	}
	st := t.Params.StMin
	if stMin != nil {
		st = *stMin
	}
	data := CraftFlowControlData(flowStatus, bs, st)
	return t.makeTxMsg(t.address.GetTxArbitrationId(Physical), append(t.address.GetTxPayloadPrefix(), data...))
}

func (t *TransportLayerLogic) StopSending() {
	t.stopSending(false)
}

func (t *TransportLayerLogic) stopSending(success bool) {
	if t.activeSendRequest != nil {
		t.activeSendRequest.Complete(success)
		t.activeSendRequest = nil
	}
	t.txState = TxIdle
	t.txFrameLength = 0
	t.timerRxFC.Stop()
	t.timerTxStMin.Stop()
	t.RemoteBlockSize = nil
	t.txBlockCounter = 0
	t.txSeqNum = 0
	t.wftCounter = 0
	t.txStandbyMsg = nil
}

func (t *TransportLayerLogic) StopReceiving() {
	t.stopReceiving()
}

func (t *TransportLayerLogic) stopReceiving() {
	t.actualRxDL = nil
	t.rxState = RxIdle
	t.rxBuffer = []byte{}
	t.stopSendingFlowControl()
	t.timerRxCF.Stop()
}

func (t *TransportLayerLogic) ClearRxQueue() {
	t.rxQueue.Clear()
}

func (t *TransportLayerLogic) ClearTxQueue() {
	t.txQueue.Clear()
}

func (t *TransportLayerLogic) startReceptionAfterFirstFrame(pdu *PDU) bool {
	if pdu.Length == nil {
		return false
	}
	if pdu.RxDL != 8 && pdu.RxDL != 12 && pdu.RxDL != 16 && pdu.RxDL != 20 && pdu.RxDL != 24 && pdu.RxDL != 32 && pdu.RxDL != 48 && pdu.RxDL != 64 {
		t.triggerError(InvalidCanFdFirstFrameRXDL{})
		t.stopReceiving()
		return false
	}

	t.actualRxDL = &pdu.RxDL
	started := false
	if *pdu.Length > t.Params.MaxFrameSize {
		t.triggerError(FrameTooLongError{})
		t.requestTxFlowControl(FlowStatusOverflow)
		t.rxState = RxIdle
	} else {
		t.rxState = RxWaitCF
		t.rxFrameLength = *pdu.Length
		t.appendRxData(pdu.Data)
		t.requestTxFlowControl(FlowStatusContinueToSend)
		t.startRxCFTimer()
		started = true
	}

	t.lastSeqNum = 0
	t.rxBlockCounter = 0
	return started
}

func (t *TransportLayerLogic) triggerError(err error, inhibitInListenMode ...bool) {
	inhibit := len(inhibitInListenMode) > 0 && inhibitInListenMode[0]
	if t.errorHandler != nil && !(inhibit && t.Params.ListenMode) {
		t.errorHandler(err)
	}
}

func (t *TransportLayerLogic) Reset() {
	t.ClearRxQueue()
	t.ClearTxQueue()
	t.stopSending(false)
	t.stopReceiving()
	t.rateLimiter.Reset()
}

func (t *TransportLayerLogic) SleepTime() float64 {
	key := [2]int{int(t.rxState), int(t.txState)}
	if v, ok := t.timings[key]; ok {
		return v
	}
	return 0.001
}

func (t *TransportLayerLogic) IsTxThrottled() bool {
	return t.txState == TxTransmitSFStandby || t.txState == TxTransmitFFStandby
}

func (t *TransportLayerLogic) IsRxActive() bool {
	return t.rxState != RxIdle
}

func (t *TransportLayerLogic) IsTxTransmittingCF() bool {
	return t.txState == TxTransmitCF
}

func (t *TransportLayerLogic) NextCFDelay() *float64 {
	if !t.IsTxTransmittingCF() {
		return nil
	}
	if t.timerTxStMin.IsTimedOut() {
		zero := 0.0
		return &zero
	}
	remaining := t.timerTxStMin.Remaining()
	return &remaining
}

func (t *TransportLayerLogic) getActualRxDL() int {
	if t.actualRxDL == nil {
		return 0
	}
	return *t.actualRxDL
}

func sliceGenerator(data []byte) ByteGenerator {
	index := 0
	return func() (byte, bool) {
		if index >= len(data) {
			return 0, false
		}
		b := data[index]
		index++
		return b, true
	}
}

func intPtr(v int) *int { return &v }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
