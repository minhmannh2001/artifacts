package container

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/imdario/mergo"
	"go.uber.org/zap"

	"your-project/logger"
	"your-project/output"
)

type OutputContainer struct {
	Type        string                 `json:"type"`
	ResultsType string                 `json:"results_type,omitempty"`
	Results     map[string]interface{} `json:"results,omitempty"`
	Message     string                 `json:"message,omitempty"`
	ErrMessage  string                 `json:"err_message,omitempty"`
}

func (c *Container) Run(name, context string, args map[string]interface{}, requestID, taskID string) (output.Output, error) {
	taskLog := logger.With(zap.String("RequestID", requestID), zap.String("task-id", taskID))
	taskLog.Info("Run container", zap.Any("container", c))

	if err := c.prepareContainer(context); err != nil {
		return output.Output{}, err
	}

	jobInfo := c.parseJobInfo(context)
	defaultResult := c.initializeDefaultResult()

	outputResult, err := c.processContainerOutput(taskLog, jobInfo, defaultResult)
	if err != nil {
		return output.Output{}, err
	}

	return c.createRunningResult(name, taskID, requestID, outputResult, args)
}

func (c *Container) prepareContainer(context string) error {
	if c.Status == 0 {
		return nil
	}

	if c.Cmd == nil || c.Stdin == nil || c.Stdout == nil {
		if err := c.StartContainer(); err != nil {
			return fmt.Errorf("error starting container: %w", err)
		}
	}

	if _, err := c.Stdin.Write([]byte(context + "\n")); err != nil {
		_ = c.StopContainer()
		return fmt.Errorf("error writing to container stdin: %w", err)
	}

	return nil
}

func (c *Container) parseJobInfo(context string) map[string]interface{} {
	var jobInfo map[string]interface{}
	_ = json.Unmarshal([]byte(context), &jobInfo)
	return jobInfo
}

func (c *Container) initializeDefaultResult() map[string]interface{} {
	return map[string]interface{}{
		"Type":           -1,
		"Contents":       make(map[string]interface{}),
		"ContentsFormat": "unknown",
	}
}

func (c *Container) processContainerOutput(taskLog *zap.Logger, jobInfo, defaultResult map[string]interface{}) (interface{}, error) {
	var outputResult interface{}

	for c.Stdout.Scan() {
		out := c.Stdout.Text()
		taskLog.Info("Task output", zap.String("task", jobInfo["job_id"].(string)), zap.String("result", out))

		var outputContainer OutputContainer
		if err := json.Unmarshal([]byte(out), &outputContainer); err != nil {
			taskLog.Error("Cannot parse output", zap.String("output", out), zap.Error(err))
			continue
		}

		outputResult = c.handleOutputType(outputContainer, defaultResult, jobInfo, taskLog)
		if outputContainer.Type == "completed" {
			break
		}
	}

	return outputResult, nil
}

func (c *Container) handleOutputType(outputContainer OutputContainer, defaultResult, jobInfo map[string]interface{}, taskLog *zap.Logger) interface{} {
	switch outputContainer.Type {
	case "result":
		return c.handleResultOutput(outputContainer, defaultResult)
	case "log":
		return c.handleLogOutput(outputContainer, jobInfo, taskLog)
	case "exception", "error":
		return c.handleErrorOutput(outputContainer)
	case "ignored_exception":
		return c.handleIgnoredExceptionOutput(outputContainer, taskLog)
	case "pending":
		return c.handlePendingOutput(outputContainer, taskLog)
	default:
		return map[string]interface{}{
			"Type":           -1,
			"Contents":       outputContainer.Results,
			"ContentsFormat": "unknown",
		}
	}
}

func (c *Container) handleResultOutput(outputContainer OutputContainer, defaultResult map[string]interface{}) map[string]interface{} {
	defaultResult["Type"] = 1
	defaultResult["ContentsFormat"] = outputContainer.ResultsType

	if fetchedData, ok := outputContainer.Results["fetched_data"]; ok {
		c.mergeFetchedData(defaultResult["Contents"].(map[string]interface{}), fetchedData)
	} else {
		defaultResult["Contents"] = outputContainer.Results
	}

	return defaultResult
}

func (c *Container) mergeFetchedData(content map[string]interface{}, newFetchedData interface{}) {
	if existingFetchedData, ok := content["fetched_data"]; ok {
		_ = mergo.Merge(&existingFetchedData, newFetchedData)
		if sliceData, ok := existingFetchedData.([]interface{}); ok {
			if newSliceData, ok := newFetchedData.([]interface{}); ok {
				content["fetched_data"] = append(sliceData, newSliceData...)
			}
		}
	} else {
		content["fetched_data"] = newFetchedData
	}
}

func (c *Container) handleLogOutput(outputContainer OutputContainer, jobInfo map[string]interface{}, taskLog *zap.Logger) map[string]interface{} {
	log := map[string]interface{}{
		"Type":     4,
		"Contents": outputContainer.Message,
		"JobID":    jobInfo["job_id"],
		"Tenant":   jobInfo["tenant"],
	}
	c.LogChan <- log
	taskLog.Info("Container log", zap.String("message", outputContainer.Message), zap.Any("chan", c.LogChan))
	return nil
}

func (c *Container) handleErrorOutput(outputContainer OutputContainer) map[string]interface{} {
	return map[string]interface{}{
		"Type":     2,
		"Contents": "Task failed: " + outputContainer.ErrMessage,
	}
}

func (c *Container) handleIgnoredExceptionOutput(outputContainer OutputContainer, taskLog *zap.Logger) map[string]interface{} {
	taskLog.Error("Ignored exception", zap.Any("Error", outputContainer.Message))
	return map[string]interface{}{
		"Type":     1,
		"Contents": outputContainer.Results,
	}
}

func (c *Container) handlePendingOutput(outputContainer OutputContainer, taskLog *zap.Logger) map[string]interface{} {
	taskLog.Error("Pending", zap.Any("Pending", outputContainer.Results))
	return map[string]interface{}{
		"Type":     3,
		"Contents": outputContainer.Results,
	}
}

func (c *Container) createRunningResult(name, taskID, requestID string, outputResult interface{}, args map[string]interface{}) (output.Output, error) {
	if resultLog, ok := outputResult.(map[string]interface{}); ok {
		c.LogChan <- resultLog
	}

	payload, err := json.Marshal(outputResult)
	if err != nil {
		return CreateContainerErrorOutput(name, args, requestID, taskID, errors.New("invalid container result")), nil
	}

	c.Status = 1
	c.ChangeTime = time.Now().Unix()

	return output.Output{
		Name:      name,
		TaskId:    taskID,
		RequestID: requestID,
		Payload:   string(payload),
	}, nil
}
