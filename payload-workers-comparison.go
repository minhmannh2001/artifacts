package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/montanaflynn/stats"
)

// Metrics for tracking performance
type MetricsCollector struct {
	messagesProcessed uint64
	batchesProcessed  uint64
	totalLatency     int64
	messageSizes     []float64
	batchSizes       []float64
	latencies        []float64
	mu               sync.Mutex
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		messageSizes: make([]float64, 0),
		batchSizes:   make([]float64, 0),
		latencies:    make([]float64, 0),
	}
}

func (m *MetricsCollector) recordBatch(size int, latency time.Duration) {
	atomic.AddUint64(&m.batchesProcessed, 1)
	atomic.AddUint64(&m.messagesProcessed, uint64(size))
	atomic.AddInt64(&m.totalLatency, int64(latency))
	
	m.mu.Lock()
	m.batchSizes = append(m.batchSizes, float64(size))
	m.latencies = append(m.latencies, float64(latency.Milliseconds()))
	m.mu.Unlock()
}

type BenchmarkResult struct {
	MessagesPerSecond float64
	AverageLatency    time.Duration
	P95Latency        time.Duration
	AverageBatchSize  float64
	TotalMessages     uint64
	TotalBatches      uint64
}

func generateTestData(count int) []Output {
	data := make([]Output, count)
	for i := 0; i < count; i++ {
		data[i] = Output{
			ID:        fmt.Sprintf("msg-%d", i),
			Payload:   fmt.Sprintf("payload-%d", i),
			Timestamp: time.Now(),
		}
	}
	return data
}

func benchmarkWorker(
	b *testing.B,
	implementation string,
	messageCount int,
	workerFunc func(chan Output, string),
) BenchmarkResult {
	metrics := NewMetricsCollector()
	outputCh := make(chan Output, 1000)
	
	// Start worker
	var wg sync.WaitGroup
	wg.Add(1)
	
	startTime := time.Now()
	
	go func() {
		defer wg.Done()
		workerFunc(outputCh, "test")
	}()
	
	// Send test messages
	testData := generateTestData(messageCount)
	for _, msg := range testData {
		outputCh <- msg
	}
	
	// Wait for processing to complete
	time.Sleep(time.Second) // Allow time for processing
	close(outputCh)
	wg.Wait()
	
	duration := time.Since(startTime)
	
	// Calculate statistics
	p95Latency, _ := stats.Percentile(metrics.latencies, 95.0)
	avgBatchSize, _ := stats.Mean(metrics.batchSizes)
	
	return BenchmarkResult{
		MessagesPerSecond: float64(metrics.messagesProcessed) / duration.Seconds(),
		AverageLatency:    time.Duration(atomic.LoadInt64(&metrics.totalLatency)) / time.Duration(metrics.batchesProcessed),
		P95Latency:        time.Duration(p95Latency) * time.Millisecond,
		AverageBatchSize:  avgBatchSize,
		TotalMessages:     metrics.messagesProcessed,
		TotalBatches:      metrics.batchesProcessed,
	}
}

func BenchmarkCompareImplementations(b *testing.B) {
	testCases := []struct {
		name         string
		messageCount int
	}{
		{"Small_Load", 100},
		{"Medium_Load", 1000},
		{"Large_Load", 10000},
	}
	
	for _, tc := range testCases {
		b.Run(fmt.Sprintf("Original_%s", tc.name), func(b *testing.B) {
			result := benchmarkWorker(b, "original", tc.messageCount, SendMultiPayloadWorker)
			b.ReportMetric(result.MessagesPerSecond, "msgs/sec")
			b.ReportMetric(float64(result.AverageLatency.Milliseconds()), "avg_latency_ms")
			b.ReportMetric(float64(result.P95Latency.Milliseconds()), "p95_latency_ms")
			b.ReportMetric(result.AverageBatchSize, "avg_batch_size")
		})
		
		b.Run(fmt.Sprintf("New_%s", tc.name), func(b *testing.B) {
			result := benchmarkWorker(b, "new", tc.messageCount, SendMultiPayloadWorkerNew)
			b.ReportMetric(result.MessagesPerSecond, "msgs/sec")
			b.ReportMetric(float64(result.AverageLatency.Milliseconds()), "avg_latency_ms")
			b.ReportMetric(float64(result.P95Latency.Milliseconds()), "p95_latency_ms")
			b.ReportMetric(result.AverageBatchSize, "avg_batch_size")
		})
	}
}

// Load test helper
func RunComparison(duration time.Duration, messagesPerSec int) {
	implementations := []struct {
		name string
		fn   func(chan Output, string)
	}{
		{"Original", SendMultiPayloadWorker},
		{"New", SendMultiPayloadWorkerNew},
	}
	
	for _, impl := range implementations {
		metrics := NewMetricsCollector()
		outputCh := make(chan Output, 1000)
		
		var wg sync.WaitGroup
		wg.Add(1)
		
		startTime := time.Now()
		
		go func() {
			defer wg.Done()
			impl.fn(outputCh, "test")
		}()
		
		// Generate load
		messageInterval := time.Second / time.Duration(messagesPerSec)
		ticker := time.NewTicker(messageInterval)
		msgCount := 0
		
		for start := time.Now(); time.Since(start) < duration; {
			select {
			case <-ticker.C:
				outputCh <- Output{
					ID:        fmt.Sprintf("msg-%d", msgCount),
					Payload:   fmt.Sprintf("payload-%d", msgCount),
					Timestamp: time.Now(),
				}
				msgCount++
			}
		}
		
		close(outputCh)
		wg.Wait()
		
		testDuration := time.Since(startTime)
		p95Latency, _ := stats.Percentile(metrics.latencies, 95.0)
		avgBatchSize, _ := stats.Mean(metrics.batchSizes)
		
		fmt.Printf("\nResults for %s implementation:\n", impl.name)
		fmt.Printf("Messages/sec: %.2f\n", float64(metrics.messagesProcessed)/testDuration.Seconds())
		fmt.Printf("Average Latency: %v\n", time.Duration(atomic.LoadInt64(&metrics.totalLatency))/time.Duration(metrics.batchesProcessed))
		fmt.Printf("P95 Latency: %v\n", time.Duration(p95Latency)*time.Millisecond)
		fmt.Printf("Average Batch Size: %.2f\n", avgBatchSize)
		fmt.Printf("Total Messages: %d\n", metrics.messagesProcessed)
		fmt.Printf("Total Batches: %d\n", metrics.batchesProcessed)
	}
}

func Example() {
	// Run a 1-minute comparison with 100 messages per second
	fmt.Println("Running 1-minute comparison at 100 msgs/sec...")
	RunComparison(1*time.Minute, 100)
}
