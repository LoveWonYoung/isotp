package tp

import (
	"fmt"
	"sync"
	"time"
)

// Timer mirrors the Python helper that tracks elapsed time against a timeout.
type Timer struct {
	startTime time.Time
	timeoutNS int64
	running   bool
}

func NewTimer(timeoutSeconds float64) *Timer {
	t := &Timer{}
	t.SetTimeout(timeoutSeconds)
	return t
}

func (t *Timer) SetTimeout(timeoutSeconds float64) {
	t.timeoutNS = int64(timeoutSeconds * 1e9)
}

func (t *Timer) Start(timeoutSeconds ...float64) {
	if len(timeoutSeconds) > 0 {
		t.SetTimeout(timeoutSeconds[0])
	}
	t.startTime = time.Now()
	t.running = true
}

func (t *Timer) Stop() {
	t.running = false
	t.startTime = time.Time{}
}

func (t *Timer) elapsedDuration() time.Duration {
	if !t.running {
		return 0
	}
	return time.Since(t.startTime)
}

func (t *Timer) Elapsed() float64 {
	return t.elapsedDuration().Seconds()
}

func (t *Timer) ElapsedNS() int64 {
	return t.elapsedDuration().Nanoseconds()
}

func (t *Timer) RemainingNS() int64 {
	if t.IsStopped() {
		return 0
	}
	remaining := t.timeoutNS - t.ElapsedNS()
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (t *Timer) Remaining() float64 {
	return float64(t.RemainingNS()) / 1e9
}

func (t *Timer) IsTimedOut() bool {
	if t.IsStopped() {
		return false
	}
	return t.ElapsedNS() > t.timeoutNS || t.timeoutNS == 0
}

func (t *Timer) IsStopped() bool {
	return !t.running
}

// ByteGenerator yields the next byte and true if more data is available.
type ByteGenerator func() (byte, bool)

type FiniteByteGenerator struct {
	gen      ByteGenerator
	size     int
	consumed int
	depleted bool
}

func NewFiniteByteGenerator(gen ByteGenerator, size int) (*FiniteByteGenerator, error) {
	if gen == nil {
		return nil, fmt.Errorf("given data tuple must be a pair of generator, size (int)")
	}
	if size < 0 {
		return nil, fmt.Errorf("given data size must a be a positive integer")
	}

	return &FiniteByteGenerator{
		gen:  gen,
		size: size,
	}, nil
}

func (f *FiniteByteGenerator) TotalLength() int {
	return f.size
}

func (f *FiniteByteGenerator) RemainingSize() int {
	return f.size - f.consumed
}

func (f *FiniteByteGenerator) Depleted() bool {
	return f.RemainingSize() <= 0 || f.depleted
}

func (f *FiniteByteGenerator) Consume(size int, enforceExact bool) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("requested size must be non-negative")
	}

	data := make([]byte, 0, size)
	for i := 0; i < size; i++ {
		b, ok := f.gen()
		if !ok {
			f.depleted = true
			break
		}
		data = append(data, b)
	}

	f.consumed += len(data)
	if f.consumed > f.size {
		return data, BadGeneratorError{IsoTpError: NewIsoTpError("Consumed more data than specified size")}
	}
	if len(data) < size {
		f.depleted = true
		if enforceExact {
			return data, BadGeneratorError{IsoTpError: NewIsoTpError(
				fmt.Sprintf("Did not read the requested amount of data. Tried to read %d, got %d", size, len(data)),
			)}
		}
	}
	return data, nil
}

// SafeQueue is a thread-safe queue using a slice and a mutex.
type SafeQueue[T any] struct {
	items []T
	mu    sync.Mutex
}

func NewSafeQueue[T any]() *SafeQueue[T] {
	return &SafeQueue[T]{
		items: make([]T, 0),
	}
}

func (q *SafeQueue[T]) Push(item T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

func (q *SafeQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		var zero T
		return zero, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *SafeQueue[T]) Peek() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		var zero T
		return zero, false
	}
	return q.items[0], true
}

func (q *SafeQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *SafeQueue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make([]T, 0)
}
