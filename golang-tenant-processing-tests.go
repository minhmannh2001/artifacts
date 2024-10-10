package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Docker client
type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	args := m.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)
	return args.Get(0).(container.ContainerCreateCreatedBody), args.Error(1)
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerAttach(ctx context.Context, container string, options types.ContainerAttachOptions) (types.HijackedResponse, error) {
	args := m.Called(ctx, container, options)
	return args.Get(0).(types.HijackedResponse), args.Error(1)
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error {
	args := m.Called(ctx, containerID, options)
	return args.Error(0)
}

func (m *MockDockerClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Test NewContainerPool
func TestNewContainerPool(t *testing.T) {
	mockClient := new(MockDockerClient)
	client.NewClientWithOpts = func(ops ...client.Opt) (*client.Client, error) {
		return mockClient, nil
	}

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.ContainerCreateCreatedBody{ID: "test-container"}, nil)
	mockClient.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("ContainerAttach", mock.Anything, mock.Anything, mock.Anything).
		Return(types.HijackedResponse{}, nil)

	pool, err := NewContainerPool(5, "test-image")
	assert.NoError(t, err)
	assert.NotNil(t, pool)
	assert.Equal(t, 5, cap(pool.containers))
}

// Test TenantRouter.Route
func TestTenantRouterRoute(t *testing.T) {
	router, _ := NewTenantRouter(3, 2, 5, "test-image")
	data := Data{
		Tenant:     "A",
		DatafeedID: "1",
		Info:       "Test info",
	}

	router.Route(data)

	// Check if the data was routed to one of the channels
	var receivedData Data
	select {
	case receivedData = <-router.channels[0]:
	case receivedData = <-router.channels[1]:
	case receivedData = <-router.channels[2]:
	case <-time.After(time.Second):
		t.Fatal("Data was not routed to any channel")
	}

	assert.Equal(t, data, receivedData)
}

// Test TenantRouter.ReportFailure
func TestTenantRouterReportFailure(t *testing.T) {
	router, _ := NewTenantRouter(3, 2, 5, "test-image")
	datafeedID := "test-datafeed"

	router.ReportFailure(datafeedID)
	router.ReportFailure(datafeedID)

	router.mu.RLock()
	status, exists := router.datafeedStatus[datafeedID]
	router.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, 2, status.circuitBreaker.failures)
}

// Test TenantRouter.processData
func TestTenantRouterProcessData(t *testing.T) {
	mockClient := new(MockDockerClient)
	client.NewClientWithOpts = func(ops ...client.Opt) (*client.Client, error) {
		return mockClient, nil
	}

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.ContainerCreateCreatedBody{ID: "test-container"}, nil)
	mockClient.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("ContainerAttach", mock.Anything, mock.Anything, mock.Anything).
		Return(types.HijackedResponse{}, nil)

	router, _ := NewTenantRouter(3, 2, 5, "test-image")
	data := Data{
		Tenant:     "A",
		DatafeedID: "1",
		Info:       "Test info",
	}

	// Mock container stdin and stdout
	mockStdin := &mockReadWriteCloser{}
	mockStdout := &mockReadWriteCloser{
		readData: []byte("Processed data\n"),
	}

	router.containerPool.containers <- &DockerContainer{
		ID:     "test-container",
		Stdin:  mockStdin,
		Stdout: mockStdout,
	}

	router.processData(data, 0)

	// Check if data was written to stdin
	writtenData := mockStdin.writtenData
	var receivedData Data
	err := json.Unmarshal(writtenData[:len(writtenData)-1], &receivedData) // Remove trailing newline
	assert.NoError(t, err)
	assert.Equal(t, data, receivedData)
}

// Test TenantRouter.Stop
func TestTenantRouterStop(t *testing.T) {
	mockClient := new(MockDockerClient)
	client.NewClientWithOpts = func(ops ...client.Opt) (*client.Client, error) {
		return mockClient, nil
	}

	mockClient.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(container.ContainerCreateCreatedBody{ID: "test-container"}, nil)
	mockClient.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("ContainerAttach", mock.Anything, mock.Anything, mock.Anything).
		Return(types.HijackedResponse{}, nil)
	mockClient.On("ContainerStop", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockClient.On("Close").Return(nil)

	router, _ := NewTenantRouter(3, 2, 5, "test-image")

	err := router.Stop()
	assert.NoError(t, err)

	// Check if all channels are closed
	for _, ch := range router.channels {
		_, open := <-ch
		assert.False(t, open)
	}

	mockClient.AssertExpectations(t)
}

// Mock ReadWriteCloser for testing
type mockReadWriteCloser struct {
	readData    []byte
	writtenData []byte
}

func (m *mockReadWriteCloser) Read(p []byte) (n int, err error) {
	copy(p, m.readData)
	return len(m.readData), nil
}

func (m *mockReadWriteCloser) Write(p []byte) (n int, err error) {
	m.writtenData = append(m.writtenData, p...)
	return len(p), nil
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}
