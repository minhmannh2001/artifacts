package job_poller

import (
    "context"
    "fmt"
    "github.com/camunda-cloud/zeebe/clients/go/pkg/entities"
    "github.com/camunda-cloud/zeebe/clients/go/pkg/pb"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    "io"
    "log"
    "sync"
    "time"
)

type JobPoller struct {
    client              pb.GatewayClient
    request             *pb.ActivateJobsRequest
    requestTimeout      time.Duration
    maxJobsActive       int
    initialPollInterval time.Duration
    pollInterval        time.Duration

    jobQueue           chan entities.Job
    dispatcherQueue    chan entities.Job
    workerFinished     chan bool
    closeSignal        chan struct{}
    remaining          int
    threshold          int
    shouldRetry        func(context.Context, error) bool
    backoffSupplier    BackoffSupplier
}

func NewJobPoller(client pb.GatewayClient, config JobPollerConfig) *JobPoller {
    return &JobPoller{
        client:              client,
        request:             config.Request,
        requestTimeout:      config.RequestTimeout,
        maxJobsActive:       config.MaxJobsActive,
        initialPollInterval: config.PollInterval,
        pollInterval:        config.PollInterval,
        jobQueue:           make(chan entities.Job, 1000),
        dispatcherQueue:    make(chan entities.Job, 1000),
        workerFinished:     make(chan bool),
        closeSignal:        make(chan struct{}),
        threshold:          config.Threshold,
        shouldRetry:        config.ShouldRetry,
        backoffSupplier:    config.BackoffSupplier,
    }
}

func (p *JobPoller) Start(closeWait *sync.WaitGroup) {
    defer closeWait.Done()

    // initial poll
    p.activateJobs()

    for {
        select {
        case <-p.workerFinished:
            p.remaining--
            p.pollInterval = p.initialPollInterval
        case <-time.After(p.pollInterval):
        case <-p.closeSignal:
            return
        }

        if p.shouldActivateJobs() {
            p.activateJobs()
        }
    }
}

func (p *JobPoller) activateJobs() {
    ctx, cancel := context.WithTimeout(context.Background(), p.requestTimeout)
    defer cancel()

    p.request.MaxJobsToActivate = int32(p.maxJobsActive - p.remaining)
    stream, err := p.openStream(ctx)
    if err != nil {
        log.Println(err.Error())
        return
    }

    for {
        response, err := stream.Recv()
        if err != nil {
            if err == io.EOF {
                break
            }

            if p.shouldRetry(ctx, err) {
                stream, err = p.openStream(ctx)
                if err != nil {
                    log.Printf("Failed to reopen job polling stream: %v\n", err)
                    break
                }
                continue
            }

            if status.Code(err) != codes.ResourceExhausted {
                log.Printf("Failed to activate jobs for poller '%s': %v\n", p.request.Worker, err)
            }

            switch status.Code(err) {
            case codes.ResourceExhausted, codes.Unavailable, codes.Internal:
                p.backoff()
            }
            break
        }

        p.remaining += len(response.Jobs)
        for _, job := range response.Jobs {
            p.dispatcherQueue <- entities.Job{ActivatedJob: job}
        }
    }
}

func (p *JobPoller) GetDispatcherQueue() chan entities.Job {
    return p.dispatcherQueue
}

// Other existing methods remain the same...
