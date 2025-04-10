func TestMonitorFetchedAlerts(t *testing.T) {
    tests := []struct {
        name          string
        setupMocks    func(*MockKafkaRepository, *KafkaRepository, chan struct{}) *gomonkey.Patches
        expectedCalls int
        shouldPanic   bool
    }{
        {
            name: "Successfully process messages",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
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
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        // Close the channel after processing messages
                        defer close(stopChan)
                        return messages, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "CommitMessage",
                    func(_ *KafkaRepository, _ *kafka.Message) ([]kafka.TopicPartition, error) {
                        return []kafka.TopicPartition{}, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
        {
            name: "Subscription failure",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        defer close(stopChan)
                        return fmt.Errorf("subscription failed")
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 0,
        },
        {
            name: "Read message batch failure",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        defer close(stopChan)
                        return nil, fmt.Errorf("read failed")
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
        {
            name: "Empty message batch",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        defer close(stopChan)
                        return []*kafka.Message{}, nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            expectedCalls: 1,
        },
        {
            name: "SOAR API failure with DLQ",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
                messages := []*kafka.Message{
                    {
                        Value: []byte(`{"id": "1", "title": "Alert 1"}`),
                    },
                }

                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        defer close(stopChan)
                        return messages, nil
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
            name: "Panic recovery",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository, stopChan chan struct{}) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
                        defer close(stopChan)
                        panic("test panic")
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "Close",
                    func(_ *KafkaRepository) {})

                return patches
            },
            shouldPanic: true,
            expectedCalls: 0,
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
            
            // Setup mocks and get patches
            patches := tt.setupMocks(mockKafka, kafkaRepo, stopChan)
            defer patches.Reset()

            // Create monitor
            monitor := NewAlertMonitor(mockKafka)
            monitor.stopChan = stopChan

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
