package tp

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	c := DefaultConfig()
	if err := c.Validate(); err != nil {
		t.Errorf("DefaultConfig should be valid, got: %v", err)
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	c := DefaultConfig()
	if c.TimeoutN_As != 1000*time.Millisecond {
		t.Errorf("Expected default TimeoutN_As to be 1000ms, got %v", c.TimeoutN_As)
	}
	if c.MaxWaitFrame != 0 {
		t.Errorf("Expected MaxWaitFrame to be 0, got %d", c.MaxWaitFrame)
	}
}
