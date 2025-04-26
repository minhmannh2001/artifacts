package job_poller

import (
    "sync"
)

type JobOrchestrator struct {
    poller          *JobPoller
    dispatcher      *JobDispatcher
    resultCollector *JobResultCollector
    closeWait       sync.WaitGroup
}

func NewJobOrchestrator(
    client pb.GatewayClient,
    services *Services,
    pollerConfig JobPollerConfig,
) *JobOrchestrator {
    resultPublisher := NewJobResultPublisher(client)
    
    return &JobOrchestrator{
        poller:          NewJobPoller(client, pollerConfig),
        dispatcher:      NewJobDispatcher(services, resultPublisher),
        resultCollector: NewJobResultCollector(resultPublisher),
    }
}

func (o *JobOrchestrator) Start() {
    o.closeWait.Add(3)
    
    // Connect components
    o.dispatcher.jobQueue = o.poller.GetDispatcherQueue()
    
    // Start components
    go o.poller.Start(&o.closeWait)
    go o.dispatcher.Start(&o.closeWait)
    go o.resultCollector.Start(&o.closeWait)
}

func (o *JobOrchestrator) Stop() {
    o.poller.closeSignal <- struct{}{}
    o.dispatcher.closeSignal <- struct{}{}
    o.resultCollector.closeSignal <- struct{}{}
    o.closeWait.Wait()
}
