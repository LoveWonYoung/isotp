package tp

import (
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestRace_Send_Process(t *testing.T) {
	// This test is designed to be run with `go test -race`.
	// It simulates a typical usage: one goroutine sending, one goroutine processing loop.

	addr := NewAddress(Normal11bits, 0x123, 0x456, 0x00, 0x00, 0, 0, 0, false, false)

	// Mocks
	rxfn := func(timeout float64) *CanMessage {
		time.Sleep(100 * time.Microsecond)
		return nil
	}
	txfn := func(msg *CanMessage) {}

	tll := NewTransportLayerLogic(rxfn, txfn, addr, func(e error) { log.Println(e) }, nil, nil)

	var wg sync.WaitGroup
	wg.Add(2)

	stopCh := make(chan struct{})

	// Goroutine 1: Process Loop (Consumer)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				tll.Process(0.001, true, true)
			}
		}
	}()

	// Goroutine 2: Sender (Producer)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// Random payload
			size := rand.Intn(50) + 1
			data := make([]byte, size)
			tll.Send(data, Physical, 0)
			time.Sleep(time.Duration(rand.Intn(500)) * time.Microsecond)
		}
		close(stopCh)
	}()

	wg.Wait()
}
