type FailedAlert struct {
    Alert       interface{}   `json:"alert"`
    Tenant      string       `json:"tenant"`
    Error       string       `json:"error"`
    RetryCount  int         `json:"retry_count"`
    FailedAt    time.Time   `json:"failed_at"`
    JobID       string      `json:"job_id"`
    DatafeedID  string      `json:"datafeed_id"`
}

func (h *MonitorJobHandlers) sendToDLQ(alerts []interface{}, tenant string, err error, jobInfo helpers.Job) error {
    dlqTopic := viper.GetString("kafka.topic.alert_dlq")
    
    failedAlert := FailedAlert{
        Alert:      alerts,
        Tenant:     tenant,
        Error:      err.Error(),
        RetryCount: 0,
        FailedAt:   time.Now(),
        JobID:      jobInfo.JobID,
        DatafeedID: jobInfo.DatafeedID,
    }

    message, err := json.Marshal(failedAlert)
    if err != nil {
        return fmt.Errorf("failed to marshal failed alert: %w", err)
    }

    if err := h.kafkaRepo.SendKafkaMessage(message, dlqTopic); err != nil {
        return fmt.Errorf("failed to send to DLQ: %w", err)
    }

    // Update job status to reflect DLQ routing
    errU := h.updateJob(
        bson.M{"_id": jobInfo.ID},
        bson.M{
            "status":         "DLQ_ROUTED",
            "status_message": fmt.Sprintf("Alerts routed to DLQ due to: %s", err.Error()),
            "dlq_info": bson.M{
                "routed_at": time.Now(),
                "error":     err.Error(),
            },
        },
    )
    if errU != nil {
        logz.Error("Failed to update job status after DLQ routing:", errU)
    }

    return nil
}

func (h *MonitorJobHandlers) handleFailedInsertion(bulk []interface{}, tenant string, jobInfo helpers.Job, err error) error {
    // Log the failure
    logz.Error(fmt.Sprintf("Failed to insert alerts for tenant %s: %v", tenant, err))

    // Send to DLQ
    if err := h.sendToDLQ(bulk, tenant, err, jobInfo); err != nil {
        logz.Error("Failed to send to DLQ:", err)
        return err
    }

    return nil
}

// Modified insertion code with DLQ handling
insertedNumber, err := pusher.InsertAlertBulk(jobInfo.Tenant)
if err != nil {
    if err := h.handleFailedInsertion(bulk, jobInfo.Tenant, jobInfo, err); err != nil {
        logz.Error("Failed to handle failed insertion:", err)
    }
    // Continue with job status update as before
    errU := h.updateJob(
        bson.M{"_id": _id.(primitive.ObjectID)},
        bson.M{
            "status":         helpers.TERMINATED,
            "status_message": err.Error(),
            "completed_time": helpers.GetCurrentTime(),
            "completed":      time.Now(),
        },
    )
    if errU != nil {
        logz.Error(errU.Error())
    }
    return
}
