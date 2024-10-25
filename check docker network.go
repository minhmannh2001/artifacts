package main

import (
    "context"
    "fmt"
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/client"
)

// NetworkManager handles Docker network operations
func EnsureNetworkExists(networkName string) error {
    // Create a Docker client
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return fmt.Errorf("failed to create Docker client: %v", err)
    }
    defer cli.Close()

    // List all networks
    networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
    if err != nil {
        return fmt.Errorf("failed to list networks: %v", err)
    }

    // Check if network exists
    for _, network := range networks {
        if network.Name == networkName {
            fmt.Printf("Network '%s' already exists\n", networkName)
            return nil
        }
    }

    // Create network if it doesn't exist
    _, err = cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
        Driver:     "bridge",
        Attachable: true,
    })
    if err != nil {
        return fmt.Errorf("failed to create network: %v", err)
    }

    fmt.Printf("Network '%s' created successfully\n", networkName)
    return nil
}

func main() {
    // Example usage
    networkName := "my-network"
    err := EnsureNetworkExists(networkName)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}