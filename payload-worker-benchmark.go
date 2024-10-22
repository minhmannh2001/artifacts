package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockServices implements the external service calls
type MockServices struct {
	latency    time.Duration
	errorRate  float64
	callCount  uint64
	totalBytes uint64
}

func NewMockServices(latency time.Duration, errorRate float64) *MockServices {
	return &MockServices{
		latency:   latency,
		errorRate: errorRate,
	}
}

func (m *MockServices) SendMultiPayload(payload string) error {
	atomic.AddUint64(&m.callCount, 1)
	atomic.AddUint64(&m.totalBytes, uint64(len(payload)))
	time.Sleep(m.latency)
	return nil
}

func (m *MockServices) UpdateAgentJobResults(payload string) error {
	atomic.AddUint64(&m.callCount, 1)
	atomic.AddUint64(&m.totalBytes, uint64(len(payload)))
	time.Sleep(m.latency)
	return nil
}

// BenchmarkMetrics holds the performance metrics
type BenchmarkMetrics struct {
	TotalMessages      uint64
	TotalBatches      uint64
	TotalBytes        uint64
	AverageLatency    time.Duration
	MessagesPerSecond float64
	BytesPerSecond    float64
}

// Helper function to generate test data
func generateTestOutput(id int) Output {
	return Output{
		Client:    fmt.Sprintf("client-%d", id),
		TaskID:    fmt.Sprintf("task-%d", id),
		RequestID: fmt.Sprintf("req-%d", id),
		Payload:   fmt.Sprintf("payload-%d", id),
	}
}

func BenchmarkSendMultiPayloadWorker(b *testing.B) {
	tests := []struct {
		name      string
		mode      string
		latency   time.Duration
		errorRate float64
		msgCount  int
	}{
		{
			name:      "Server_LowLatency",
			mode:      "server",
			latency:   50 * time.Millisecond,
			errorRate: 0,
			msgCount: 1000,
		},
		{
			name:      "Server_HighLatency",
			mode:      "server",
			latency:   200 * time.Millisecond,
			errorRate: 0,
			msgCount: 1000,
		},
		{
			name:      "Agent_WithErrors",
			mode:      "agent",
			latency:   50 * time.Millisecond,
			errorRate: 0.1,
			msgCount: 1000,
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Initialize channels and services
			outputCh := make(chan Output, 1000)
			mockServices := NewMockServices(tt.latency, tt.errorRate)
			
			// Replace the actual service clients with mocks
			client = mockServices
			utils = mockServices
			
			// Start the worker
			var wg sync.WaitGroup
			wg.Add(1)
			
			startTime := time.Now()
			
			go func() {
				defer wg.Done()
				SendMultiPayloadWorker(outputCh, tt.mode)
			}()
			
			// Send test messages
			for i := 0; i < tt.msgCount; i++ {
				outputCh <- generateTestOutput(i)
			}
			
			// Wait a bit for processing
			time.Sleep(2 * time.Second)
			close(outputCh)
			wg.Wait()
			
			duration := time.Since(startTime)
			
			// Calculate metrics
			metrics := BenchmarkMetrics{
				TotalMessages:      uint64(tt.msgCount),
				TotalBatches:      atomic.LoadUint64(&mockServices.callCount),
				TotalBytes:        atomic.LoadUint64(&mockServices.totalBytes),
				AverageLatency:    duration / time.Duration(atomic.LoadUint64(&mockServices.callCount)),
				MessagesPerSecond: float64(tt.msgCount) / duration.Seconds(),
				BytesPerSecond:    float64(atomic.LoadUint64(&mockServices.totalBytes)) / duration.Seconds(),
			}
			
			// Report results
			b.ReportMetric(float64(metrics.MessagesPerSecond), "msgs/sec")
			b.ReportMetric(float64(metrics.BytesPerSecond), "bytes/sec")
			b.ReportMetric(float64(metrics.AverageLatency.Milliseconds()), "avg_latency_ms")
			b.ReportMetric(float64(metrics.TotalBatches), "total_batches")
			
			fmt.Printf("\nDetailed metrics for %s:\n", tt.name)
			fmt.Printf("Total Messages: %d\n", metrics.TotalMessages)
			fmt.Printf("Total Batches: %d\n", metrics.TotalBatches)
			fmt.Printf("Total Bytes: %d\n", metrics.TotalBytes)
			fmt.Printf("Average Latency: %v\n", metrics.AverageLatency)
			fmt.Printf("Messages/sec: %.2f\n", metrics.MessagesPerSecond)
			fmt.Printf("Bytes/sec: %.2f\n", metrics.BytesPerSecond)
		})
	}
}

// Example usage:
// go test -bench=. -benchmem -benchtime=10s

// Helper to run load test
func RunLoadTest(duration time.Duration, messagesPerSec int, mode string) BenchmarkMetrics {
	outputCh := make(chan Output, 1000)
	mockServices := NewMockServices(50*time.Millisecond, 0)
	
	// Replace actual services with mocks
	client = mockServices
	utils = mockServices
	
	var wg sync.WaitGroup
	wg.Add(1)
	
	startTime := time.Now()
	
	// Start worker
	go func() {
		defer wg.Done()
		SendMultiPayloadWorker(outputCh, mode)
	}()
	
	// Generate load
	messageInterval := time.Second / time.Duration(messagesPerSec)
	ticker := time.NewTicker(messageInterval)
	defer ticker.Stop()
	
	msgCount := 0
	for start := time.Now(); time.Since(start) < duration; {
		select {
		case <-ticker.C:
			outputCh <- generateTestOutput(msgCount)
			msgCount++
		}
	}
	
	close(outputCh)
	wg.Wait()
	
	testDuration := time.Since(startTime)
	
	return BenchmarkMetrics{
		TotalMessages:      uint64(msgCount),
		TotalBatches:      atomic.LoadUint64(&mockServices.callCount),
		TotalBytes:        atomic.LoadUint64(&mockServices.totalBytes),
		AverageLatency:    testDuration / time.Duration(atomic.LoadUint64(&mockServices.callCount)),
		MessagesPerSecond: float64(msgCount) / testDuration.Seconds(),
		BytesPerSecond:    float64(atomic.LoadUint64(&mockServices.totalBytes)) / testDuration.Seconds(),
	}
}

func ExampleLoadTest() {
	metrics := RunLoadTest(1*time.Minute, 100, "server")
	fmt.Printf("Load Test Results:\n")
	fmt.Printf("Total Messages: %d\n", metrics.TotalMessages)
	fmt.Printf("Total Batches: %d\n", metrics.TotalBatches)
	fmt.Printf("Average Latency: %v\n", metrics.AverageLatency)
	fmt.Printf("Messages/sec: %.2f\n", metrics.MessagesPerSecond)
	fmt.Printf("Bytes/sec: %.2f\n", metrics.BytesPerSecond)
}
