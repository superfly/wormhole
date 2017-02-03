package utils

import (
	"sync"
)

// Shutdown is a struct that coordinates shutdowns
type Shutdown struct {
	beginOnce    sync.Once // synchronizes closing begin channel
	completeOnce sync.Once //synchronizes closing complete channel
	begin        chan int  // closed when the shutdown begins
	complete     chan int  // closed when the shutdown completes
}

// NewShutdown returns a new Shutdown struct
func NewShutdown() *Shutdown {
	return &Shutdown{
		begin:    make(chan int),
		complete: make(chan int),
	}
}

// Begin marks shutdown as started
func (s *Shutdown) Begin() {
	s.beginOnce.Do(func() {
		close(s.begin)
	})
}

// WaitBegin blocks until shutdown is started
func (s *Shutdown) WaitBegin() {
	<-s.begin
}

// WaitBeginCh returns a channel that programs can block on.
// The channel will close when the shutdown is started.
func (s *Shutdown) WaitBeginCh() <-chan int {
	return s.begin
}

// Complete marks shutdown as finished
func (s *Shutdown) Complete() {
	s.completeOnce.Do(func() {
		close(s.complete)
	})
}

// WaitComplete blocks until shutdown is finished
func (s *Shutdown) WaitComplete() {
	<-s.complete
}
