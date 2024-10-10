// Add this method to the TenantRouter struct

func (tr *TenantRouter) Stop() error {
	// Close all channels
	for _, ch := range tr.channels {
		close(ch)
	}

	// Stop all worker pools
	for _, pool := range tr.workerPools {
		pool.StopAndWait()
	}

	// Stop and remove all containers
	ctx := context.Background()
	for i := 0; i < cap(tr.containerPool.containers); i++ {
		container := <-tr.containerPool.containers
		err := tr.containerPool.client.ContainerStop(ctx, container.ID, container.StopOptions{})
		if err != nil {
			return fmt.Errorf("failed to stop container %s: %v", container.ID, err)
		}
		err = tr.containerPool.client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{})
		if err != nil {
			return fmt.Errorf("failed to remove container %s: %v", container.ID, err)
		}
	}

	// Close Docker client
	return tr.containerPool.client.Close()
}

// Update the main function to use the Stop method
func main() {
	// ... (previous main function code)

	// Use defer to ensure the router is stopped even if an error occurs
	defer func() {
		if err := router.Stop(); err != nil {
			fmt.Printf("Error stopping router: %v\n", err)
		}
	}()

	// ... (rest of the main function)
}
