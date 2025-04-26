package job_poller

import (
    "encoding/json"
    "fmt"
    "github.com/pkg/errors"
    "go.uber.org/zap"
    "strconv"
    "time"
)

type JobProcessor struct {
    agentService     agent.IAgentService
    zbClient         zeebe.Client
    taskInfoService  TaskInfoService
    syncMap          *singleton.SyncMap
    logger          *zap.Logger
}

func NewJobProcessor(
    agentService agent.IAgentService,
    zbClient zeebe.Client,
    taskInfoService TaskInfoService,
    syncMap *singleton.SyncMap,
    logger *zap.Logger,
) *JobProcessor {
    return &JobProcessor{
        agentService:    agentService,
        zbClient:        zbClient,
        taskInfoService: taskInfoService,
        syncMap:         syncMap,
        logger:         logger,
    }
}

func (p *JobProcessor) ProcessJob(job entities.Job) error {
    jobLogger := p.logger.With(zap.String("job_id", strconv.FormatInt(job.Key, 10)))
    
    // Extract job metadata
    metadata, err := p.extractJobMetadata(job)
    if err != nil {
        return p.handleError(job.Key, err, jobLogger)
    }

    // Get task information
    taskInfo, err := p.taskInfoService.GetTaskInfo(job, metadata)
    if err != nil {
        return p.handleError(job.Key, err, jobLogger)
    }

    // Create task
    task := p.createTask(metadata, taskInfo)

    // Process based on agent presence
    if metadata.AgentID != Empty {
        return p.processAgentTask(task, metadata, taskInfo, jobLogger)
    }

    return p.processRegularTask(task, metadata.RequestID, jobLogger)
}

type JobMetadata struct {
    RequestID   string
    TaskID      string
    TaskVersion string
    InsRef      string
    AgentID     string
    Tenant      string
}

func (p *JobProcessor) extractJobMetadata(job entities.Job) (*JobMetadata, error) {
    var variables map[string]string
    if err := json.Unmarshal([]byte(job.Variables), &variables); err != nil {
        return nil, fmt.Errorf("failed to unmarshal variables: %v", err)
    }

    taskVersion, insRef, agentId, tenant, err := p.taskInfoService.GetServiceTaskVersion(&job)
    if err != nil {
        return nil, fmt.Errorf("failed to get service task version: %v", err)
    }

    return &JobMetadata{
        RequestID:   variables["oren_request_id"],
        TaskID:      strconv.FormatInt(job.Key, 10),
        TaskVersion: taskVersion,
        InsRef:      insRef,
        AgentID:     agentId,
        Tenant:      tenant,
    }, nil
}

func (p *JobProcessor) createTask(metadata *JobMetadata, taskInfo *TaskInfo) singleton.Task {
    return singleton.Task{
        Type:             "playbook",
        TaskID:           metadata.TaskID,
        Script:           taskInfo.Script,
        Docker:           taskInfo.Docker,
        Args:             taskInfo.Args,
        Command:          taskInfo.Cmd,
        Params:           taskInfo.Params,
        IntegrationID:    taskInfo.IntegrationId,
        WorkflowInstance: taskInfo.WorkflowInstance,
        Tenant:           metadata.Tenant,
        Agent:            metadata.AgentID,
        RequestID:        metadata.RequestID,
    }
}

func (p *JobProcessor) processAgentTask(task singleton.Task, metadata *JobMetadata, taskInfo *TaskInfo, logger *zap.Logger) error {
    // Verify agent status
    if err := p.verifyAgentStatus(metadata.AgentID, metadata.Tenant); err != nil {
        return err
    }

    // Create and store agent job
    agentJob := p.createAgentJob(task, metadata, taskInfo)
    if err := p.agentService.Create(agentJob); err != nil {
        logger.Error("Failed to create agent job", zap.Error(err))
        return err
    }

    p.syncMap.Processing.Store(task.TaskID, task)
    return nil
}

func (p *JobProcessor) verifyAgentStatus(agentID string, tenant string) error {
    agentInfo, err := p.agentService.GetAgent(agentID, tenant2.NewTenantFromString(tenant))
    if (err != nil && errors.Is(err, &agent.AgentNotFound{})) || (agentInfo != nil && !agentInfo.Active) {
        return errors.New("agent is inactive or deleted")
    }
    return nil
}

func (p *JobProcessor) createAgentJob(task singleton.Task, metadata *JobMetadata, taskInfo *TaskInfo) models.AgentJob {
    agentTask := task
    agentTask.Params = ""
    agentTask.Args = ""
    
    return models.AgentJob{
        CreatedTime:  time.Now(),
        Tenant:       metadata.Tenant,
        Job:          agentTask,
        Agent:        metadata.AgentID,
        Instance:     taskInfo.AgentInstance,
        Integration:  taskInfo.AgentIntegration,
        AgentArgs:    taskInfo.AgentArgs,
    }
}

func (p *JobProcessor) processRegularTask(task singleton.Task, requestID string, logger *zap.Logger) error {
    p.syncMap.Tasks <- task
    logger.Info("Task has been sent", 
        zap.String("RequestID", requestID),
        zap.String("TaskID", task.TaskID))
    return nil
}

func (p *JobProcessor) handleError(jobKey int64, err error, logger *zap.Logger) error {
    logger.Error("Job processing failed", zap.Error(err))
    return p.zbClient.SendJobsIncident(jobKey, err.Error())
}
