package job_poller

import (
    "fmt"
    "strings"
)

type TaskInfoService struct {
    pbInstanceService pbInstanceService.IPlaybookInstanceService
    scriptService     script.IScriptService
    integrationService integration.IIntegrationService
}

func NewTaskInfoService(
    pbInstanceService pbInstanceService.IPlaybookInstanceService,
    scriptService script.IScriptService,
    integrationService integration.IIntegrationService,
) *TaskInfoService {
    return &TaskInfoService{
        pbInstanceService:   pbInstanceService,
        scriptService:      scriptService,
        integrationService: integrationService,
    }
}

func (s *TaskInfoService) GetTaskInfo(job entities.Job, metadata *JobMetadata) (*TaskInfo, error) {
    taskInfo, err := s.makeTaskInfo(metadata.TaskVersion, metadata.Tenant)
    if err != nil {
        return nil, fmt.Errorf("failed to get task info: %v", err)
    }

    taskInfo.Instance = metadata.InsRef
    if err = s.enrichTaskInfo(job, taskInfo); err != nil {
        return nil, fmt.Errorf("failed to enrich task info: %v", err)
    }

    return taskInfo, nil
}

func (s *TaskInfoService) GetServiceTaskVersion(job *entities.Job) (string, string, string, string, error) {
    variables, err := job.GetVariablesAsMap()
    if err != nil {
        return "", "", "", "", err
    }

    customKey, ok := variables["cycir_workflow_instance_key"]
    if !ok {
        customKey = job.ProcessInstanceKey
    }

    vm, iv, agentMapping, tenant, err := s.pbInstanceService.GetServiceTaskVersionByCustomKey(customKey)
    if err != nil {
        return "", "", "", "", err
    }

    return vm[job.ElementId], iv[job.ElementId], agentMapping[job.ElementId], tenant, nil
}

func (s *TaskInfoService) makeTaskInfo(version, tenant string) (*TaskInfo, error) {
    if strings.Contains(version, ".") {
        return s.integrationService.GetIntegrationInfo(version)
    }
    return s.scriptService.GetScriptTaskInfo(version, tenant)
}

func (s *TaskInfoService) enrichTaskInfo(job entities.Job, taskInfo *TaskInfo) error {
    // Implementation of enrichTaskInfo moved from JobPoller
    // This would contain the logic to enrich the task info with additional data
    return nil
}
