package main

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/buraksezer/consistent"
	"github.com/cespare/xxhash"
)

type Data struct {
	Tenant    string
	DatafeedID string
	Info      string
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
}

func NewTenantRouter(numChannels int) *TenantRouter {
	cfg := consistent.Config{
		PartitionCount:    271,
		ReplicationFactor: 20,
		Load:              1.25,
		Hasher:            hasher{},
	}

	channels := make([]chan Data, numChannels)
	members := make([]consistent.Member, numChannels)
	for i := range channels {
		channels[i] = make(chan Data, 100) // Buffer size of 100, adjust as needed
		members[i] = Member(fmt.Sprintf("channel-%d", i))
	}

	ring := consistent.New(members, cfg)

	return &TenantRouter{
		channels:       channels,
		consistentHash: ring,
		datafeedStatus: make(map[string]*DatafeedStatus),
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

func worker(id int, channel <-chan Data, router *TenantRouter, wg *sync.WaitGroup) {
	defer wg.Done()
	for data := range channel {
		// Simulate processing
		fmt.Printf("Worker %d processing data for tenant %s, datafeed %s: %s\n", id, data.Tenant, data.DatafeedID, data.Info)
		
		// Simulate random failures
		if hash(data.DatafeedID) % 10 == 0 {
			fmt.Printf("Worker %d failed processing datafeed %s\n", id, data.DatafeedID)
			router.ReportFailure(data.DatafeedID)
		}
	}
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func main() {
	numChannels := 5
	workersPerChannel := 3

	router := NewTenantRouter(numChannels)

	var wg sync.WaitGroup
	for i := 0; i < numChannels; i++ {
		for j := 0; j < workersPerChannel; j++ {
			wg.Add(1)
			go worker(i*workersPerChannel+j, router.channels[i], router, &wg)
		}
	}

	// Simulate incoming data
	tenants := []string{"A", "B", "C", "D", "E"}
	datafeeds := []string{"1", "2", "3", "4", "5"}
	for i := 0; i < 1000; i++ {
		data := Data{
			Tenant:    tenants[hash(fmt.Sprintf("tenant-%d", i)) % uint32(len(tenants))],
			DatafeedID: datafeeds[hash(fmt.Sprintf("datafeed-%d", i)) % uint32(len(datafeeds))],
			Info:      fmt.Sprintf("Info %d", i),
		}
		router.Route(data)
		time.Sleep(time.Millisecond * 10) // Simulate some delay between data arrivals
	}

	// Close channels to signal workers to finish
	for _, ch := range router.channels {
		close(ch)
	}

	wg.Wait()
}
