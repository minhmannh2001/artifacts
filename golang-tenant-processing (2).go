package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/alitto/pond"
	"github.com/buraksezer/consistent"
	"github.com/cespare/xxhash"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Data struct {
	Tenant     string `json:"tenant"`
	DatafeedID string `json:"datafeed_id"`
	Info       string `json:"info"`
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

type DockerContainer struct {
	ID     string
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

type ContainerPool struct {
	containers chan *DockerContainer
	client     *client.Client
}

func NewContainerPool(poolSize int, imageName string) (*ContainerPool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	pool := &ContainerPool{
		containers: make(chan *DockerContainer, poolSize),
		client:     cli,
	}

	for i := 0; i < poolSize; i++ {
		container, err := pool.createContainer(imageName)
		if err != nil {
			return nil, fmt.Errorf("failed to create container: %v", err)
		}
		pool.containers <- container
	}

	return pool, nil
}

func (cp *ContainerPool) createContainer(imageName string) (*DockerContainer, error) {
	ctx := context.Background()
	resp, err := cp.client.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		Tty:   true,
		OpenStdin: true,
	}, nil, nil, nil, "")
	if err != nil {
		return nil, err
	}

	if err := cp.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, err
	}

	attachOptions := types.ContainerAttachOptions{
		Stdin:  true,
		Stdout: true,
		Stream: true,
	}
	conn, err := cp.client.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		return nil, err
	}

	return &DockerContainer{
		ID:     resp.ID,
		Stdin:  conn.Conn,
		Stdout: conn.Reader,
	}, nil
}

func (cp *ContainerPool) GetContainer() *DockerContainer {
	return <-cp.containers
}

func (cp *ContainerPool) ReleaseContainer(container *DockerContainer) {
	cp.containers <- container
}

type TenantRouter struct {
	channels        []chan Data
	consistentHash  *consistent.Consistent
	datafeedStatus  map[string]*DatafeedStatus
	mu              sync.RWMutex
	workerPools     []*pond.WorkerPool
	containerPool   *ContainerPool
}

func NewTenantRouter(numChannels, workersPerChannel, containerPoolSize int, imageName string) (*TenantRouter, error) {
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
		channels[i] = make(chan Data, 100)
		members[i] = Member(fmt.Sprintf("channel-%d", i))
		workerPools[i] = pond.New(workersPerChannel, 1000)
	}

	ring := consistent.New(members, cfg)

	containerPool, err := NewContainerPool(containerPoolSize, imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container pool: %v", err)
	}

	return &TenantRouter{
		channels:       channels,
		consistentHash: ring,
		datafeedStatus: make(map[string]*DatafeedStatus),
		workerPools:    workerPools,
		containerPool:  containerPool,
	}, nil
}

func (tr *TenantRouter) Route(data Data) {
	key := data.Tenant + "-" + data.DatafeedID
	member := tr.consistentHash.LocateKey([]byte(key))
	channelIndex := int(member.(Member)[8] - '0')

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
	container := tr.containerPool.GetContainer()
	defer tr.containerPool.ReleaseContainer(container)

	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("Error marshaling data: %v\n", err)
		return
	}

	_, err = container.Stdin.Write(append(jsonData, '\n'))
	if err != nil {
		fmt.Printf("Error writing to container stdin: %v\n", err)
		return
	}

	scanner := bufio.NewScanner(container.Stdout)
	if scanner.Scan() {
		output := scanner.Text()
		fmt.Printf("Worker %d processed data for tenant %s, datafeed %s: %s\n", workerID, data.Tenant, data.DatafeedID, output)
	} else {
		fmt.Printf("Error reading from container stdout: %v\n", scanner.Err())
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
					for j, otherChannel := range tr.channels {
						if i != j {
							select {
							case data, ok := <-otherChannel:
								if !ok {
									return
								}
								pool.Submit(func() {
									tr.processData(data, i)
								})
							default:
							}
						}
					}
				}
			}
			time.Sleep(time.Millisecond * 10)
		}
	}()
}

func main() {
	numChannels := 5
	workersPerChannel := 3
	containerPoolSize := 10
	imageName := "your-docker-image:latest" // Replace with your actual Docker image

	router, err := NewTenantRouter(numChannels, workersPerChannel, containerPoolSize, imageName)
	if err != nil {
		fmt.Printf("Error creating router: %v\n", err)
		return
	}

	done := make(chan bool, numChannels)
	router.startWorkers(done)

	// Simulate incoming data
	tenants := []string{"A", "B", "C", "D", "E"}
	datafeeds := []string{"1", "2", "3", "4", "5"}
	for i := 0; i < 1000; i++ {
		data := Data{
			Tenant:     tenants[i%len(tenants)],
			DatafeedID: datafeeds[i%len(datafeeds)],
			Info:       fmt.Sprintf("Info %d", i),
		}
		router.Route(data)
		time.Sleep(time.Millisecond * 10)
	}

	for _, ch := range router.channels {
		close(ch)
	}

	for i := 0; i < numChannels; i++ {
		<-done
	}
}
