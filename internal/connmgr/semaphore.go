// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import "context"

// semaphore is a simple context-aware channel based semaphore for bounding
// concurrent access.
type semaphore chan struct{}

// makeSemaphore returns a new semaphore with the given capacity.
func makeSemaphore(n uint32) semaphore {
	return make(chan struct{}, n)
}

// Acquire acquires the semaphore.  It blocks until resources are available or
// the provided context is done.  It returns true on success and false when the
// context is done before semaphore can be acquired.
func (s semaphore) Acquire(ctx context.Context) bool {
	select {
	case s <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	return true
}

// TryAcquire attempts to acquire the semaphore without blocking when there are
// no resources immediately available.
//
// It returns true with a nil error on success.  It returns false with a nil
// error when the semaphore is at capacity and no permit is available.
//
// Finally, it returns false with the error associated with the context
// immediately when the context is already canceled or timed out at the time of
// the call.  It does not attempt to acquire the semaphore in that case.
func (s semaphore) TryAcquire(ctx context.Context) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	select {
	case s <- struct{}{}:
		return true, nil
	default:
	}
	return false, nil
}

// Release release the semaphore.
func (s semaphore) Release() {
	select {
	case <-s:
	default:
	}
}
