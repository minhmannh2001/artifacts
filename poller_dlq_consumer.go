package dlq

import (
    "context"
    "encoding/json"
    "time"
)

type DLQConsumer struct {
    kafkaRepo    KafkaRepoI
    maxRetries   int
    retryBackoff time.Duration
    ingestor     *ingestor.Ingestor
}

func NewDLQConsumer(kafkaRepo KafkaRepoI, ingestor *ingestor.Ingestor) *DLQConsumer {
    return &DLQConsumer{
        kafkaRepo:    kafkaRepo,
        maxRetries:   3,
        retryBackoff: time.Minute * 5,
        ingestor:     ingestor,
    }
}

func (c *DLQConsumer) Start(ctx context.Context) {
    dlqTopic := viper.GetString("kafka.topic.alert_dlq")
    retryTopic := viper.GetString("kafka.topic.alert_retry")

    _ = c.kafkaRepo.SubscribeTopics([]string{dlqTopic}, nil)

    for {
        select {
        case <-ctx.Done():
            return
        default:
            msg, err := c.kafkaRepo.ReadMessage(-1)
            if err != nil {
                logz.Error("Error reading from DLQ:", err)
                continue
            }

            var failedAlert FailedAlert
            if err := json.Unmarshal(msg.Value, &failedAlert); err != nil {
                logz.Error("Failed to unmarshal DLQ message:", err)
                continue
            }

            // Check retry count
            if failedAlert.RetryCount >= c.maxRetries {
                logz.Error("Max retries exceeded for alert:", failedAlert.JobID)
                // Could implement permanent failure storage here
                continue
            }

            // Implement exponential backoff
            backoffDuration := c.retryBackoff * time.Duration(1 << failedAlert.RetryCount)
            if time.Since(failedAlert.FailedAt) < backoffDuration {
                // Not ready for retry yet, send back to DLQ
                failedAlert.RetryCount++
                message, _ := json.Marshal(failedAlert)
                c.kafkaRepo.SendKafkaMessage(message, dlqTopic)
                continue
            }

            // Attempt retry
            if err := c.retryAlert(failedAlert); err != nil {
                logz.Error("Retry failed:", err)
                // Increment retry count and send back to DLQ
                failedAlert.RetryCount++
                failedAlert.FailedAt = time.Now()
                message, _ := json.Marshal(failedAlert)
                c.kafkaRepo.SendKafkaMessage(message, dlqTopic)
            }
        }
    }
}

func (c *DLQConsumer) retryAlert(failedAlert FailedAlert) error {
    bulk := []interface{}{failedAlert.Alert}
    _, err := c.ingestor.InsertAlertBulk(failedAlert.Tenant)
    return err
}
