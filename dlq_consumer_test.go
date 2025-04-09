package dlq

import (
    "context"
    "datafeedctl/internal/app/helpers"
    "datafeedctl/internal/app/poller/ingestor"
    "encoding/json"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "testing"
    "time"
)

// MockKafkaRepo is a mock implementation of KafkaRepoI
type MockKafkaRepo struct {
    mock.Mock
}

func (m *MockKafkaRepo) SendKafkaMessage(message []byte, topic string) error {
    args := m.Called(message, topic)
    return args.Error(0)
}

func (m *MockKafkaRepo) SubscribeTopics(topics []string, cb interface{}) error {
    args := m.Called(topics, cb)
    return args.Error(0)
}

func (m *MockKafkaRepo) ReadMessage(timeout time.Duration) (*kafka.Message, error) {
    args := m.Called(timeout)
    return args.Get(0).(*kafka.Message), args.Error(1)
}

// MockIngestor is a mock implementation of the Ingestor
type MockIngestor struct {
    mock.Mock
}

func (m *MockIngestor) InsertAlertBulk(tenant string) (int, error) {
    args := m.Called(tenant)
    return args.Int(0), args.Error(1)
}

func TestNewDLQConsumer(t *testing.T) {
    mockKafka := new(MockKafkaRepo)
    mockIngestor := new(MockIngestor)

    consumer := NewDLQConsumer(mockKafka, mockIngestor)

    assert.NotNil(t, consumer)
    assert.Equal(t, 3, consumer.maxRetries)
    assert.Equal(t, time.Minute*5, consumer.retryBackoff)
}

func TestDLQConsumer_RetryAlert(t *testing.T) {
    mockKafka := new(MockKafkaRepo)
    mockIngestor := new(MockIngestor)
    consumer := NewDLQConsumer(mockKafka, mockIngestor)

    testCases := []struct {
        name        string
        failedAlert FailedAlert
        mockReturn  error
        expectError bool
    }{
        {
            name: "successful retry",
            failedAlert: FailedAlert{
                Alert:      map[string]interface{}{"test": "data"},
                Tenant:     "test-tenant",
                RetryCount: 1,
            },
            mockReturn:  nil,
            expectError: false,
        },
        {
            name: "failed retry",
            failedAlert: FailedAlert{
                Alert:      map[string]interface{}{"test": "data"},
                Tenant:     "test-tenant",
                RetryCount: 1,
            },
            mockReturn:  errors.New("insertion failed"),
            expectError: true,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            mockIngestor.On("InsertAlertBulk", tc.failedAlert.Tenant).Return(1, tc.mockReturn).Once()

            err := consumer.retryAlert(tc.failedAlert)

            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
            mockIngestor.AssertExpectations(t)
        })
    }
}

func TestDLQConsumer_Start(t *testing.T) {
    mockKafka := new(MockKafkaRepo)
    mockIngestor := new(MockIngestor)
    consumer := NewDLQConsumer(mockKafka, mockIngestor)

    // Test context cancellation
    t.Run("context cancellation", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        mockKafka.On("SubscribeTopics", mock.Anything, mock.Anything).Return(nil)

        go func() {
            time.Sleep(100 * time.Millisecond)
            cancel()
        }()

        consumer.Start(ctx)
        mockKafka.AssertExpectations(t)
    })

    // Test message processing
    t.Run("process message", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        failedAlert := FailedAlert{
            Alert:      map[string]interface{}{"test": "data"},
            Tenant:     "test-tenant",
            RetryCount: 0,
            FailedAt:   time.Now().Add(-10 * time.Minute),
        }

        messageBytes, _ := json.Marshal(failedAlert)
        mockMessage := &kafka.Message{
            Value: messageBytes,
        }

        mockKafka.On("SubscribeTopics", mock.Anything, mock.Anything).Return(nil)
        mockKafka.On("ReadMessage", mock.Anything).Return(mockMessage, nil)
        mockIngestor.On("InsertAlertBulk", failedAlert.Tenant).Return(1, nil)

        go func() {
            time.Sleep(100 * time.Millisecond)
            cancel()
        }()

        consumer.Start(ctx)

        mockKafka.AssertExpectations(t)
        mockIngestor.AssertExpectations(t)
    })
}
