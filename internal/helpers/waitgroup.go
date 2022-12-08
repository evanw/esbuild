package helpers

import "sync/atomic"

// Go's "sync.WaitGroup" is not thread-safe. Specifically it's not safe to call
// "Add" concurrently with "Wait", which is problematic because we have a case
// where we would like to do that.
//
// This is a simple alternative implementation of "sync.WaitGroup" that is
// thread-safe and that works for our purposes. We don't need to worry about
// multiple waiters so the implementation can be very simple.
type ThreadSafeWaitGroup struct {
	counter int32
	channel chan struct{}
}

func MakeThreadSafeWaitGroup() *ThreadSafeWaitGroup {
	return &ThreadSafeWaitGroup{
		channel: make(chan struct{}, 1),
	}
}

func (wg *ThreadSafeWaitGroup) Add(delta int32) {
	if counter := atomic.AddInt32(&wg.counter, delta); counter == 0 {
		wg.channel <- struct{}{}
	} else if counter < 0 {
		panic("sync: negative WaitGroup counter")
	}
}

func (wg *ThreadSafeWaitGroup) Done() {
	wg.Add(-1)
}

func (wg *ThreadSafeWaitGroup) Wait() {
	<-wg.channel
}
