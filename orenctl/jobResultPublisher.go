package job_poller

import (
    "github.com/camunda-cloud/zeebe/clients/go/pkg/pb"
    "log"
)

type JobResultPublisher struct {
    client pb.GatewayClient
}

func NewJobResultPublisher(client pb.GatewayClient) *JobResultPublisher {
    return &JobResultPublisher{
        client: client,
    }
}

func (p *JobResultPublisher) PublishSuccess(key int64, result interface{}) {
    if err := p.client.CompleteJob(key, result); err != nil {
        log.Printf("Failed to complete job %d: %v\n", key, err)
    }
}

func (p *JobResultPublisher) PublishFailure(key int64, err error) {
    if err := p.client.SendJobsIncident(key, err.Error()); err != nil {
        log.Printf("Failed to send incident for job %d: %v\n", key, err)
    }
}
