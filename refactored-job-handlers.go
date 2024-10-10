package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"your-project/helpers"
	"your-project/kafka"
	"your-project/output"
)

type JobHandlers struct {
	kafkaRepo     *kafka.KafkaRepo
	containerRepo ContainerRepository
}

func (h *JobHandlers) RunDatafeed(name, context string, args map[string]interface{}, requestID, taskID string) string {
	jobInfo, err := h.parseJobInfo(context)
	if err != nil {
		return ""
	}

	h.updateJobStatus(&jobInfo, helpers.COMPLETING)
	if err := h.sendKafkaMessage(jobInfo); err != nil {
		return ""
	}

	output := h.runContainerTask(name, context, args, requestID, taskID)
	h.processJobOutput(&jobInfo, output)

	return h.sendResults(jobInfo, output)
}

func (h *JobHandlers) parseJobInfo(context string) (helpers.Job, error) {
	var jobInfo helpers.Job
	err := json.Unmarshal([]byte(context), &jobInfo)
	if err != nil {
		return helpers.Job{}, err
	}
	tenants <- jobInfo.Tenant
	return jobInfo, nil
}

func (h *JobHandlers) updateJobStatus(jobInfo *helpers.Job, status string) {
	jobInfo.Output.UpdateStatusOnly = true
	jobInfo.Status = status
	jobInfo.StatusMessage = status
	jobInfo.Script = ""
	jobInfo.Consumed = true
}

func (h *JobHandlers) sendKafkaMessage(jobInfo helpers.Job) error {
	kafkaMessage := helpers.KafkaMessage{
		Type:       jobInfo.Status,
		TargetType: "job",
		TargetID:   jobInfo.JobID,
		Data:       jobInfo,
	}

	message, _ := json.Marshal(kafkaMessage)
	agentMode := viper.GetString("agent.mode")
	resultTopic := viper.GetString("kafka.topic.job_state")
	return HandleMessageByAgent(agentMode, message, resultTopic, h.kafkaRepo.GetKafkaRepo())
}

func (h *JobHandlers) runContainerTask(name, context string, args map[string]interface{}, requestID, taskID string) output.Output {
	for {
		idx := h.containerRepo.FindFreeIndex(viper.GetString("worker.python_base_image"), jobInfo.Tenant)
		if idx != -1 {
			container := h.containerRepo.GetContainerByIndex(idx)
			logz.Info("Start run container", zap.String("container", container.Name))
			output, err := container.Run(name, context, args, requestID, taskID)
			if err != nil {
				logz.Error("Run task failed", zap.Error(err), zap.String("container", container.Name))
			}
			return output
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (h *JobHandlers) processJobOutput(jobInfo *helpers.Job, output output.Output) {
	parseJobOutputByType(output, jobInfo)
	data, _ := json.Marshal(jobInfo.Output.Contents)
	var fetchedData helpers.Content
	_ = json.Unmarshal(data, &fetchedData)
	jobInfo.ExtraInfo = fetchedData.ExtraInfo
}

func (h *JobHandlers) sendResults(jobInfo helpers.Job, output output.Output) string {
	agentMode := viper.GetString("agent.mode")
	resultTopic := viper.GetString("kafka.topic.job_state")
	kafkaRepo := h.kafkaRepo.GetKafkaRepo()

	for idx, alert := range jobInfo.Output.Contents.FetchedData {
		lastMessage := idx == len(jobInfo.Output.Contents.FetchedData)-1
		h.sendAlert(jobInfo, alert, idx, lastMessage, agentMode, resultTopic, kafkaRepo)
		if lastMessage {
			res, _ := json.Marshal(output)
			return string(res)
		}
	}

	h.finalizeJob(&jobInfo)
	return h.sendFinalMessage(jobInfo, agentMode, resultTopic, kafkaRepo)
}

func (h *JobHandlers) sendAlert(jobInfo helpers.Job, alert map[string]interface{}, idx int, lastMessage bool, agentMode, resultTopic string, kafkaRepo *kafka.KafkaRepo) {
	payload := helpers.Result{
		Contents: helpers.Content{
			FetchedData: []map[string]interface{}{alert},
			AlertOrder:  fmt.Sprintf("%d/%d", idx+1, len(jobInfo.Output.Contents.FetchedData)),
			Count:       int64(len(jobInfo.Output.Contents.FetchedData)),
		},
		LastMessage:      lastMessage,
		UpdateStatusOnly: false,
	}
	jobInfo.Output = payload
	kafkaMessage := helpers.KafkaMessage{
		Type:       jobInfo.Status,
		TargetType: "job",
		TargetID:   jobInfo.JobID,
		Data:       jobInfo,
	}
	outputStr, _ := json.Marshal(kafkaMessage)
	HandleMessageByAgent(agentMode, outputStr, resultTopic, kafkaRepo)
}

func (h *JobHandlers) finalizeJob(jobInfo *helpers.Job) {
	if jobInfo.Status == helpers.COMPLETING {
		jobInfo.Status = helpers.COMPLETED
		jobInfo.StatusMessage = helpers.COMPLETED
		jobInfo.CompletedTime = helpers.GetCurrentTime()
		jobInfo.Completed = time.Now()
	}
}

func (h *JobHandlers) sendFinalMessage(jobInfo helpers.Job, agentMode, resultTopic string, kafkaRepo *kafka.KafkaRepo) string {
	kafkaMessage := helpers.KafkaMessage{
		Type:       jobInfo.Status,
		TargetType: "job",
		TargetID:   jobInfo.JobID,
		Data:       jobInfo,
	}
	outputStr, _ := json.Marshal(kafkaMessage)
	HandleMessageByAgent(agentMode, outputStr, resultTopic, kafkaRepo)
	return string(outputStr)
}

func HandleMessageByAgent(agentMode string, outputStr []byte, resultTopic string, kafkaRepo *kafka.KafkaRepo) error {
	if agentMode == Agent {
		return helpers.UpdateAgentJobResults(outputStr)
	}
	kafkaRepo.SendKafkaMessage(outputStr, resultTopic)
	return nil
}
