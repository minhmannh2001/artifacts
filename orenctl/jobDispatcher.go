package job_poller

import (
    "context"
    "github.com/camunda-cloud/zeebe/clients/go/pkg/entities"
    "sync"
)

type JobDispatcher struct {
    jobQueue          chan entities.Job
    preparedJobQueue  chan *PreparedJob
    resultPublisher   *JobResultPublisher
    services          *Services
    closeSignal       chan struct{}
}

type PreparedJob struct {
    Job       entities.Job
    TaskInfo  *TaskInfo
    Variables map[string]interface{}
}

type Services struct {
    ScriptRepo       database.IScriptRepo
    IntegrationRepo  database.IIntegrationRepo
    ItgInstanceRepo  database.IItgInstanceRepo
    PbRepo          database.IPlaybookRepo
    PbInstanceService pbInstanceService.IPlaybookInstanceService
}

func NewJobDispatcher(services *Services, resultPublisher *JobResultPublisher) *JobDispatcher {
    return &JobDispatcher{
        jobQueue:         make(chan entities.Job, 1000),
        preparedJobQueue: make(chan *PreparedJob, 1000),
        resultPublisher:  resultPublisher,
        services:        services,
        closeSignal:      make(chan struct{}),
    }
}

func (d *JobDispatcher) Start(closeWait *sync.WaitGroup) {
    defer closeWait.Done()

    for {
        select {
        case job := <-d.jobQueue:
            go d.prepareAndDispatchJob(job)
        case <-d.closeSignal:
            return
        }
    }
}

func (d *JobDispatcher) prepareAndDispatchJob(job entities.Job) {
    taskVersion, insRef, agentId, tenant, err := d.getServiceTaskVersion(&job)
    if err != nil {
        d.resultPublisher.PublishFailure(job.Key, fmt.Errorf("failed to determine version of task %s: %v", job.ElementId, err))
        return
    }

    taskInfo, err := d.getTaskInfo(taskVersion, tenant)
    if err != nil {
        d.resultPublisher.PublishFailure(job.Key, err)
        return
    }

    preparedJob := &PreparedJob{
        Job:      job,
        TaskInfo: taskInfo,
    }

    d.preparedJobQueue <- preparedJob
}

func (d *JobDispatcher) GetPreparedJobQueue() chan *PreparedJob {
    return d.preparedJobQueue
}

// Implement other necessary methods from your existing code...
