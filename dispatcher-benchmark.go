package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Output represents a sample payload with a timestamp for latency tracking
type Output struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// Global counters for metrics
var (
	totalMessages uint64
	totalLatency  time.Duration
)

// SendMultiPayload simulates sending data to an external service with network latency
func SendMultiPayload(payload string) {
	// Simulate variable network latency between 50-150ms
	latency := time.Duration(50+rand.Intn(100)) * time.Millisecond
	time.Sleep(latency)
}

// Metrics represents the performance metrics
type Metrics struct {
	MessagesProcessed uint64
	AverageLatency    time.Duration
	Throughput        float64 // messages per second
}

func trackMetrics(output Output) {
	atomic.AddUint64(&totalMessages, 1)
	latency := time.Since(output.Timestamp)
	atomic.AddInt64((*int64)(&totalLatency), int64(latency))
}

func getMetrics(duration time.Duration) Metrics {
	messages := atomic.LoadUint64(&totalMessages)
	totalLat := atomic.LoadInt64((*int64)(&totalLatency))
	
	var avgLatency time.Duration
	if messages > 0 {
		avgLatency = time.Duration(totalLat) / time.Duration(messages)
	}
	
	throughput := float64(messages) / duration.Seconds()
	
	return Metrics{
		MessagesProcessed: messages,
		AverageLatency:    avgLatency,
		Throughput:        throughput,
	}
}

func (w Worker) handleOutputs(outputs []Output) {
	outputsByte, err := json.Marshal(outputs)
	if err != nil {
		return
	}

	outputsStr := string(outputsByte)
	SendMultiPayload(outputsStr)
	
	// Track metrics for each output
	for _, output := range outputs {
		trackMetrics(output)
	}
}

func main() {
	// Configuration
	maxSize := 100
	flushInterval := 1 * time.Second
	numWorkers := 5
	testDuration := 1 * time.Minute
	
	// Create input channel and dispatcher
	inputChannel := make(chan Output)
	dispatcher := NewDispatcher(maxSize, flushInterval, inputChannel)
	
	// Create and start worker pool
	pool := NewWorkerPool(numWorkers, dispatcher.GetOutputChannel(), "test")
	
	// Start components
	dispatcher.Start()
	pool.Start()
	
	// Reset metrics
	atomic.StoreUint64(&totalMessages, 0)
	atomic.StoreInt64((*int64)(&totalLatency), 0)
	
	// Start time for benchmarking
	startTime := time.Now()
	
	// Generate test data
	go func() {
		for time.Since(startTime) < testDuration {
			output := Output{
				ID:        fmt.Sprintf("msg-%d", rand.Int()),
				Payload:   "test payload",
				Timestamp: time.Now(),
			}
			inputChannel <- output
			// Simulate input rate of ~1000 messages per second
			time.Sleep(time.Millisecond)
		}
		close(inputChannel)
	}()
	
	// Wait for test duration
	time.Sleep(testDuration)
	
	// Stop components
	dispatcher.Stop()
	pool.Stop()
	
	// Calculate and display metrics
	metrics := getMetrics(testDuration)
	fmt.Printf("\nBenchmark Results:\n")
	fmt.Printf("Test Duration: %v\n", testDuration)
	fmt.Printf("Messages Processed: %d\n", metrics.MessagesProcessed)
	fmt.Printf("Average Latency: %v\n", metrics.AverageLatency)
	fmt.Printf("Throughput: %.2f messages/second\n", metrics.Throughput)
}
