package main

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/alitto/pond"
	"github.com/buraksezer/consistent"
	"github.com/cespare/xxhash"
)

type Data struct {
	Tenant     string
	DatafeedID string
	Info       string
}

type Member string

func (m Member) String() string {
	return string(m)
}

type hasher struct{}

func (h hasher) Sum64(data []byte) uint64 {
	return xxhash.Sum64(data)
}

type CircuitBreaker struct {
	failures  int
	threshold int
	lastFail  time.Time
	cooldown  time.Duration
}

type DatafeedStatus struct {
	circuitBreaker CircuitBreaker
	mu             sync.Mutex
}

type TenantRouter struct {
	channels        []chan Data
	consistentHash  *consistent.Consistent
	datafeedStatus  map[string]*DatafeedStatus
	mu              sync.RWMutex
	workerPools     []*pond.WorkerPool
}

func NewTenantRouter(numChannels, workersPerChannel int) *TenantRouter {
	cfg := consistent.Config{
		PartitionCount:    271,
		ReplicationFactor: 20,
		Load:              1.25,
		Hasher:            hasher{},
	}

	channels := make([]chan Data, numChannels)
	members := make([]consistent.Member, numChannels)
	workerPools := make([]*pond.WorkerPool, numChannels)

	for i := range channels {
		channels[i] = make(chan Data, 100) // Buffer size of 100, adjust as needed
		members[i] = Member(fmt.Sprintf("channel-%d", i))
		workerPools[i] = pond.New(workersPerChannel, 1000) // 1000 is the task queue size
	}

	ring := consistent.New(members, cfg)

	return &TenantRouter{
		channels:       channels,
		consistentHash: ring,
		datafeedStatus: make(map[string]*DatafeedStatus),
		workerPools:    workerPools,
	}
}

func (tr *TenantRouter) Route(data Data) {
	key := data.Tenant + "-" + data.DatafeedID
	member := tr.consistentHash.LocateKey([]byte(key))
	channelIndex := int(member.(Member)[8] - '0') // Extract channel number from "channel-X"

	tr.mu.RLock()
	status, exists := tr.datafeedStatus[data.DatafeedID]
	tr.mu.RUnlock()

	if !exists {
		status = &DatafeedStatus{
			circuitBreaker: CircuitBreaker{
				threshold: 5,
				cooldown:  time.Minute,
			},
		}
		tr.mu.Lock()
		tr.datafeedStatus[data.DatafeedID] = status
		tr.mu.Unlock()
	}

	status.mu.Lock()
	defer status.mu.Unlock()

	if status.circuitBreaker.failures >= status.circuitBreaker.threshold {
		if time.Since(status.circuitBreaker.lastFail) > status.circuitBreaker.cooldown {
			status.circuitBreaker.failures = 0
		} else {
			fmt.Printf("Dropping data for datafeed %s due to circuit breaker\n", data.DatafeedID)
			return
		}
	}

	tr.channels[channelIndex] <- data
}

func (tr *TenantRouter) ReportFailure(datafeedID string) {
	tr.mu.RLock()
	status, exists := tr.datafeedStatus[datafeedID]
	tr.mu.RUnlock()

	if !exists {
		return
	}

	status.mu.Lock()
	defer status.mu.Unlock()

	status.circuitBreaker.failures++
	status.circuitBreaker.lastFail = time.Now()
}

func (tr *TenantRouter) processData(data Data, workerID int) {
	// Simulate processing
	fmt.Printf("Worker %d processing data for tenant %s, datafeed %s: %s\n", workerID, data.Tenant, data.DatafeedID, data.Info)

	// Simulate random failures
	if hash(data.DatafeedID)%10 == 0 {
		fmt.Printf("Worker %d failed processing datafeed %s\n", workerID, data.DatafeedID)
		tr.ReportFailure(data.DatafeedID)
	}
}

func (tr *TenantRouter) startWorkers(done chan bool) {
	for i, pool := range tr.workerPools {
		go func(channelIndex int, workerPool *pond.WorkerPool) {
			for data := range tr.channels[channelIndex] {
				workerPool.Submit(func() {
					tr.processData(data, channelIndex)
				})
			}
			workerPool.StopAndWait()
			done <- true
		}(i, pool)
	}

	// Work stealing
	go func() {
		for {
			for i, pool := range tr.workerPools {
				if pool.IdleWorkers() > 0 {
					// Try to steal work from other channels
					for j, otherChannel := range tr.channels {
						if i != j {
							select {
							case data, ok := <-otherChannel:
								if !ok {
									return // Channel closed, exit the goroutine
								}
								pool.Submit(func() {
									tr.processData(data, i)
								})
							default:
								// No work to steal from this channel, continue to the next
							}
						}
					}
				}
			}
			time.Sleep(time.Millisecond * 10) // Avoid tight loop
		}
	}()
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func main() {
	numChannels := 5
	workersPerChannel := 3

	router := NewTenantRouter(numChannels, workersPerChannel)

	done := make(chan bool, numChannels)
	router.startWorkers(done)

	// Simulate incoming data
	tenants := []string{"A", "B", "C", "D", "E"}
	datafeeds := []string{"1", "2", "3", "4", "5"}
	for i := 0; i < 1000; i++ {
		data := Data{
			Tenant:     tenants[hash(fmt.Sprintf("tenant-%d", i))%uint32(len(tenants))],
			DatafeedID: datafeeds[hash(fmt.Sprintf("datafeed-%d", i))%uint32(len(datafeeds))],
			Info:       fmt.Sprintf("Info %d", i),
		}
		router.Route(data)
		time.Sleep(time.Millisecond * 10) // Simulate some delay between data arrivals
	}

	// Close channels to signal workers to finish
	for _, ch := range router.channels {
		close(ch)
	}

	// Wait for all worker pools to finish
	for i := 0; i < numChannels; i++ {
		<-done
	}
}
