package main

import (
    "context"
    "fmt"
    "strings"
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/filters"
    "github.com/docker/docker/client"
)

// RemoveContainersByImage removes all containers with the specified image name
func RemoveContainersByImage(imageName string, force bool) error {
    // Create a Docker client
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return fmt.Errorf("failed to create Docker client: %v", err)
    }
    defer cli.Close()

    // List all containers (including stopped ones)
    containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
        All: true,
    })
    if err != nil {
        return fmt.Errorf("failed to list containers: %v", err)
    }

    // Keep track of removed containers
    removed := 0
    
    // Check each container
    for _, container := range containers {
        // Check if the container's image matches (either by name or ID)
        if strings.Contains(container.Image, imageName) {
            fmt.Printf("Found container: %s (ID: %s)\n", container.Names[0], container.ID[:12])
            
            // Remove container
            removeOptions := types.ContainerRemoveOptions{
                Force:         force, // If true, forces removal even if running
                RemoveVolumes: true,  // Remove associated volumes
            }
            
            if err := cli.ContainerRemove(ctx, container.ID, removeOptions); err != nil {
                fmt.Printf("Error removing container %s: %v\n", container.ID[:12], err)
                continue
            }
            
            fmt.Printf("Successfully removed container: %s\n", container.Names[0])
            removed++
        }
    }

    if removed == 0 {
        fmt.Printf("No containers found with image: %s\n", imageName)
    } else {
        fmt.Printf("Successfully removed %d container(s) with image: %s\n", removed, imageName)
    }

    return nil
}

// ListContainersByImage lists all containers with the specified image name
func ListContainersByImage(imageName string) error {
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return fmt.Errorf("failed to create Docker client: %v", err)
    }
    defer cli.Close()

    containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
        All: true,
    })
    if err != nil {
        return fmt.Errorf("failed to list containers: %v", err)
    }

    found := false
    fmt.Printf("Containers with image '%s':\n", imageName)
    for _, container := range containers {
        if strings.Contains(container.Image, imageName) {
            found = true
            fmt.Printf("- Name: %s\n  ID: %s\n  Status: %s\n  Created: %s\n\n",
                strings.TrimPrefix(container.Names[0], "/"),
                container.ID[:12],
                container.Status,
                container.State)
        }
    }

    if !found {
        fmt.Printf("No containers found with image: %s\n", imageName)
    }

    return nil
}

func main() {
    // Example usage
    imageName := "nginx"
    
    // First list containers
    fmt.Println("Listing containers before removal:")
    if err := ListContainersByImage(imageName); err != nil {
        fmt.Printf("Error listing containers: %v\n", err)
        return
    }
    
    // Then remove them
    fmt.Println("\nRemoving containers:")
    if err := RemoveContainersByImage(imageName, true); err != nil {
        fmt.Printf("Error removing containers: %v\n", err)
        return
    }
}