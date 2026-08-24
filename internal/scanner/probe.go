package scanner

import "sync"

// registryProbeConcurrency bounds the fan-out of network-bound registry
// freshness probes (npm view / release checks) so we stay polite to
// registries while avoiding serial latency.
const registryProbeConcurrency = 4

// probeBounded runs fn(i) for i in [0,n) with at most maxWorkers concurrent
// goroutines. n <= 0 is a no-op; maxWorkers <= 0 degrades to serial.
func probeBounded(n, maxWorkers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if maxWorkers <= 0 || maxWorkers > n {
		maxWorkers = n
	}
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}
