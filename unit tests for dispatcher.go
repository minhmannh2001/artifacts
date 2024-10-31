package dispatcher

import (
	"datafeedctl/internal/app/jobworker/worker/containerpool"
	"datafeedctl/internal/app/jobworker/worker/jobhandler"
	"datafeedctl/internal/app/jobworker/worker/shared"
	"datafeedctl/internal/app/jobworker/worker/tokenstore"
	"encoding/json"
	"errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
	"time"
)

// Mock implementations
type MockContainer struct {
	mock.Mock
}

func (m *MockContainer) Run(data shared.DatafeedJob, tokens tokenstore.TenantTokens) (shared.DatafeedOutput, error) {
	args := m.Called(data, tokens)
	return args.Get(0).(shared.DatafeedOutput), args.Error(1)
}

type MockContainerPool struct {
	mock.Mock
}

func (m *MockContainerPool) GetContainer() containerpool.Container {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(containerpool.Container)
}

func (m *MockContainerPool) ReleaseContainer(container containerpool.Container) {
	m.Called(container)
}

func (m *MockContainerPool) StopAndRemoveContainers() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockContainerPool) CloseClient() error {
	args := m.Called()
	return args.Error(0)
}

type MockJobHandler struct {
	mock.Mock
}

func (m *MockJobHandler) PreprocessDatafeed(data shared.DatafeedJob) (*jobhandler.JobInfo, error) {
	args := m.Called(data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*jobhandler.JobInfo), args.Error(1)
}

func (m *MockJobHandler) PostprocessDatafeed(jobInfo *jobhandler.JobInfo, output shared.DatafeedOutput) error {
	args := m.Called(jobInfo, output)
	return args.Error(0)
}

func setupTest(t *testing.T) (*Dispatcher, *MockContainerPool, *MockJobHandler) {
	// Set up viper configuration
	viper.Set("worker.minimum_containers", 1)
	viper.Set("worker.maximum_containers", 10)
	viper.Set("worker.container_idle_timeout", time.Minute)

	mockContainerPool := new(MockContainerPool)
	mockJobHandler := new(MockJobHandler)

	dispatcher := &Dispatcher{
		channel:        make(chan shared.DatafeedJob),
		datafeedStatus: make(map[string]*DatafeedStatus),
		workerPool:     nil, // We'll use a small pool for testing
		containerPool:  mockContainerPool,
		done:          make(chan bool),
		jobHandler:    mockJobHandler,
		tokenStore:    tokenstore.NewTokenStore(),
	}
	dispatcher.startWorkers()

	return dispatcher, mockContainerPool, mockJobHandler
}

func TestDispatcher_Dispatch_CircuitBreakerOpen(t *testing.T) {
	dispatcher, _, _ := setupTest(t)

	// Create a datafeed status with an open circuit breaker
	status := &DatafeedStatus{
		circuitBreaker: CircuitBreaker{
			failures:  10,
			threshold: 5,
			lastFail:  time.Now(),
			cooldown:  time.Minute,
		},
	}
	
	datafeedID := "test-feed"
	dispatcher.datafeedStatus[datafeedID] = status

	// Dispatch a job
	job := shared.DatafeedJob{
		DatafeedID: datafeedID,
		Name:      "test",
		TaskID:    "task1",
		RequestID: "req1",
	}

	// The job should be dropped due to open circuit breaker
	dispatcher.Dispatch(job)
	
	// Add a small delay to ensure the job would have been processed if not dropped
	time.Sleep(100 * time.Millisecond)
	
	// Verify the job was dropped (no calls to process it)
	// This is implicit since we didn't set up any expectations on the mocks
}

func TestDispatcher_ProcessData_Success(t *testing.T) {
	dispatcher, mockContainerPool, mockJobHandler := setupTest(t)

	mockContainer := new(MockContainer)
	job := shared.DatafeedJob{
		DatafeedID: "test-feed",
		Name:      "test",
		TaskID:    "task1",
		RequestID: "req1",
		Tenant:    "tenant1",
	}

	expectedOutput := shared.DatafeedOutput{
		Name:      job.Name,
		TaskId:    job.TaskID,
		RequestID: job.RequestID,
		Payload:   `{"data": "success"}`,
	}

	jobInfo := &jobhandler.JobInfo{ID: "test-job"}

	// Set up expectations
	mockJobHandler.On("PreprocessDatafeed", job).Return(jobInfo, nil)
	mockContainerPool.On("GetContainer").Return(mockContainer)
	mockContainer.On("Run", job, mock.Anything).Return(expectedOutput, nil)
	mockContainerPool.On("ReleaseContainer", mockContainer).Return()
	mockJobHandler.On("PostprocessDatafeed", jobInfo, expectedOutput).Return(nil)

	// Process the job
	dispatcher.processData(job)

	// Verify expectations
	mockJobHandler.AssertExpectations(t)
	mockContainerPool.AssertExpectations(t)
	mockContainer.AssertExpectations(t)
}

func TestDispatcher_ProcessData_Error(t *testing.T) {
	dispatcher, mockContainerPool, mockJobHandler := setupTest(t)

	mockContainer := new(MockContainer)
	job := shared.DatafeedJob{
		DatafeedID: "test-feed",
		Name:      "test",
		TaskID:    "task1",
		RequestID: "req1",
		Tenant:    "tenant1",
	}

	expectedError := errors.New("container error")
	jobInfo := &jobhandler.JobInfo{ID: "test-job"}

	// Set up expectations
	mockJobHandler.On("PreprocessDatafeed", job).Return(jobInfo, nil)
	mockContainerPool.On("GetContainer").Return(mockContainer)
	mockContainer.On("Run", job, mock.Anything).Return(shared.DatafeedOutput{}, expectedError)
	mockContainerPool.On("ReleaseContainer", mockContainer).Return()

	// Expect error output to be processed
	errorOutput := createDatafeedErrorOutput(job, expectedError)
	mockJobHandler.On("PostprocessDatafeed", jobInfo, errorOutput).Return(nil)

	// Process the job
	dispatcher.processData(job)

	// Verify expectations
	mockJobHandler.AssertExpectations(t)
	mockContainerPool.AssertExpectations(t)
	mockContainer.AssertExpectations(t)

	// Verify circuit breaker state
	status := dispatcher.getDatafeedStatus(job.DatafeedID)
	assert.Equal(t, 1, status.circuitBreaker.failures)
}

func TestDispatcher_Stop(t *testing.T) {
	dispatcher, mockContainerPool, _ := setupTest(t)

	// Set up expectations
	mockContainerPool.On("StopAndRemoveContainers").Return(nil)
	mockContainerPool.On("CloseClient").Return(nil)

	// Stop the dispatcher
	err := dispatcher.Stop()

	// Verify expectations
	assert.NoError(t, err)
	assert.True(t, dispatcher.stopped)
	mockContainerPool.AssertExpectations(t)
}

func TestCreateDatafeedErrorOutput(t *testing.T) {
	job := shared.DatafeedJob{
		Name:      "test",
		TaskID:    "task1",
		RequestID: "req1",
	}
	err := errors.New("test error")

	output := createDatafeedErrorOutput(job, err)

	// Verify the output structure
	assert.Equal(t, job.Name, output.Name)
	assert.Equal(t, job.TaskID, output.TaskId)
	assert.Equal(t, job.RequestID, output.RequestID)

	// Verify the payload contains the error
	var payload map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(output.Payload), &payload))
	assert.Equal(t, float64(2), payload["Type"])
	assert.Equal(t, err.Error(), payload["Contents"])
}