func TestMonitorFetchedAlerts(t *testing.T) {
    tests := []struct {
        name          string
        setupMocks    func(*MockKafkaRepository, *KafkaRepository, *AlertMonitor, chan struct{}) *gomonkey.Patches
        expectedCalls int
        shouldPanic   bool
    }{
        {
            name: "Successfully process messages",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, am *AlertMonitor, stopChan chan struct{}) *gomonkey.Patches {
                messages := []*kafka.Message{
                    {
                        Value: []byte(`{"id": "1", "title": "Alert 1"}`),
                    },
                    {
                        Value: []byte(`{"id": "2", "title": "Alert 2"}`),
                    },
                }

                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.NewPatches()

                patches.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                var messageCallCount int
                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        messageCallCount++
                        if messageCallCount == 1 {
                            return messages, nil
                        }
                        // Close channel on second call to simulate completion
                        close(stopChan)
                        return nil, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(am), "forwardAlertsToSoarAPI",
                    func(_ *AlertMonitor, _ []helpers.AlertWithEvents) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(am), "commitProcessedMessages",
                    func(_ *AlertMonitor, _ []*kafka.Message, _ KafkaRepositoryInterface) {
                        // Successfully committed messages
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
        {
            name: "SOAR API failure with DLQ",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, am *AlertMonitor, stopChan chan struct{}) *gomonkey.Patches {
                messages := []*kafka.Message{
                    {
                        Value: []byte(`{"id": "1", "title": "Alert 1"}`),
                    },
                }

                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.NewPatches()

                patches.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                var messageCallCount int
                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        messageCallCount++
                        if messageCallCount == 1 {
                            return messages, nil
                        }
                        close(stopChan)
                        return nil, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(am), "forwardAlertsToSoarAPI",
                    func(_ *AlertMonitor, _ []helpers.AlertWithEvents) error {
                        return fmt.Errorf("SOAR API error")
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "SendKafkaMessage",
                    func(_ *KafkaRepository, _ []byte, _ string) {})

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
        {
            name: "Commit messages failure",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, am *AlertMonitor, stopChan chan struct{}) *gomonkey.Patches {
                messages := []*kafka.Message{
                    {
                        Value: []byte(`{"id": "1", "title": "Alert 1"}`),
                    },
                }

                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.NewPatches()

                patches.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                var messageCallCount int
                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        messageCallCount++
                        if messageCallCount == 1 {
                            return messages, nil
                        }
                        close(stopChan)
                        return nil, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(am), "forwardAlertsToSoarAPI",
                    func(_ *AlertMonitor, _ []helpers.AlertWithEvents) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(am), "commitProcessedMessages",
                    func(_ *AlertMonitor, _ []*kafka.Message, _ KafkaRepositoryInterface) {
                        logz.Error("Failed to commit messages")
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Initialize test environment
            mockKafka := new(MockKafkaRepository)
            kafkaRepo := &KafkaRepository{
                IsInitialized: true,
            }
            
            // Create stop channel
            stopChan := make(chan struct{})
            
            // Create monitor
            monitor := NewAlertMonitor(mockKafka)
            monitor.stopChan = stopChan

            // Setup mocks and get patches
            patches := tt.setupMocks(mockKafka, kafkaRepo, monitor, stopChan)
            defer patches.Reset()

            // Setup viper config
            viper.Set("kafka.topic.job_state", "test-topic")
            viper.Set("kafka.batch_size", 10)
            viper.Set("kafka.topic.dlq", "dlq-topic")

            // Run monitor
            monitor.MonitorFetchedAlerts()

            // Verify expectations
            mockKafka.AssertExpectations(t)
        })
    }
}
