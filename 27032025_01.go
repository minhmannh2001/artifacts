package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func main() {
	// Create a Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}
	defer cli.Close()

	// Pull the image (replace with the image you want to use)
	imageName := "ubuntu:latest"
	reader, err := cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		log.Fatalf("Failed to pull image: %v", err)
	}
	io.Copy(os.Stdout, reader)

	// Create the container
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		Cmd:   []string{"bash"},
		Tty:   false,
		// Enable stdin for input
		OpenStdin: true,
	}, nil, nil, nil, "")
	if err != nil {
		log.Fatalf("Failed to create container: %v", err)
	}

	// Start the container
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}

	// Create pipes for stdin and stdout
	reader, writer := io.Pipe()
	
	// Goroutine to read container stdout
	go func() {
		defer reader.Close()
		
		// Attach to the container to get logs/stdout
		out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			log.Printf("Failed to get container logs: %v", err)
			return
		}
		defer out.Close()

		// Copy stdout with potential multiplexing
		_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
		if err != nil {
			log.Printf("Error copying container output: %v", err)
		}
	}()

	// Send input to container stdin
	go func() {
		defer writer.Close()
		
		// Example input - you can modify this or replace with your own input method
		inputData := "echo 'Hello from stdin!'\nls -l\n"
		
		// Create an exec instance to run commands
		exec, err := cli.ContainerExecCreate(ctx, resp.ID, types.ExecConfig{
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Cmd:          []string{"bash"},
			Tty:          false,
		})
		if err != nil {
			log.Printf("Failed to create exec instance: %v", err)
			return
		}

		// Attach to the exec instance
		hijack, err := cli.ContainerExecAttach(ctx, exec.ID, types.ExecStartCheck{})
		if err != nil {
			log.Printf("Failed to attach to exec instance: %v", err)
			return
		}
		defer hijack.Close()

		// Write input to the exec instance
		_, err = hijack.Conn.Write([]byte(inputData))
		if err != nil {
			log.Printf("Failed to write to container: %v", err)
		}
	}()

	// Wait for the container to finish
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("Error waiting for container: %v", err)
		}
	case status := <-statusCh:
		fmt.Printf("Container exited with status %d\n", status.StatusCode)
	}

	// Optional: Remove the container
	err = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		log.Printf("Failed to remove container: %v", err)
	}
}