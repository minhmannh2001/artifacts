package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MetricsCollector tracks performance metrics for both implementations
type MetricsCollector struct {
	messagesProcessed uint64
	batchesProcessed  uint64
	totalLatency      int64
	maxLatency        int64
	batchSizes        []int
	mu                sync.Mutex
}

func newMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		batchSizes: make([]int, 0),
	}
}

func (m *MetricsCollector) recordBatch(size int, latency time.Duration) {
	atomic.AddUint64(&m.messagesProcessed, uint64(size))
	atomic.AddUint64(&m.batchesProcessed, 1)
	atomic.AddInt64(&m.totalLatency, int64(latency))
	
	if latencyMs := int64(latency.Milliseconds()); latencyMs > atomic.LoadInt64(&m.maxLatency) {
		atomic.StoreInt64(&m.maxLatency, latencyMs)
	}
	
	m.mu.Lock()
	m.batchSizes = append(m.batchSizes, size)
	m.mu.Unlock()
}

// TestSystem represents a system under test
type TestSystem struct {
	name     string
	metrics  *MetricsCollector
	shutdown chan struct{}
}

func runPoolSystem(b *testing.B, messageCount int, numWorkers int) *TestSystem {
	metrics := newMetricsCollector()
	shutdown := make(chan struct{})
	
	// Setup channels
	inputCh := make(chan Output, 1000)
	
	// Create and start dispatcher
	dispatcher := NewDispatcher(256, 100*time.Millisecond, inputCh)
	dispatcher.Start()
	
	// Create and start worker pool
	pool := NewWorkerPool(numWorkers, dispatcher.GetOutputChannel(), "test")
	pool.Start()
	
	// Generate and send test data
	go func() {
		for i := 0; i < messageCount; i++ {
			inputCh <- Output{
				ID:        fmt.Sprintf("msg-%d", i),
				Payload:   fmt.Sprintf("payload-%d", i),
				Timestamp: time.Now(),
			}
		}
		close(inputCh)
	}()
	
	// Cleanup routine
	go func() {
		<-shutdown
		dispatcher.Stop()
		pool.Stop()
	}()
	
	return &TestSystem{
		name:     "WorkerPool",
		metrics:  metrics,
		shutdown: shutdown,
	}
}

func runSimpleSystem(b *testing.B, messageCount int) *TestSystem {
	metrics := newMetricsCollector()
	shutdown := make(chan struct{})
	
	// Setup channel
	inputCh := make(chan Output, 1000)
	
	// Start worker
	var wg sync.WaitGroup
	wg.Add(1)
	
	go func() {
		defer wg.Done()
		SendMultiPayloadWorker(inputCh, "test")
	}()
	
	// Generate and send test data
	go func() {
		for i := 0; i < messageCount; i++ {
			inputCh <- Output{
				ID:        fmt.Sprintf("msg-%d", i),
				Payload:   fmt.Sprintf("payload-%d", i),
				Timestamp: time.Now(),
			}
		}
		close(inputCh)
	}()
	
	// Cleanup routine
	go func() {
		<-shutdown
		wg.Wait()
	}()
	
	return &TestSystem{
		name:     "SimpleWorker",
		metrics:  metrics,
		shutdown: shutdown,
	}
}

func BenchmarkPayloadSystems(b *testing.B) {
	testCases := []struct {
		name         string
		messageCount int
		workers      int
	}{
		{"Small_Load", 100, 2},
		{"Medium_Load", 1000, 5},
		{"Large_Load", 10000, 10},
	}
	
	for _, tc := range testCases {
		// Test Worker Pool System
		b.Run(fmt.Sprintf("WorkerPool_%s", tc.name), func(b *testing.B) {
			b.ResetTimer()
			system := runPoolSystem(b, tc.messageCount, tc.workers)
			time.Sleep(2 * time.Second) // Allow processing to complete
			close(system.shutdown)
			
			reportMetrics(b, system.metrics)
		})
		
		// Test Simple Worker System
		b.Run(fmt.Sprintf("SimpleWorker_%s", tc.name), func(b *testing.B) {
			b.ResetTimer()
			system := runSimpleSystem(b, tc.messageCount)
			time.Sleep(2 * time.Second) // Allow processing to complete
			close(system.shutdown)
			
			reportMetrics(b, system.metrics)
		})
	}
}

func reportMetrics(b *testing.B, m *MetricsCollector) {
	totalMessages := atomic.LoadUint64(&m.messagesProcessed)
	totalBatches := atomic.LoadUint64(&m.batchesProcessed)
	avgLatency := time.Duration(atomic.LoadInt64(&m.totalLatency)) / time.Duration(totalBatches)
	maxLatency := time.Duration(atomic.LoadInt64(&m.maxLatency)) * time.Millisecond
	
	var avgBatchSize float64
	if totalBatches > 0 {
		avgBatchSize = float64(totalMessages) / float64(totalBatches)
	}
	
	b.ReportMetric(float64(totalMessages), "total_messages")
	b.ReportMetric(float64(totalBatches), "total_batches")
	b.ReportMetric(float64(avgLatency.Milliseconds()), "avg_latency_ms")
	b.ReportMetric(float64(maxLatency.Milliseconds()), "max_latency_ms")
	b.ReportMetric(avgBatchSize, "avg_batch_size")
}

// LoadTest provides a more realistic test scenario
func RunLoadTest(duration time.Duration, messagesPerSec int) {
	fmt.Printf("Running load test for %v at %d messages/second\n", duration, messagesPerSec)
	
	// Test Worker Pool System
	runSystemLoadTest("Worker Pool", duration, messagesPerSec, func(inputCh chan Output) {
		dispatcher := NewDispatcher(256, 100*time.Millisecond, inputCh)
		dispatcher.Start()
		pool := NewWorkerPool(5, dispatcher.GetOutputChannel(), "test")
		pool.Start()
		
		time.Sleep(duration)
		dispatcher.Stop()
		pool.Stop()
	})
	
	// Test Simple Worker System
	runSystemLoadTest("Simple Worker", duration, messagesPerSec, func(inputCh chan Output) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			SendMultiPayloadWorker(inputCh, "test")
		}()
		
		time.Sleep(duration)
		close(inputCh)
		wg.Wait()
	})
}

func runSystemLoadTest(name string, duration time.Duration, messagesPerSec int, systemFunc func(chan Output)) {
	inputCh := make(chan Output, 1000)
	metrics := newMetricsCollector()
	
	// Start the system
	go systemFunc(inputCh)
	
	// Generate load
	start := time.Now()
	messageInterval := time.Second / time.Duration(messagesPerSec)
	ticker := time.NewTicker(messageInterval)
	defer ticker.Stop()
	
	msgCount := 0
	for time.Since(start) < duration {
		select {
		case <-ticker.C:
			inputCh <- Output{
				ID:        fmt.Sprintf("msg-%d", msgCount),
				Payload:   fmt.Sprintf("payload-%d", msgCount),
				Timestamp: time.Now(),
			}
			msgCount++
		}
	}
	
	// Print results
	fmt.Printf("\nResults for %s:\n", name)
	fmt.Printf("Total Messages: %d\n", atomic.LoadUint64(&metrics.messagesProcessed))
	fmt.Printf("Total Batches: %d\n", atomic.LoadUint64(&metrics.batchesProcessed))
	fmt.Printf("Average Latency: %v\n", time.Duration(atomic.LoadInt64(&metrics.totalLatency)/int64(metrics.batchesProcessed)))
	fmt.Printf("Max Latency: %v\n", time.Duration(atomic.LoadInt64(&metrics.maxLatency))*time.Millisecond)
}

func Example() {
	// Run a 1-minute load test at 100 messages per second
	RunLoadTest(1*time.Minute, 100)
}
