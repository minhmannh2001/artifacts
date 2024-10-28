package containerpool

import (
	"bufio"
	"context"
	"datafeedctl/internal/app/logz"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
)

type ContainerPool struct {
	containersList      []*DockerContainer
	availableContainers chan *DockerContainer
	client             *client.Client
	imageName          string
	mu                 sync.Mutex
	
	minContainers      int
	maxContainers      int
	idleTimeout        time.Duration
	lastUsedTime       map[string]time.Time
}

type DockerContainer struct {
	ID     string
	Stdin  *bufio.Writer
	Stdout *bufio.Scanner
	State  ContainerState
}

func NewContainerPool(minSize, maxSize int, idleTimeout time.Duration, imageName string) (*ContainerPool, error) {
	if minSize > maxSize {
		return nil, fmt.Errorf("minimum size cannot be greater than maximum size")
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	pool := &ContainerPool{
		availableContainers: make(chan *DockerContainer, maxSize),
		client:             cli,
		containersList:     make([]*DockerContainer, 0, maxSize),
		imageName:          imageName,
		minContainers:      minSize,
		maxContainers:      maxSize,
		idleTimeout:        idleTimeout,
		lastUsedTime:       make(map[string]time.Time),
	}

	// Initialize with minimum number of containers
	for i := 0; i < minSize; i++ {
		con, err := pool.createContainer()
		if err != nil {
			pool.cleanupContainers()
			return nil, fmt.Errorf("failed to create container: %v", err)
		}
		pool.availableContainers <- con
		pool.containersList = append(pool.containersList, con)
		pool.lastUsedTime[con.ID] = time.Now()
	}

	// Start the cleanup goroutine
	go pool.cleanupIdleContainers()

	return pool, nil
}

func (cp *ContainerPool) GetContainer() *DockerContainer {
	cp.mu.Lock()
	currentSize := len(cp.containersList)
	cp.mu.Unlock()

	// Try to get an available container
	select {
	case con := <-cp.availableContainers:
		if cp.CheckContainerAlive(con) == nil {
			return cp.GetContainer()
		}
		cp.lastUsedTime[con.ID] = time.Now()
		con.State = Busy
		return con
	default:
		// No available containers, create new one if possible
		if currentSize < cp.maxContainers {
			cp.mu.Lock()
			newContainer, err := cp.createContainer()
			if err != nil {
				cp.mu.Unlock()
				logz.Error("Failed to create new container")
				return nil
			}
			cp.containersList = append(cp.containersList, newContainer)
			cp.lastUsedTime[newContainer.ID] = time.Now()
			cp.mu.Unlock()
			
			newContainer.State = Busy
			return newContainer
		}

		// Wait for an available container if at max capacity
		con := <-cp.availableContainers
		if cp.CheckContainerAlive(con) == nil {
			return cp.GetContainer()
		}
		cp.lastUsedTime[con.ID] = time.Now()
		con.State = Busy
		return con
	}
}

func (cp *ContainerPool) ReleaseContainer(con *DockerContainer) {
	if con != nil && con.State == Busy {
		con.State = Free
		cp.lastUsedTime[con.ID] = time.Now()
		cp.availableContainers <- con
	}
}

func (cp *ContainerPool) cleanupIdleContainers() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cp.mu.Lock()
		if len(cp.containersList) <= cp.minContainers {
			cp.mu.Unlock()
			continue
		}

		now := time.Now()
		var containersToRemove []string
		
		// Identify idle containers
		for _, con := range cp.containersList {
			if con.State == Free {
				if lastUsed, exists := cp.lastUsedTime[con.ID]; exists {
					if now.Sub(lastUsed) > cp.idleTimeout && len(cp.containersList) > cp.minContainers {
						containersToRemove = append(containersToRemove, con.ID)
					}
				}
			}
		}

		// Remove idle containers
		for _, id := range containersToRemove {
			cp.removeContainer(id)
		}
		
		cp.mu.Unlock()
	}
}

func (cp *ContainerPool) removeContainer(id string) {
	ctx := context.Background()
	err := cp.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	if err != nil {
		logz.Error(fmt.Sprintf("failed to remove container %s: %v", id, err))
		return
	}

	// Update containersList
	newList := make([]*DockerContainer, 0, len(cp.containersList)-1)
	for _, con := range cp.containersList {
		if con.ID != id {
			newList = append(newList, con)
		}
	}
	cp.containersList = newList
	delete(cp.lastUsedTime, id)
}

// Rest of the methods remain the same...