package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetContainerID retrieves the container ID for a given container name
func GetContainerID(containerName string) (string, error) {
	// Prepare the command
	cmd := exec.Command("docker", "inspect", 
		fmt.Sprintf("--format={{.Id}}", containerName))

	// Run the command and capture output
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to inspect container %s: %w", containerName, err)
	}

	// Trim any whitespace and return the container ID
	return strings.TrimSpace(string(output)), nil
}

func main() {
	// Example usage
	containerName := "your_container_name"
	containerID, err := GetContainerID(containerName)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Container ID for %s: %s\n", containerName, containerID)
}