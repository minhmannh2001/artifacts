func TestMonitorJobHandlers_SendToDLQ(t *testing.T) {
    mockKafkaRepo := new(MockKafkaRepo)
    h := &MonitorJobHandlers{
        kafkaRepo: mockKafkaRepo,
    }

    testCases := []struct {
        name        string
        alerts      []interface{}
        tenant      string
        inputError  error
        jobInfo     helpers.Job
        expectError bool
    }{
        {
            name: "successful DLQ send",
            alerts: []interface{}{
                map[string]interface{}{"alert": "test"},
            },
            tenant:     "test-tenant",
            inputError: errors.New("insertion failed"),
            jobInfo: helpers.Job{
                JobID:      "test-job",
                DatafeedID: "test-datafeed",
            },
            expectError: false,
        },
        {
            name: "kafka send failure",
            alerts: []interface{}{
                map[string]interface{}{"alert": "test"},
            },
            tenant:     "test-tenant",
            inputError: errors.New("insertion failed"),
            jobInfo: helpers.Job{
                JobID:      "test-job",
                DatafeedID: "test-datafeed",
            },
            expectError: true,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            if tc.expectError {
                mockKafkaRepo.On("SendKafkaMessage", mock.Anything, mock.Anything).
                    Return(errors.New("kafka error")).Once()
            } else {
                mockKafkaRepo.On("SendKafkaMessage", mock.Anything, mock.Anything).
                    Return(nil).Once()
            }

            err := h.sendToDLQ(tc.alerts, tc.tenant, tc.inputError, tc.jobInfo)

            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
            mockKafkaRepo.AssertExpectations(t)
        })
    }
}

func TestMonitorJobHandlers_HandleFailedInsertion(t *testing.T) {
    mockKafkaRepo := new(MockKafkaRepo)
    h := &MonitorJobHandlers{
        kafkaRepo: mockKafkaRepo,
    }

    testCases := []struct {
        name        string
        bulk        []interface{}
        tenant      string
        jobInfo     helpers.Job
        inputError  error
        expectError bool
    }{
        {
            name: "successful handling",
            bulk: []interface{}{
                map[string]interface{}{"data": "test"},
            },
            tenant: "test-tenant",
            jobInfo: helpers.Job{
                JobID:      "test-job",
                DatafeedID: "test-datafeed",
            },
            inputError:  errors.New("insertion failed"),
            expectError: false,
        },
        {
            name: "DLQ send failure",
            bulk: []interface{}{
                map[string]interface{}{"data": "test"},
            },
            tenant: "test-tenant",
            jobInfo: helpers.Job{
                JobID:      "test-job",
                DatafeedID: "test-datafeed",
            },
            inputError:  errors.New("insertion failed"),
            expectError: true,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            if tc.expectError {
                mockKafkaRepo.On("SendKafkaMessage", mock.Anything, mock.Anything).
                    Return(errors.New("kafka error")).Once()
            } else {
                mockKafkaRepo.On("SendKafkaMessage", mock.Anything, mock.Anything).
                    Return(nil).Once()
            }

            err := h.handleFailedInsertion(tc.bulk, tc.tenant, tc.jobInfo, tc.inputError)

            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
            mockKafkaRepo.AssertExpectations(t)
        })
    }
}

func TestMonitorJobHandlers_Integration(t *testing.T) {
    mockKafkaRepo := new(MockKafkaRepo)
    mockIngestor := new(MockIngestor)
    h := &MonitorJobHandlers{
        kafkaRepo: mockKafkaRepo,
    }

    t.Run("full insertion failure flow", func(t *testing.T) {
        // Setup test data
        bulk := []interface{}{
            map[string]interface{}{"data": "test"},
        }
        jobInfo := helpers.Job{
            JobID:      "test-job",
            DatafeedID: "test-datafeed",
            Tenant:     "test-tenant",
        }

        // Mock the insertion failure
        mockIngestor.On("InsertAlertBulk", jobInfo.Tenant).
            Return(0, errors.New("insertion failed")).Once()

        // Mock successful DLQ send
        mockKafkaRepo.On("SendKafkaMessage", mock.Anything, mock.Anything).
            Return(nil).Once()

        // Execute the flow
        insertedNumber, err := mockIngestor.InsertAlertBulk(jobInfo.Tenant)
        assert.Error(t, err)
        assert.Equal(t, 0, insertedNumber)

        err = h.handleFailedInsertion(bulk, jobInfo.Tenant, jobInfo, err)
        assert.NoError(t, err)

        mockIngestor.AssertExpectations(t)
        mockKafkaRepo.AssertExpectations(t)
    })
}
