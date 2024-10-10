package main

import (
	"fmt"
	"math/rand"
	"sync"
)

type Data struct {
	Tenant string
	Info   string
}

type TenantRouter struct {
	channels     []chan Data
	tenantToChar map[string]int
	mu           sync.Mutex
}

func NewTenantRouter(numChannels int) *TenantRouter {
	channels := make([]chan Data, numChannels)
	for i := range channels {
		channels[i] = make(chan Data, 100) // Buffer size of 100, adjust as needed
	}
	return &TenantRouter{
		channels:     channels,
		tenantToChar: make(map[string]int),
	}
}

func (tr *TenantRouter) Route(data Data) {
	tr.mu.Lock()
	channelIndex, exists := tr.tenantToChar[data.Tenant]
	if !exists {
		channelIndex = rand.Intn(len(tr.channels))
		tr.tenantToChar[data.Tenant] = channelIndex
	}
	tr.mu.Unlock()

	tr.channels[channelIndex] <- data
}

func worker(id int, channel <-chan Data, wg *sync.WaitGroup) {
	defer wg.Done()
	for data := range channel {
		// Simulate processing
		fmt.Printf("Worker %d processing data for tenant %s: %s\n", id, data.Tenant, data.Info)
	}
}

func main() {
	numChannels := 5
	workersPerChannel := 3

	router := NewTenantRouter(numChannels)

	var wg sync.WaitGroup
	for i := 0; i < numChannels; i++ {
		for j := 0; j < workersPerChannel; j++ {
			wg.Add(1)
			go worker(i*workersPerChannel+j, router.channels[i], &wg)
		}
	}

	// Simulate incoming data
	tenants := []string{"A", "B", "C", "D", "E"}
	for i := 0; i < 100; i++ {
		data := Data{
			Tenant: tenants[rand.Intn(len(tenants))],
			Info:   fmt.Sprintf("Info %d", i),
		}
		router.Route(data)
	}

	// Close channels to signal workers to finish
	for _, ch := range router.channels {
		close(ch)
	}

	wg.Wait()
}
