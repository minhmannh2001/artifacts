package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
)

type ContainerPool struct {
	client       *client.Client
	containers   map[string]*ContainerInfo
	mutex        sync.Mutex
	maxSize      int
	freePool     chan string
	processedTenants map[string]bool
}

// ... (previous ContainerInfo struct and other methods remain the same)

func NewContainerPool(maxSize int) (*ContainerPool, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	return &ContainerPool{
		client:     cli,
		maxSize:    maxSize,
		containers: make(map[string]*ContainerInfo),
		freePool:   make(chan string, maxSize),
		processedTenants: make(map[string]bool),
	}, nil
}

func (p *ContainerPool) ListenForTenants(tenantChan <-chan string) {
	for tenant := range tenantChan {
		p.mutex.Lock()
		if !p.processedTenants[tenant] {
			p.processedTenants[tenant] = true
			p.mutex.Unlock()
			go p.createContainersForTenant(tenant)
		} else {
			p.mutex.Unlock()
		}
	}
}

func (p *ContainerPool) createContainersForTenant(tenant string) {
	numInstances := viper.GetInt("worker.numberOfInstances")
	for i := 0; i < numInstances; i++ {
		containerName := fmt.Sprintf("datafeed_worker_%s_tenant_%s", strconv.Itoa(i), tenant)
		err := p.createContainer(containerName, tenant)
		if err != nil {
			log.Printf("Failed to create container for tenant %s: %v", tenant, err)
		}
	}
}

func (p *ContainerPool) createContainer(name, tenant string) error {
	ctx := context.Background()
	
	// You may want to customize this configuration based on your needs
	config := &container.Config{
		Image: viper.GetString("worker.image"),
		Env: []string{fmt.Sprintf("TENANT=%s", tenant)},
	}
	
	hostConfig := &container.HostConfig{}
	
	resp, err := p.client.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	if err := p.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	p.mutex.Lock()
	p.containers[resp.ID] = &ContainerInfo{
		ID: resp.ID,
		State: Free,
	}
	p.freePool <- resp.ID
	p.mutex.Unlock()

	log.Printf("Created and started container %s for tenant %s", name, tenant)
	return nil
}

func main() {
	// Initialize viper configuration
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	pool, err := NewContainerPool(viper.GetInt("worker.maxPoolSize"))
	if err != nil {
		log.Fatalf("Failed to create container pool: %v", err)
	}

	// Create a channel for receiving tenants
	tenantChan := make(chan string)

	// Start the goroutine to listen for tenants
	go pool.ListenForTenants(tenantChan)

	// Example: Send some tenants to the channel
	tenants := []string{"tenant1", "tenant2", "tenant3"}
	for _, tenant := range tenants {
		tenantChan <- tenant
	}

	// In a real application, you would keep the main function running
	// Here, we'll use a simple wait to see the output
	select {}
}
