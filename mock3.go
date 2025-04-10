func TestMonitorFetchedAlerts(t *testing.T) {
    tests := []struct {
        name          string
        setupMocks    func(*MockKafkaRepository, *KafkaRepository) *gomonkey.Patches
        expectedCalls int
        shouldPanic   bool
    }{
        {
            name: "Successfully process messages",
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
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
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
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
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
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
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
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
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
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
            setupMocks: func(mk *MockKafkaRepository, kr *KafkaRepository) *gomonkey.Patches {
                mk.On("GetKafkaRepository").Return(kr)

                kr.IsInitialized = true
                patches := gomonkey.ApplyMethod(reflect.TypeOf(kr), "SubscribeTopics",
                    func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
                        return nil
                    })

                patches.ApplyMethod(reflect.TypeOf(kr), "ReadMessageBatch",
                    func(_ *KafkaRepository, _ time.Duration, _ int) ([]*kafka.Message, error) {
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
            
            // Setup mocks and get patches
            patches := tt.setupMocks(mockKafka, kafkaRepo)
            defer patches.Reset()

            // Create monitor
            monitor := NewAlertMonitor(mockKafka)

            // Setup viper config
            viper.Set("kafka.topic.job_state", "test-topic")
            viper.Set("kafka.batch_size", 10)
            viper.Set("kafka.topic.dlq", "dlq-topic")

            // Create a channel to stop the monitor after a short time
            stopChan := make(chan struct{})
            go func() {
                time.Sleep(100 * time.Millisecond)
                close(stopChan)
            }()
            monitor.stopChan = stopChan

            // Run monitor
            monitor.MonitorFetchedAlerts()

            // Verify expectations
            mockKafka.AssertExpectations(t)
        })
    }
}

func TestAlertMonitor_Stop(t *testing.T) {
    mockKafka := new(MockKafkaRepository)
    kafkaRepo := &KafkaRepository{
        IsInitialized: true,
    }
    
    mockKafka.On("GetKafkaRepository").Return(kafkaRepo)
    
    patches := gomonkey.ApplyMethod(reflect.TypeOf(kafkaRepo), "SubscribeTopics",
        func(_ *KafkaRepository, _ []string, _ kafka.RebalanceCb) error {
            return nil
        })
    defer patches.Reset()
    
    patches.ApplyMethod(reflect.TypeOf(kafkaRepo), "Close",
        func(_ *KafkaRepository) {})

    monitor := NewAlertMonitor(mockKafka)
    monitor.Stop()
    
    mockKafka.AssertExpectations(t)
}
