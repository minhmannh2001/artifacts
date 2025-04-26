package job_poller

import (
    "sync"
)

type JobResultCollector struct {
    resultChan      chan JobResult
    resultPublisher *JobResultPublisher
    closeSignal     chan struct{}
}

type JobResult struct {
    JobKey    int64
    Success   bool
    Result    interface{}
    Error     error
}

func NewJobResultCollector(resultPublisher *JobResultPublisher) *JobResultCollector {
    return &JobResultCollector{
        resultChan:      make(chan JobResult, 1000),
        resultPublisher: resultPublisher,
        closeSignal:     make(chan struct{}),
    }
}

func (c *JobResultCollector) Start(closeWait *sync.WaitGroup) {
    defer closeWait.Done()

    for {
        select {
        case result := <-c.resultChan:
            if result.Success {
                c.resultPublisher.PublishSuccess(result.JobKey, result.Result)
            } else {
                c.resultPublisher.PublishFailure(result.JobKey, result.Error)
            }
        case <-c.closeSignal:
            return
        }
    }
}

func (c *JobResultCollector) CollectResult(result JobResult) {
    c.resultChan <- result
}
