// Package lxc provides API operations for LXC container management.
package lxc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/fivetwenty-io/pve-apiclient-go/v3/pkg/client"
)

// Client provides LXC container management operations.
type Client struct {
	pveClient client.Client
	node      string
}

// NewClient creates a new LXC API client for the specified node.
func NewClient(pveClient client.Client, node string) *Client {
	return &Client{
		pveClient: pveClient,
		node:      node,
	}
}

// ContainerConfig represents LXC container configuration.
type ContainerConfig struct {
	VMID         int               `json:"vmid"`
	OSTemplate   string            `json:"ostemplate"`
	Hostname     string            `json:"hostname,omitempty"`
	Description  string            `json:"description,omitempty"`
	Memory       int               `json:"memory,omitempty"`       // MB
	Swap         int               `json:"swap,omitempty"`         // MB
	Cores        int               `json:"cores,omitempty"`        // CPU cores
	CPULimit     int               `json:"cpulimit,omitempty"`     // CPU limit (0-128)
	CPUUnits     int               `json:"cpuunits,omitempty"`     // CPU weight
	RootFS       string            `json:"rootfs,omitempty"`       // Root filesystem (e.g., "local:8")
	Net0         string            `json:"net0,omitempty"`         // Network config
	Unprivileged bool              `json:"unprivileged,omitempty"` // Unprivileged container
	Features     map[string]string `json:"features,omitempty"`     // Features (nesting, fuse, etc.)
	Password     string            `json:"password,omitempty"`     // Root password
	SSHKeys      string            `json:"ssh-public-keys,omitempty"`
	Nameserver   string            `json:"nameserver,omitempty"`
	Searchdomain string            `json:"searchdomain,omitempty"`
	Start        bool              `json:"start,omitempty"` // Start after creation
	Storage      string            `json:"storage,omitempty"`
	Pool         string            `json:"pool,omitempty"` // Resource pool
}

// ContainerStatus represents the current status of an LXC container.
type ContainerStatus struct {
	VMID   int    `json:"vmid"`
	Status string `json:"status"` // running, stopped, etc.
	Name   string `json:"name"`
	Uptime int64  `json:"uptime,omitempty"`
	CPUs   int    `json:"cpus,omitempty"`
	MaxMem int64  `json:"maxmem,omitempty"`
	Mem    int64  `json:"mem,omitempty"`
	MaxDisk int64 `json:"maxdisk,omitempty"`
	Disk   int64  `json:"disk,omitempty"`
}

// List returns all LXC containers on the node.
func (c *Client) List(ctx context.Context) ([]ContainerStatus, error) {
	path := fmt.Sprintf("/nodes/%s/lxc", c.node)

	resp, err := c.pveClient.GetCtx(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list LXC containers: %w", err)
	}

	// Parse response
	containers := []ContainerStatus{}
	if data, ok := resp.([]interface{}); ok {
		for _, item := range data {
			if ct, ok := item.(map[string]interface{}); ok {
				status := ContainerStatus{
					Status: getString(ct, "status"),
					Name:   getString(ct, "name"),
				}
				if vmid, ok := ct["vmid"].(float64); ok {
					status.VMID = int(vmid)
				}
				if uptime, ok := ct["uptime"].(float64); ok {
					status.Uptime = int64(uptime)
				}
				containers = append(containers, status)
			}
		}
	}

	return containers, nil
}

// Create creates a new LXC container.
func (c *Client) Create(ctx context.Context, config ContainerConfig) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc", c.node)

	params := map[string]interface{}{
		"vmid":      config.VMID,
		"ostemplate": config.OSTemplate,
	}

	// Add optional parameters
	if config.Hostname != "" {
		params["hostname"] = config.Hostname
	}
	if config.Description != "" {
		params["description"] = config.Description
	}
	if config.Memory > 0 {
		params["memory"] = config.Memory
	}
	if config.Swap > 0 {
		params["swap"] = config.Swap
	}
	if config.Cores > 0 {
		params["cores"] = config.Cores
	}
	if config.CPULimit > 0 {
		params["cpulimit"] = config.CPULimit
	}
	if config.CPUUnits > 0 {
		params["cpuunits"] = config.CPUUnits
	}
	if config.RootFS != "" {
		params["rootfs"] = config.RootFS
	}
	if config.Net0 != "" {
		params["net0"] = config.Net0
	}
	if config.Unprivileged {
		params["unprivileged"] = 1
	}
	if config.Password != "" {
		params["password"] = config.Password
	}
	if config.SSHKeys != "" {
		params["ssh-public-keys"] = config.SSHKeys
	}
	if config.Nameserver != "" {
		params["nameserver"] = config.Nameserver
	}
	if config.Searchdomain != "" {
		params["searchdomain"] = config.Searchdomain
	}
	if config.Start {
		params["start"] = 1
	}
	if config.Storage != "" {
		params["storage"] = config.Storage
	}
	if config.Pool != "" {
		params["pool"] = config.Pool
	}

	// Add features
	for key, value := range config.Features {
		params[key] = value
	}

	resp, err := c.pveClient.PostCtx(ctx, path, params)
	if err != nil {
		return "", fmt.Errorf("failed to create LXC container: %w", err)
	}

	// Extract UPID (task ID)
	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// Status returns the current status of an LXC container.
func (c *Client) Status(ctx context.Context, vmid int) (*ContainerStatus, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", c.node, vmid)

	resp, err := c.pveClient.GetCtx(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}

	status := &ContainerStatus{VMID: vmid}

	if data, ok := resp.(map[string]interface{}); ok {
		status.Status = getString(data, "status")
		status.Name = getString(data, "name")
		if uptime, ok := data["uptime"].(float64); ok {
			status.Uptime = int64(uptime)
		}
		if cpus, ok := data["cpus"].(float64); ok {
			status.CPUs = int(cpus)
		}
		if maxmem, ok := data["maxmem"].(float64); ok {
			status.MaxMem = int64(maxmem)
		}
		if mem, ok := data["mem"].(float64); ok {
			status.Mem = int64(mem)
		}
		if maxdisk, ok := data["maxdisk"].(float64); ok {
			status.MaxDisk = int64(maxdisk)
		}
		if disk, ok := data["disk"].(float64); ok {
			status.Disk = int64(disk)
		}
	}

	return status, nil
}

// Start starts an LXC container.
func (c *Client) Start(ctx context.Context, vmid int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/start", c.node, vmid)

	resp, err := c.pveClient.PostCtx(ctx, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// Stop stops an LXC container.
func (c *Client) Stop(ctx context.Context, vmid int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", c.node, vmid)

	resp, err := c.pveClient.PostCtx(ctx, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to stop container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// Shutdown gracefully shuts down an LXC container.
func (c *Client) Shutdown(ctx context.Context, vmid int, timeout int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/shutdown", c.node, vmid)

	params := map[string]interface{}{}
	if timeout > 0 {
		params["timeout"] = timeout
	}

	resp, err := c.pveClient.PostCtx(ctx, path, params)
	if err != nil {
		return "", fmt.Errorf("failed to shutdown container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// Reboot reboots an LXC container.
func (c *Client) Reboot(ctx context.Context, vmid int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/reboot", c.node, vmid)

	resp, err := c.pveClient.PostCtx(ctx, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to reboot container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// Delete deletes an LXC container.
func (c *Client) Delete(ctx context.Context, vmid int, purge bool) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d", c.node, vmid)

	params := map[string]interface{}{}
	if purge {
		params["purge"] = 1
	}

	resp, err := c.pveClient.DeleteCtx(ctx, path, params)
	if err != nil {
		return "", fmt.Errorf("failed to delete container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// GetConfig retrieves the configuration of an LXC container.
func (c *Client) GetConfig(ctx context.Context, vmid int) (map[string]interface{}, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", c.node, vmid)

	resp, err := c.pveClient.GetCtx(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container config: %w", err)
	}

	if config, ok := resp.(map[string]interface{}); ok {
		return config, nil
	}

	return nil, fmt.Errorf("unexpected response type: %T", resp)
}

// UpdateConfig updates the configuration of an LXC container.
func (c *Client) UpdateConfig(ctx context.Context, vmid int, config map[string]interface{}) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", c.node, vmid)

	_, err := c.pveClient.PutCtx(ctx, path, config)
	if err != nil {
		return fmt.Errorf("failed to update container config: %w", err)
	}

	return nil
}

// Clone clones an LXC container.
func (c *Client) Clone(ctx context.Context, vmid int, newID int, opts CloneOptions) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/clone", c.node, vmid)

	params := map[string]interface{}{
		"newid": newID,
	}

	if opts.Hostname != "" {
		params["hostname"] = opts.Hostname
	}
	if opts.Description != "" {
		params["description"] = opts.Description
	}
	if opts.Target != "" {
		params["target"] = opts.Target
	}
	if opts.Pool != "" {
		params["pool"] = opts.Pool
	}
	if opts.Storage != "" {
		params["storage"] = opts.Storage
	}
	if opts.Full {
		params["full"] = 1
	}

	resp, err := c.pveClient.PostCtx(ctx, path, params)
	if err != nil {
		return "", fmt.Errorf("failed to clone container %d: %w", vmid, err)
	}

	if upid, ok := resp.(string); ok {
		return upid, nil
	}

	return "", fmt.Errorf("unexpected response type: %T", resp)
}

// CloneOptions represents options for cloning a container.
type CloneOptions struct {
	Hostname    string
	Description string
	Target      string // Target node
	Pool        string // Resource pool
	Storage     string // Target storage
	Full        bool   // Full copy (not linked)
}

// Resize resizes a container disk.
func (c *Client) Resize(ctx context.Context, vmid int, disk string, size string) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/resize", c.node, vmid)

	params := map[string]interface{}{
		"disk": disk,
		"size": size,
	}

	_, err := c.pveClient.PutCtx(ctx, path, params)
	if err != nil {
		return fmt.Errorf("failed to resize container disk: %w", err)
	}

	return nil
}

// Helper function to safely get string from map.
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// Helper function to safely get int from map.
func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	if val, ok := m[key].(string); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return 0
}
