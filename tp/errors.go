package tp

// messageOrDefault returns msg if present, otherwise fallback.
func messageOrDefault(msg, fallback string) string {
	if msg != "" {
		return msg
	}
	return fallback
}

type IsoTpError struct {
	msg string
}

func NewIsoTpError(msg string) IsoTpError {
	return IsoTpError{msg: msg}
}

func (e IsoTpError) Error() string {
	return messageOrDefault(e.msg, "ISO-TP error")
}

type BlockingSendFailure struct {
	IsoTpError
}

func (e BlockingSendFailure) Error() string {
	return messageOrDefault(e.msg, "blocking send failed to complete")
}

type BadGeneratorError struct {
	IsoTpError
}

func (e BadGeneratorError) Error() string {
	return messageOrDefault(e.msg, "generator size does not match data length")
}

type BlockingSendTimeout struct {
	BlockingSendFailure
}

func (e BlockingSendTimeout) Error() string {
	return messageOrDefault(e.msg, "blocking send timed out")
}

type FlowControlTimeoutError struct {
	IsoTpError
}

func (e FlowControlTimeoutError) Error() string {
	return messageOrDefault(e.msg, "flow control frame not sent in time")
}

type ConsecutiveFrameTimeoutError struct {
	IsoTpError
}

func (e ConsecutiveFrameTimeoutError) Error() string {
	return messageOrDefault(e.msg, "consecutive frame not sent in time")
}

type InvalidCanDataError struct {
	IsoTpError
}

func (e InvalidCanDataError) Error() string {
	return messageOrDefault(e.msg, "invalid CAN data received")
}

type UnexpectedFlowControlError struct {
	IsoTpError
}

func (e UnexpectedFlowControlError) Error() string {
	return messageOrDefault(e.msg, "unexpected flow control frame received")
}

type UnexpectedConsecutiveFrameError struct {
	IsoTpError
}

func (e UnexpectedConsecutiveFrameError) Error() string {
	return messageOrDefault(e.msg, "unexpected consecutive frame received")
}

type ReceptionInterruptedWithSingleFrameError struct {
	IsoTpError
}

func (e ReceptionInterruptedWithSingleFrameError) Error() string {
	return messageOrDefault(e.msg, "reception interrupted by a single frame")
}

type ReceptionInterruptedWithFirstFrameError struct {
	IsoTpError
}

func (e ReceptionInterruptedWithFirstFrameError) Error() string {
	return messageOrDefault(e.msg, "reception interrupted by a first frame")
}

type WrongSequenceNumberError struct {
	IsoTpError
}

func (e WrongSequenceNumberError) Error() string {
	return messageOrDefault(e.msg, "wrong sequence number in consecutive frame")
}

type UnsupportedWaitFrameError struct {
	IsoTpError
}

func (e UnsupportedWaitFrameError) Error() string {
	return messageOrDefault(e.msg, "wait flow control frame not supported")
}

type MaximumWaitFrameReachedError struct {
	IsoTpError
}

func (e MaximumWaitFrameReachedError) Error() string {
	return messageOrDefault(e.msg, "maximum wait flow control frames reached")
}

type FrameTooLongError struct {
	IsoTpError
}

func (e FrameTooLongError) Error() string {
	return messageOrDefault(e.msg, "first frame length exceeds maximum frame size")
}

type ChangingInvalidRXDLError struct {
	IsoTpError
}

func (e ChangingInvalidRXDLError) Error() string {
	return messageOrDefault(e.msg, "consecutive frame length smaller than RX_DL before final frame")
}

type MissingEscapeSequenceError struct {
	IsoTpError
}

func (e MissingEscapeSequenceError) Error() string {
	return messageOrDefault(e.msg, "missing escape sequence for single frame payload")
}

type InvalidCanFdFirstFrameRXDL struct {
	IsoTpError
}

func (e InvalidCanFdFirstFrameRXDL) Error() string {
	return messageOrDefault(e.msg, "invalid CAN-FD first frame RX_DL")
}

type OverflowError struct {
	IsoTpError
}

func (e OverflowError) Error() string {
	return messageOrDefault(e.msg, "remote node reported overflow")
}
