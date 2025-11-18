package main

import (
	"context"
	"fmt"
	"log"

	"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/api/lxc"
	pve "github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client"
)

func main() {
	fmt.Println("=== LXC Container Management Example ===")
	fmt.Println()

	// Create PVE client
	client, err := pve.NewClient(pve.Options{
		Host:      "pve.example.com",
		Username:  "root@pam",
		Password:  "your-password",
		AutoLogin: true,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Create LXC client for node "pve"
	lxcClient := lxc.NewClient(client, "pve")
	ctx := context.Background()

	// Example 1: List all LXC containers
	fmt.Println("Example 1: List LXC Containers")

	containers, err := lxcClient.List(ctx)
	if err != nil {
		log.Printf("Failed to list containers: %v", err)
	} else {
		fmt.Printf("Found %d containers:\n", len(containers))

		for _, ct := range containers {
			fmt.Printf("  - CT %d: %s (status: %s)\n", ct.VMID, ct.Name, ct.Status)
		}
	}

	fmt.Println()

	// Example 2: Create a new LXC container
	fmt.Println("Example 2: Create LXC Container")

	config := lxc.ContainerConfig{
		VMID:         200,
		OSTemplate:   "local:vztmpl/ubuntu-22.04-standard_22.04-1_amd64.tar.zst",
		Hostname:     "test-ct",
		Description:  "Test container created via API",
		Memory:       1024, // 1GB
		Swap:         512,  // 512MB
		Cores:        2,
		RootFS:       "local-lvm:8", // 8GB root filesystem on local-lvm
		Net0:         "name=eth0,bridge=vmbr0,ip=dhcp",
		Unprivileged: true,
		Password:     "secret",
		Start:        false, // Don't auto-start after creation
		Storage:      "local-lvm",
	}

	upid, err := lxcClient.Create(ctx, config)
	if err != nil {
		log.Printf("Failed to create container: %v", err)
	} else {
		fmt.Printf("✓ Container creation task started: %s\n", upid)
		fmt.Println("  VMID: 200")
		fmt.Println("  Hostname: test-ct")
		fmt.Println("  Memory: 1024 MB")
		fmt.Println("  Note: Wait for task to complete before proceeding")
	}

	fmt.Println()

	// Example 3: Get container status
	fmt.Println("Example 3: Get Container Status")

	status, err := lxcClient.Status(ctx, 200)
	if err != nil {
		log.Printf("Failed to get status: %v", err)
	} else {
		fmt.Printf("Container 200 Status:\n")
		fmt.Printf("  Status: %s\n", status.Status)
		fmt.Printf("  Name: %s\n", status.Name)

		if status.Uptime > 0 {
			fmt.Printf("  Uptime: %d seconds\n", status.Uptime)
		}

		if status.CPUs > 0 {
			fmt.Printf("  CPUs: %d\n", status.CPUs)
		}

		if status.MaxMem > 0 {
			fmt.Printf("  Memory: %d MB / %d MB\n", status.Mem/(1024*1024), status.MaxMem/(1024*1024))
		}
	}

	fmt.Println()

	// Example 4: Start container
	fmt.Println("Example 4: Start Container")

	upid, err = lxcClient.Start(ctx, 200)
	if err != nil {
		log.Printf("Failed to start container: %v", err)
	} else {
		fmt.Printf("✓ Container 200 start task: %s\n", upid)
	}

	fmt.Println()

	// Example 5: Get container configuration
	fmt.Println("Example 5: Get Container Configuration")

	ctConfig, err := lxcClient.GetConfig(ctx, 200)
	if err != nil {
		log.Printf("Failed to get config: %v", err)
	} else {
		fmt.Println("Container 200 Configuration:")

		for key, value := range ctConfig {
			fmt.Printf("  %s: %v\n", key, value)
		}
	}

	fmt.Println()

	// Example 6: Update container configuration
	fmt.Println("Example 6: Update Container Configuration")

	updates := map[string]interface{}{
		"memory":      2048, // Increase to 2GB
		"cores":       4,    // Increase to 4 cores
		"description": "Updated via API",
	}

	err = lxcClient.UpdateConfig(ctx, 200, updates)
	if err != nil {
		log.Printf("Failed to update config: %v", err)
	} else {
		fmt.Println("✓ Container 200 configuration updated")
		fmt.Println("  Memory: 1024 MB → 2048 MB")
		fmt.Println("  Cores: 2 → 4")
	}

	fmt.Println()

	// Example 7: Clone container
	fmt.Println("Example 7: Clone Container")

	cloneOpts := lxc.CloneOptions{
		Hostname:    "test-ct-clone",
		Description: "Clone of test-ct",
		Full:        true, // Full copy (not linked)
	}

	upid, err = lxcClient.Clone(ctx, 200, 201, cloneOpts)
	if err != nil {
		log.Printf("Failed to clone container: %v", err)
	} else {
		fmt.Printf("✓ Container clone task started: %s\n", upid)
		fmt.Println("  Source: CT 200")
		fmt.Println("  Target: CT 201")
		fmt.Println("  Type: Full clone")
	}

	fmt.Println()

	// Example 8: Resize container disk
	fmt.Println("Example 8: Resize Container Disk")

	err = lxcClient.Resize(ctx, 200, "rootfs", "+2G")
	if err != nil {
		log.Printf("Failed to resize disk: %v", err)
	} else {
		fmt.Println("✓ Container 200 rootfs resized")
		fmt.Println("  Disk: rootfs")
		fmt.Println("  Size: +2G (added 2GB)")
	}

	fmt.Println()

	// Example 9: Graceful shutdown
	fmt.Println("Example 9: Shutdown Container")

	upid, err = lxcClient.Shutdown(ctx, 200, 60) // 60 second timeout
	if err != nil {
		log.Printf("Failed to shutdown container: %v", err)
	} else {
		fmt.Printf("✓ Container 200 shutdown task: %s\n", upid)
		fmt.Println("  Timeout: 60 seconds")
	}

	fmt.Println()

	// Example 10: Reboot container
	fmt.Println("Example 10: Reboot Container")

	upid, err = lxcClient.Reboot(ctx, 200)
	if err != nil {
		log.Printf("Failed to reboot container: %v", err)
	} else {
		fmt.Printf("✓ Container 200 reboot task: %s\n", upid)
	}

	fmt.Println()

	// Example 11: Stop container (forceful)
	fmt.Println("Example 11: Stop Container")

	upid, err = lxcClient.Stop(ctx, 200)
	if err != nil {
		log.Printf("Failed to stop container: %v", err)
	} else {
		fmt.Printf("✓ Container 200 stop task: %s\n", upid)
		fmt.Println("  Type: Forceful stop")
	}

	fmt.Println()

	// Example 12: Delete container
	fmt.Println("Example 12: Delete Container")

	upid, err = lxcClient.Delete(ctx, 200, true) // purge=true removes all data
	if err != nil {
		log.Printf("Failed to delete container: %v", err)
	} else {
		fmt.Printf("✓ Container 200 deletion task: %s\n", upid)
		fmt.Println("  Purge: true (all data will be removed)")
	}

	fmt.Println()

	// Summary
	fmt.Println("=== Examples Complete ===")
	fmt.Println()
	fmt.Println("Key Operations Demonstrated:")
	fmt.Println("1.  List - Enumerate all LXC containers")
	fmt.Println("2.  Create - Create new container from template")
	fmt.Println("3.  Status - Get container status and metrics")
	fmt.Println("4.  Start - Start a stopped container")
	fmt.Println("5.  GetConfig - Retrieve container configuration")
	fmt.Println("6.  UpdateConfig - Modify container settings")
	fmt.Println("7.  Clone - Create copy of existing container")
	fmt.Println("8.  Resize - Expand container disk space")
	fmt.Println("9.  Shutdown - Graceful shutdown with timeout")
	fmt.Println("10. Reboot - Restart container")
	fmt.Println("11. Stop - Forceful stop")
	fmt.Println("12. Delete - Remove container and data")
	fmt.Println()
	fmt.Println("Note: Most operations return a task UPID.")
	fmt.Println("Use the tasks API to monitor completion.")
}
