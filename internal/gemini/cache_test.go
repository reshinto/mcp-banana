package gemini

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genai"
)

// newTestClientCache creates a ClientCache for testing.
func newTestClientCache(timeoutSecs int, proConcurrency int) *ClientCache {
	return NewClientCache(timeoutSecs, proConcurrency)
}

// TestClientCache_EmptyKeyReturnsError verifies that an empty key returns an error.
func TestClientCache_EmptyKeyReturnsError(test *testing.T) {
	cache := newTestClientCache(30, 2)

	_, clientError := cache.GetClient(context.Background(), "")
	if clientError == nil {
		test.Fatal("expected error for empty key")
	}
}

// TestClientCache_CustomKey verifies that a non-empty key creates and caches a new client.
func TestClientCache_CustomKey(test *testing.T) {
	cache := newTestClientCache(30, 2)

	result, clientError := cache.GetClient(context.Background(), "custom-api-key-xyz")
	if clientError != nil {
		test.Fatalf("unexpected error: %v", clientError)
	}
	if result == nil {
		test.Fatal("expected non-nil client for custom key")
	}
}

// TestClientCache_CachedClient verifies that the same key returns the same client instance.
func TestClientCache_CachedClient(test *testing.T) {
	cache := newTestClientCache(30, 2)

	firstClient, firstError := cache.GetClient(context.Background(), "repeat-key")
	if firstError != nil {
		test.Fatalf("first call: unexpected error: %v", firstError)
	}

	secondClient, secondError := cache.GetClient(context.Background(), "repeat-key")
	if secondError != nil {
		test.Fatalf("second call: unexpected error: %v", secondError)
	}

	if firstClient != secondClient {
		test.Error("expected same client pointer on second call (cache miss would create a new one)")
	}
}

// TestClientCache_FactoryError verifies that a factory failure returns an error.
func TestClientCache_FactoryError(test *testing.T) {
	original := genaiClientFactory
	defer func() { genaiClientFactory = original }()

	genaiClientFactory = func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
		return nil, errors.New("simulated factory failure")
	}

	cache := newTestClientCache(30, 2)
	_, clientError := cache.GetClient(context.Background(), "failing-key")
	if clientError == nil {
		test.Fatal("expected error when factory fails")
	}
}

// TestClientCache_ConcurrentSameKey verifies that concurrent requests for the
// same key only create one client, exercising the double-check under write lock.
func TestClientCache_ConcurrentSameKey(test *testing.T) {
	cache := newTestClientCache(30, 2)

	const goroutineCount = 20
	results := make([]GeminiService, goroutineCount)
	errs := make([]error, goroutineCount)
	done := make(chan struct{})
	start := make(chan struct{})

	for goroutineIndex := 0; goroutineIndex < goroutineCount; goroutineIndex++ {
		go func(index int) {
			<-start
			results[index], errs[index] = cache.GetClient(context.Background(), "shared-concurrent-key")
			done <- struct{}{}
		}(goroutineIndex)
	}

	close(start)
	for goroutineIndex := 0; goroutineIndex < goroutineCount; goroutineIndex++ {
		<-done
	}

	var firstResult GeminiService
	for goroutineIndex := 0; goroutineIndex < goroutineCount; goroutineIndex++ {
		if errs[goroutineIndex] != nil {
			test.Errorf("goroutine %d: unexpected error: %v", goroutineIndex, errs[goroutineIndex])
			continue
		}
		if results[goroutineIndex] == nil {
			test.Errorf("goroutine %d: expected non-nil client", goroutineIndex)
			continue
		}
		if firstResult == nil {
			firstResult = results[goroutineIndex]
		} else if results[goroutineIndex] != firstResult {
			test.Errorf("goroutine %d: expected same client pointer, got a different one", goroutineIndex)
		}
	}
}

// TestClientCache_DoubleCheckUnderWriteLock exercises the re-check inside the
// write lock (line 55 of cache.go). It uses the afterReadMiss hook to inject a
// rendezvous between the read-lock miss and write-lock acquisition so that:
//
//  1. Goroutine 1 passes the read-lock miss and then blocks in afterReadMiss.
//  2. Goroutine 2 is launched and also passes the read-lock miss.
//  3. Goroutine 2 waits to acquire the write lock because goroutine 1 holds it.
//  4. Goroutine 1 releases the write lock (after seeding the cache).
//  5. Goroutine 2 acquires the write lock and finds the entry in the double-check.
func TestClientCache_DoubleCheckUnderWriteLock(test *testing.T) {
	// Restore the hook after the test.
	test.Cleanup(func() { afterReadMiss = nil })

	cache := newTestClientCache(30, 2)

	// goroutine1Waiting is closed when goroutine 1 has passed the read-lock miss
	// and is inside afterReadMiss, waiting to proceed to the write lock.
	goroutine1Waiting := make(chan struct{})
	// goroutine2Ready is closed to release goroutine 1 once goroutine 2 is
	// confirmed to be blocking on the write lock.
	goroutine2Ready := make(chan struct{})

	// Only let the first caller rendezvous; subsequent callers pass straight through.
	firstCall := true
	afterReadMiss = func() {
		if !firstCall {
			return
		}
		firstCall = false
		close(goroutine1Waiting) // signal: goroutine 1 has missed read lock
		<-goroutine2Ready        // wait until goroutine 2 is queued on write lock
	}

	const sharedKey = "double-check-key"
	type result struct {
		client GeminiService
		err    error
	}
	results := make(chan result, 2)

	// Goroutine 1 will be the first to miss the read lock and acquire the write lock.
	go func() {
		client, clientError := cache.GetClient(context.Background(), sharedKey)
		results <- result{client, clientError}
	}()

	// Wait until goroutine 1 has passed the read-lock miss and is paused.
	<-goroutine1Waiting

	// Goroutine 2 starts now. It will also miss the read lock (cache is still empty
	// because goroutine 1 is paused before acquiring the write lock), then block
	// waiting for the write lock.
	go func() {
		client, clientError := cache.GetClient(context.Background(), sharedKey)
		results <- result{client, clientError}
	}()

	// Give goroutine 2 time to block on the write lock.
	time.Sleep(20 * time.Millisecond)

	// Release goroutine 1. It acquires the write lock, creates the client, seeds
	// the cache, and releases the lock. Goroutine 2 then acquires the write lock,
	// hits the double-check at cache.go line 55, finds the entry, and returns it
	// without calling the factory again.
	close(goroutine2Ready)

	firstResult := <-results
	secondResult := <-results

	if firstResult.err != nil {
		test.Errorf("goroutine 1: unexpected error: %v", firstResult.err)
	}
	if secondResult.err != nil {
		test.Errorf("goroutine 2: unexpected error: %v", secondResult.err)
	}
	if firstResult.client != secondResult.client {
		test.Error("expected both goroutines to return the same client pointer")
	}
}
