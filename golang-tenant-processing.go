package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ... (previous imports and type definitions)

type ContainerPool struct {
	containers     chan *DockerContainer
	client         *client.Client
	mu             sync.Mutex
	imageName      string
	minContainers  int
	maxContainers  int
	idleThreshold  time.Duration
	scalingTicker  *time.Ticker
	stopScaling    chan struct{}
}

func NewContainerPool(minContainers, maxContainers int, imageName string) (*ContainerPool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	pool := &ContainerPool{
		containers:     make(chan *DockerContainer, maxContainers),
		client:         cli,
		imageName:      imageName,
		minContainers:  minContainers,
		maxContainers:  maxContainers,
		idleThreshold:  5 * time.Minute,
		scalingTicker:  time.NewTicker(30 * time.Second),
		stopScaling:    make(chan struct{}),
	}

	for i := 0; i < minContainers; i++ {
		container, err := pool.createContainer()
		if err != nil {
			return nil, fmt.Errorf("failed to create initial container: %v", err)
		}
		pool.containers <- container
	}

	go pool.scaleContainers()

	return pool, nil
}

func (cp *ContainerPool) scaleContainers() {
	for {
		select {
		case <-cp.scalingTicker.C:
			cp.adjustContainerCount()
		case <-cp.stopScaling:
			return
		}
	}
}

func (cp *ContainerPool) adjustContainerCount() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	currentCount := len(cp.containers)
	if currentCount < cp.minContainers {
		cp.scaleUp(cp.minContainers - currentCount)
	} else if currentCount > cp.maxContainers {
		cp.scaleDown(currentCount - cp.maxContainers)
	} else {
		// Check for idle containers
		idleCount := 0
		now := time.Now()
		for i := 0; i < len(cp.containers); i++ {
			container := <-cp.containers
			if now.Sub(container.LastUsed) > cp.idleThreshold && len(cp.containers) > cp.minContainers {
				cp.removeContainer(container)
				idleCount++
			} else {
				cp.containers <- container
			}
		}
		if idleCount > 0 {
			fmt.Printf("Removed %d idle containers\n", idleCount)
		}
	}
}

func (cp *ContainerPool) scaleUp(count int) {
	for i := 0; i < count; i++ {
		container, err := cp.createContainer()
		if err != nil {
			fmt.Printf("Failed to create container during scale up: %v\n", err)
			continue
		}
		cp.containers <- container
	}
	fmt.Printf("Scaled up %d containers\n", count)
}

func (cp *ContainerPool) scaleDown(count int) {
	for i := 0; i < count; i++ {
		if len(cp.containers) > cp.minContainers {
			container := <-cp.containers
			cp.removeContainer(container)
		}
	}
	fmt.Printf("Scaled down %d containers\n", count)
}

func (cp *ContainerPool) removeContainer(container *DockerContainer) {
	ctx := context.Background()
	err := cp.client.ContainerStop(ctx, container.ID, container.StopOptions{})
	if err != nil {
		fmt.Printf("Failed to stop container %s: %v\n", container.ID, err)
	}
	err = cp.client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{})
	if err != nil {
		fmt.Printf("Failed to remove container %s: %v\n", container.ID, err)
	}
}

func (cp *ContainerPool) GetContainer() *DockerContainer {
	container := <-cp.containers
	container.LastUsed = time.Now()
	return container
}

func (cp *ContainerPool) ReleaseContainer(container *DockerContainer) {
	cp.containers <- container
}

func (cp *ContainerPool) Stop() error {
	close(cp.stopScaling)
	cp.scalingTicker.Stop()

	for {
		select {
		case container := <-cp.containers:
			cp.removeContainer(container)
		default:
			return cp.client.Close()
		}
	}
}

type DockerContainer struct {
	ID        string
	Stdin     io.WriteCloser
	Stdout    io.ReadCloser
	LastUsed  time.Time
}

// Update TenantRouter to use the new ContainerPool
type TenantRouter struct {
	// ... (previous fields)
	containerPool *ContainerPool
}

func NewTenantRouter(numChannels, workersPerChannel, minContainers, maxContainers int, imageName string) (*TenantRouter, error) {
	// ... (previous initialization)

	containerPool, err := NewContainerPool(minContainers, maxContainers, imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container pool: %v", err)
	}

	return &TenantRouter{
		// ... (previous fields)
		containerPool: containerPool,
	}, nil
}

func (tr *TenantRouter) Stop() error {
	// ... (previous stop logic)

	// Stop the container pool
	return tr.containerPool.Stop()
}

// Update main function
func main() {
	numChannels := 5
	workersPerChannel := 3
	minContainers := 5
	maxContainers := 20
	imageName := "your-docker-image:latest"

	router, err := NewTenantRouter(numChannels, workersPerChannel, minContainers, maxContainers, imageName)
	if err != nil {
		fmt.Printf("Error creating router: %v\n", err)
		return
	}

	// ... (rest of the main function)
}
