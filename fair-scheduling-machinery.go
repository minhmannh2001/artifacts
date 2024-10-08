package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RichardKnop/machinery/v2"
	"github.com/RichardKnop/machinery/v2/config"
	"github.com/RichardKnop/machinery/v2/tasks"
	"github.com/go-redis/redis/v8"
)

type FairScheduler struct {
	server       *machinery.Server
	tenantQueues map[string]string
	tenants      []string
	redisClient  *redis.Client
}

type TaskData struct {
	TenantID string      `json:"tenant_id"`
	TaskType string      `json:"task_type"`
	Payload  interface{} `json:"payload"`
}

func NewFairScheduler(redisURL string, tenants []string) (*FairScheduler, error) {
	cnf := &config.Config{
		Broker:        redisURL,
		ResultBackend: redisURL,
		DefaultQueue:  "machinery_tasks",
	}

	server, err := machinery.NewServer(cnf)
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: redisURL[8:], // Remove "redis://" prefix
	})

	tenantQueues := make(map[string]string)
	for _, tenant := range tenants {
		tenantQueues[tenant] = fmt.Sprintf("tenant:%s:tasks", tenant)
	}

	return &FairScheduler{
		server:       server,
		tenantQueues: tenantQueues,
		tenants:      tenants,
		redisClient:  redisClient,
	}, nil
}

func (fs *FairScheduler) EnqueueTask(tenant string, taskType string, payload interface{}) error {
	taskData := TaskData{
		TenantID: tenant,
		TaskType: taskType,
		Payload:  payload,
	}

	taskDataJSON, err := json.Marshal(taskData)
	if err != nil {
		return err
	}

	taskID := fmt.Sprintf("task:%s:%d", tenant, time.Now().UnixNano())

	err = fs.redisClient.Set(context.Background(), taskID, taskDataJSON, 24*time.Hour).Err()
	if err != nil {
		return err
	}

	signature := &tasks.Signature{
		Name: "processTask",
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: taskID,
			},
		},
	}

	_, err = fs.server.SendTask(signature)
	if err != nil {
		return err
	}

	return fs.redisClient.LPush(context.Background(), fs.tenantQueues[tenant], taskID).Err()
}

func (fs *FairScheduler) processTask(taskID string) error {
	taskDataJSON, err := fs.redisClient.Get(context.Background(), taskID).Bytes()
	if err != nil {
		return err
	}

	var taskData TaskData
	err = json.Unmarshal(taskDataJSON, &taskData)
	if err != nil {
		return err
	}

	fmt.Printf("Processing task for tenant %s, type: %s, payload: %v\n", taskData.TenantID, taskData.TaskType, taskData.Payload)

	// Implement your task processing logic here
	switch taskData.TaskType {
	case "email":
		// Process email task
	case "report":
		// Generate report
	default:
		fmt.Printf("Unknown task type: %s\n", taskData.TaskType)
	}

	return fs.redisClient.Del(context.Background(), taskID).Err()
}

func (fs *FairScheduler) StartWorker(concurrency int) error {
	fs.server.RegisterTask("processTask", fs.processTask)

	worker := fs.server.NewWorker("fair_worker", concurrency)

	for i := 0; i < concurrency; i++ {
		go fs.fairDistributionLoop()
	}

	return worker.Launch()
}

func (fs *FairScheduler) fairDistributionLoop() {
	for {
		for _, tenant := range fs.tenants {
			taskID, err := fs.redisClient.RPop(context.Background(), fs.tenantQueues[tenant]).Result()
			if err == redis.Nil {
				continue
			} else if err != nil {
				fmt.Printf("Error getting task for tenant %s: %v\n", tenant, err)
				continue
			}

			signature := &tasks.Signature{
				Name: "processTask",
				Args: []tasks.Arg{
					{
						Type:  "string",
						Value: taskID,
					},
				},
			}

			_, err = fs.server.SendTask(signature)
			if err != nil {
				fmt.Printf("Error queueing task %s: %v\n", taskID, err)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func main() {
	tenants := []string{"tenant1", "tenant2", "tenant3"}
	scheduler, err := NewFairScheduler("redis://localhost:6379", tenants)
	if err != nil {
		panic(err)
	}

	go func() {
		err := scheduler.StartWorker(5) // Start 5 worker goroutines
		if err != nil {
			panic(err)
		}
	}()

	// Example usage
	for i := 0; i < 10; i++ {
		for _, tenant := range tenants {
			err := scheduler.EnqueueTask(tenant, "email", map[string]interface{}{
				"to":      "user@example.com",
				"subject": fmt.Sprintf("Test email %d", i),
			})
			if err != nil {
				fmt.Printf("Error enqueueing task: %v\n", err)
			}
		}
	}

	// Keep the main goroutine running
	select {}
}
