package containerpool

import (
	"context"
	"datafeedctl/internal/app/logz"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
	"reflect"
	"testing"
	"time"
)

func TestNewContainerPool(t *testing.T) {
	tests := []struct {
		name        string
		minSize     int
		maxSize     int
		idleTimeout time.Duration
		imageName   string
		wantErr     bool
	}{
		{
			name:        "valid configuration",
			minSize:     2,
			maxSize:     5,
			idleTimeout: time.Minute * 10,
			imageName:   "test/image",
			wantErr:     false,
		},
		{
			name:        "invalid configuration (min > max)",
			minSize:     6,
			maxSize:     4,
			idleTimeout: time.Minute * 10,
			imageName:   "test/image",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewContainerPool(tt.minSize, tt.maxSize, tt.idleTimeout, tt.imageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewContainerPool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestContainerPool_GetContainer(t *testing.T) {
	// Mock the Docker client
	mockClient := &mockDockerClient{}
	viper.Set("network.name", "test-network")
	viper.Set("network.dns", []string{"8.8.8.8"})
	viper.Set("network.dns_search", []string{"example.com"})
	viper.Set("network.hosts", []string{"host1:192.168.1.100"})
	viper.Set("environments", map[string]interface{}{"ENV_VAR": "value"})
	viper.Set("agentCert.path", "/path/to/cert")

	cp, _ := NewContainerPool(2, 5, time.Minute*10, "test/image")
	cp.client = mockClient

	tests := []struct {
		name                 string
		containerAliveStatus []bool
		wantErr              bool
	}{
		{
			name:                 "all containers alive",
			containerAliveStatus: []bool{true, true, true},
			wantErr:              false,
		},
		{
			name:                 "one container dead",
			containerAliveStatus: []bool{true, false, true},
			wantErr:              false,
		},
		{
			name:                 "all containers dead",
			containerAliveStatus: []bool{false, false, false},
			wantErr:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.aliveStatus = tt.containerAliveStatus
			con := cp.GetContainer()
			if (con == nil) != tt.wantErr {
				t.Errorf("GetContainer() error = %v, wantErr %v", con, tt.wantErr)
			}
		})
	}
}

func TestContainerPool_ReleaseContainer(t *testing.T) {
	cp, _ := NewContainerPool(2, 5, time.Minute*10, "test/image")

	// Create some containers
	con1 := &DockerContainer{ID: "container1", State: Busy}
	con2 := &DockerContainer{ID: "container2", State: Free}

	// Release the containers
	cp.ReleaseContainer(con1)
	cp.ReleaseContainer(con2)

	// Check the state of the containers
	if con1.State != Free {
		t.Errorf("ReleaseContainer() did not set con1 state to Free")
	}
	if con2.State != Free {
		t.Errorf("ReleaseContainer() did not preserve con2 state as Free")
	}

	// Check the availability of the containers
	select {
	case c := <-cp.availableContainers:
		if c.ID != con1.ID {
			t.Errorf("ReleaseContainer() did not add con1 to the available containers channel")
		}
	default:
		t.Errorf("ReleaseContainer() did not add con1 to the available containers channel")
	}

	select {
	case c := <-cp.availableContainers:
		if c.ID != con2.ID {
			t.Errorf("ReleaseContainer() did not preserve con2 in the available containers channel")
		}
	default:
		t.Errorf("ReleaseContainer() did not preserve con2 in the available containers channel")
	}
}

func TestContainerPool_CheckContainerAlive(t *testing.T) {
	// Mock the Docker client
	mockClient := &mockDockerClient{}
	cp, _ := NewContainerPool(2, 5, time.Minute*10, "test/image")
	cp.client = mockClient

	tests := []struct {
		name            string
		container       *DockerContainer
		containerAlive  bool
		wantNewContainer bool
	}{
		{
			name:            "container alive",
			container:       &DockerContainer{ID: "container1"},
			containerAlive:  true,
			wantNewContainer: false,
		},
		{
			name:            "container dead",
			container:       &DockerContainer{ID: "container2"},
			containerAlive:  false,
			wantNewContainer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient.aliveStatus = []bool{tt.containerAlive}
			newContainer := cp.CheckContainerAlive(tt.container)
			if (newContainer != nil) != tt.wantNewContainer {
				t.Errorf("CheckContainerAlive() = %v, want new container: %v", newContainer, tt.wantNewContainer)
			}
		})
	}
}

func TestContainerPool_cleanupIdleContainers(t *testing.T) {
	cp, _ := NewContainerPool(2, 5, time.Minute*5, "test/image")

	// Add some containers to the pool
	con1 := &DockerContainer{ID: "container1", State: Free}
	con2 := &DockerContainer{ID: "container2", State: Busy}
	con3 := &DockerContainer{ID: "container3", State: Free}
	cp.containersList = []*DockerContainer{con1, con2, con3}
	cp.lastUsedTime = map[string]time.Time{
		"container1": time.Now().Add(-time.Minute * 7),
		"container2": time.Now().Add(-time.Minute * 3),
		"container3": time.Now().Add(-time.Minute * 6),
	}

	// Run the cleanup
	cp.cleanupIdleContainers()

	// Check the container list
	if len(cp.containersList) != 3 {
		t.Errorf("cleanupIdleContainers() removed too many containers, expected 3, got %d", len(cp.containersList))
	}

	if _, ok := cp.lastUsedTime["container1"]; ok {
		t.Errorf("cleanupIdleContainers() did not remove idle container container1")
	}
	if _, ok := cp.lastUsedTime["container2"]; !ok {
		t.Errorf("cleanupIdleContainers() removed active container container2")
	}
	if _, ok := cp.lastUsedTime["container3"]; ok {
		t.Errorf("cleanupIdleContainers() removed container3 when it should not have")
	}
}

// Mock Docker client
type mockDockerClient struct {
	aliveStatus []bool
	client.Client
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *container.Platform, containerName string) (container.CreateResponse, error) {
	return container.CreateResponse{ID: "mock-container-id"}, nil
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, container string, options container.StartOptions) error {
	return nil
}

func (m *mockDockerClient) ContainerAttach(ctx context.Context, container string, options container.AttachOptions) (container.ContainerAttachResponse, error) {
	return container.ContainerAttachResponse{
		Conn:   &mockConn{},
		Reader: &mockReader{},
	}, nil
}

func (m *mockDockerClient) ContainerRemove(ctx context.Context, container string, options container.RemoveOptions) error {
	return nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

type mockConn struct{}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type mockReader struct{}

func (m *mockReader) Read(p []byte) (n int, err error) {
	return 0, nil
}

func TestMain(m *testing.M) {
	// Set up logging for the tests
	logz.Init(logz.DebugLevel, "")
	m.Run()
}