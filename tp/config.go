package tp

import "time"

// Config defines the configuration for the ISO-TP Transport.
type Config struct {
	// PaddingByte, if not nil, is used to pad frames to declared length (8 or 64).
	PaddingByte *byte

	// Transmitter Side Timeouts
	TimeoutN_As time.Duration // Time for transmission of N_PDU on sender side
	TimeoutN_Bs time.Duration // Time until reception of FlowControl
	TimeoutN_Cs time.Duration // Time until transmission of next CF

	// Receiver Side Timeouts
	TimeoutN_Ar time.Duration // Time for transmission of N_PDU on receiver side
	TimeoutN_Br time.Duration // Time until transmission of FlowControl
	TimeoutN_Cr time.Duration // Time until reception of next CF

	// Parameters
	BlockSize int
	StMin     int

	// MaxWaitFrame (WFTMax) defines the maximum number of Flow Control Wait frames allowed.
	// 0 means not allowed (if strict) or default behavior.
	MaxWaitFrame int

	// TxDataMinLength forces the transmitted data length to be at least this value.
	// Used for padding short frames (e.g. to 8 bytes for CAN 2.0).
	// If 0, it means no forced minimum length (except what's required by protocol).
	TxDataMinLength int
}

// DefaultConfig returns the standard ISO-15765-2 default messages.
func DefaultConfig() Config {
	return Config{
		PaddingByte: nil, // No padding by default

		// Default Timeouts (ISO 15765-2 recommended values)
		TimeoutN_As: 1000 * time.Millisecond,
		TimeoutN_Bs: 1000 * time.Millisecond,
		TimeoutN_Cs: 1000 * time.Millisecond,

		TimeoutN_Ar: 1000 * time.Millisecond,
		TimeoutN_Br: 1000 * time.Millisecond,
		TimeoutN_Cr: 1000 * time.Millisecond,

		BlockSize: 0,  // BlockSize 0 means unlimited
		StMin:     20, // 20ms separation time

		MaxWaitFrame:    0, // Default 0
		TxDataMinLength: 0, // Default 0
	}
}

// Validate checks if the configuration parameters are valid.
func (c *Config) Validate() error {
	// Simple validation logic
	// In the future, we can add range checks for timeouts, etc.
	return nil
}
