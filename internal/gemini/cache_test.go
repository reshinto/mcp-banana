package gemini

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genai"
)

// newTestClientCache creates a ClientCache with a pre-built test default client.
func newTestClientCache(timeoutSecs int, proConcurrency int) *ClientCache {
	defaultClient := newTestClient(&mockContentGenerator{})
	return NewClientCache(defaultClient, timeoutSecs, proConcurrency)
}

// TestClientCache_DefaultClient verifies that an empty key returns the default client.
func TestClientCache_DefaultClient(test *testing.T) {
	cache := newTestClientCache(30, 2)

	result, clientError := cache.GetClient(context.Background(), "")
	if clientError != nil {
		test.Fatalf("unexpected error: %v", clientError)
	}
	if result != cache.defaultClient {
		test.Error("expected default client for empty key")
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
	if result == cache.defaultClient {
		test.Error("expected a different client for custom key, not the default")
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
	results := make([]*Client, goroutineCount)
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

	var firstResult *Client
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
// write lock by arranging for two goroutines to race past the read-lock miss
// and then queue behind the write lock. The first goroutine acquires the write
// lock, creates the client, seeds the cache, and releases. The second goroutine
// acquires the write lock next and hits the double-check re-read, finding the
// entry already present and returning it without calling the factory.
func TestClientCache_DoubleCheckUnderWriteLock(test *testing.T) {
	cache := newTestClientCache(30, 2)

	// Track factory call count to confirm the second goroutine skips creation.
	factoryCallCount := 0
	original := genaiClientFactory
	defer func() { genaiClientFactory = original }()
	genaiClientFactory = func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
		factoryCallCount++
		return original(ctx, config)
	}

	// readyToRace is closed once both goroutines have confirmed the cache miss
	// under read lock. holdWriteLock is used to keep the first goroutine inside
	// the write-lock critical section while the second goroutine queues behind it.
	holdWriteLock := make(chan struct{})
	firstHasLock := make(chan struct{})

	// Override factory for first goroutine to signal when it holds the write lock.
	firstFactory := genaiClientFactory
	signalFactory := func(ctx context.Context, config *genai.ClientConfig) (*genai.Client, error) {
		close(firstHasLock) // signal: first goroutine now holds write lock
		<-holdWriteLock     // block until test releases it
		return firstFactory(ctx, config)
	}

	const sharedKey = "double-check-race-key"
	results := make(chan *Client, 2)
	errs := make(chan error, 2)

	// First goroutine: will acquire write lock first (we arrange this via signalFactory).
	go func() {
		genaiClientFactory = signalFactory
		client, clientError := cache.GetClient(context.Background(), sharedKey)
		results <- client
		errs <- clientError
	}()

	// Wait until the first goroutine holds the write lock.
	<-firstHasLock

	// Restore normal factory for the second goroutine, then launch it.
	// At this point the cache is still empty under the write lock held by goroutine 1,
	// so goroutine 2 will pass the read lock miss and block on the write lock.
	genaiClientFactory = original
	go func() {
		client, clientError := cache.GetClient(context.Background(), sharedKey)
		results <- client
		errs <- clientError
	}()

	// Give goroutine 2 time to reach and block on the write lock.
	time.Sleep(20 * time.Millisecond)

	// Release goroutine 1; it seeds the cache and releases the write lock.
	// Goroutine 2 then acquires the write lock, hits the double-check, and returns
	// the already-created client without calling the factory a second time.
	close(holdWriteLock)

	for goroutineIndex := 0; goroutineIndex < 2; goroutineIndex++ {
		clientError := <-errs
		if clientError != nil {
			test.Errorf("goroutine %d: unexpected error: %v", goroutineIndex, clientError)
		}
	}
	firstResult := <-results
	secondResult := <-results

	if firstResult != secondResult {
		test.Errorf("expected both goroutines to return the same client, got different pointers")
	}
	if factoryCallCount > 1 {
		test.Errorf("expected factory to be called at most once, got %d calls", factoryCallCount)
	}
}
