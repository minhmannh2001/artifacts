// AlertIngestionResult represents the result of ingesting a single alert
type AlertIngestionResult struct {
    Alert interface{}
    Error error
}

// BulkIngestionResult represents the result of a bulk ingestion operation
type BulkIngestionResult struct {
    SuccessCount int
    FailedAlerts []AlertIngestionResult
}

func (ingestor *Ingestor) InsertAlertBulk(tenant string) (*BulkIngestionResult, error) {
    result := &BulkIngestionResult{
        SuccessCount: 0,
        FailedAlerts: make([]AlertIngestionResult, 0),
    }

    for _, alert := range ingestor.Bulk {
        success := false
        var lastError error

        for i := 0; i < ingestor.Retries; i++ {
            alertJson, err := json.Marshal(alert)
            if err != nil {
                lastError = fmt.Errorf("failed to marshal alert: %w", err)
                continue
            }

            payLoad := bytes.NewReader(alertJson)
            req, err := http.NewRequest(
                "POST",
                fmt.Sprintf("%s/v1/%s/alert/entity_extraction", ingestor.BaseUrl, tenant),
                payLoad,
            )
            if err != nil {
                lastError = fmt.Errorf("failed to create request: %w", err)
                continue
            }

            req.Close = true
            req.Header.Set("X-API-KEY", ingestor.ApiKey)
            req.Header.Set("Tenant", tenant)
            req.Header.Set("Content-Type", "application/json")

            client := &http.Client{}
            resp, reqErr := client.Do(req)
            if reqErr != nil {
                lastError = fmt.Errorf("request failed: %w", reqErr)
                logz.Error(reqErr.Error())
                continue
            }

            res, _ := ioutil.ReadAll(resp.Body)
            _ = resp.Body.Close()

            apiRes := ApiResponse{}
            if err := json.Unmarshal(res, &apiRes); err != nil {
                lastError = fmt.Errorf("failed to unmarshal response: %w", err)
                continue
            }

            logz.Info(
                fmt.Sprintf(
                    "Ingest alert response: status=%v, code=%v",
                    resp.Status,
                    resp.StatusCode,
                ),
            )

            switch {
            case resp.StatusCode == 400 && apiRes.ErrorCode == 4001001:
                // Permanent failure, no need to retry
                lastError = fmt.Errorf("permanent failure: %s", apiRes.Detail)
                i = ingestor.Retries // Break out of retry loop
            case resp.StatusCode == 500:
                if i == (ingestor.Retries - 1) {
                    lastError = fmt.Errorf("server error after %d retries: %s", i+1, apiRes.Detail)
                } else {
                    time.Sleep(time.Duration(ingestor.RetryInterval) * time.Second)
                    continue
                }
            case resp.StatusCode == 200 || resp.StatusCode == 201:
                success = true
                result.SuccessCount++
                i = ingestor.Retries // Break out of retry loop
            default:
                lastError = fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(res))
                continue
            }
        }

        if !success {
            result.FailedAlerts = append(result.FailedAlerts, AlertIngestionResult{
                Alert: alert,
                Error: lastError,
            })
        }
    }

    // Return error only if no alerts were successfully processed
    if result.SuccessCount == 0 && len(result.FailedAlerts) > 0 {
        return result, fmt.Errorf("all alerts failed to process")
    }

    return result, nil
}

Key improvements:
Returns detailed results for both successful and failed alerts
Continues processing even if some alerts fail
Better error handling and reporting
More informative logging
Cleaner retry logic
Usage example:

result, err := pusher.InsertAlertBulk(jobInfo.Tenant)
if err != nil {
    logz.Error(err.Error())
} else {
    logz.Info(fmt.Sprintf("%d alerts ingested successfully", result.SuccessCount))
    
    // Handle failed alerts
    if len(result.FailedAlerts) > 0 {
        failedAlerts := make([]interface{}, len(result.FailedAlerts))
        for i, fa := range result.FailedAlerts {
            failedAlerts[i] = fa.Alert
        }
        
        // Send failed alerts to DLQ
        if err := h.sendToDLQ(failedAlerts, jobInfo.Tenant, nil, jobInfo); err != nil {
            logz.Error(fmt.Sprintf("Failed to send %d alerts to DLQ: %v", len(failedAlerts), err))
        }
    }
}

Benefits:
Partial success is now possible - successful alerts are processed even if others fail
Better visibility into what failed and why
More granular error handling
Easier integration with DLQ mechanism
More maintainable code structure
