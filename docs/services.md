# Service APIs (CPI-Oriented)

This document summarizes the new CPI-style service packages, their purpose, and key method signatures. All methods accept a `context.Context` and use the wired internal HTTP client (TLS, retries, auth middleware). Return values follow Proxmox semantics (e.g., UPID for async operations) and use `pkg/errors` types for API failures.

## Packages

- `pkg/api/qemu`: VM lifecycle, disk operations, snapshots
- `pkg/api/storage`: volume create/delete/exists
- `pkg/api/network`: bridge ensure/delete/reload
- `pkg/api/tasks`: task wait/status helpers
- `pkg/api/cloudinit`: cloud-init attach and IP config helpers

## QEMU (`pkg/api/qemu`)

Key methods:

```
Create(ctx, node string, params map[string]interface{}) (string /*UPID*/, error)
Config(ctx, node string, vmid int) (map[string]interface{}, error)
Status(ctx, node string, vmid int) (map[string]interface{}, error)
Start/Stop/Reset(ctx, node string, vmid int) (string /*UPID*/, error)
Clone(ctx, node string, vmid int, params map[string]interface{}) (string /*UPID*/, error)
Template(ctx, node string, vmid int) (string /*UPID*/, error)

AttachDisk(ctx, node string, vmid int, volid string, bus string, opts *AttachOpts) (string /*diskID*/, error)
DetachDisk(ctx, node string, vmid int, diskID string) error
ResizeDisk(ctx, node string, vmid int, diskID string, sizeGiB int) (string /*UPID*/, error)

Snapshot(ctx, node string, vmid int, name string, opts map[string]interface{}) (string /*UPID*/, error)
DeleteSnapshot(ctx, node string, vmid int, name string) error
ListSnapshots(ctx, node string, vmid int) ([]map[string]interface{}, error)
RollbackSnapshot(ctx, node string, vmid int, name string) (string /*UPID*/, error)
```

Notes:
- `AttachDisk` computes the next bus index (scsi|virtio|ide|sata) from current config (unless `DiskID` is supplied) and PUTs VM config with `<diskID>=<volid>`.
- `ResizeDisk` formats size as `+<n>G`.

## Storage (`pkg/api/storage`)

```
CreateVolume(ctx, node, storage string, sizeGiB int, format string, vmid int, name string) (string /*volid*/, error)
DeleteVolume(ctx, node, storage, volume string) error
Exists(ctx, node, storage, volume string) (bool, error)
```

- `CreateVolume` posts to `/nodes/{node}/storage/{storage}/content` with size in bytes; returns `volid` if present.
- `Exists` returns `false` on 404 using `errors.APIError.IsNotFound()`.

## Network (`pkg/api/network`)

```
EnsureBridge(ctx, node, bridge string, params map[string]interface{}) error
DeleteBridge(ctx, node, bridge string) error
BridgeExists(ctx, node, bridge string) (bool, error)
Reload(ctx, node string) error
```

- `EnsureBridge` posts with `type=bridge` and `iface=<bridge>` plus extra params.
- `Reload` posts `reload=1` to apply changes.

## Tasks (`pkg/api/tasks`)

```
Wait(ctx, node, upid string, opts *WaitOptions) (*Status, error)

type WaitOptions struct { TimeoutSeconds int; IntervalMillis int }
type Status struct { Status, ExitStatus, UpID string }
```

- Polls `/nodes/{node}/tasks/{upid}/status` until `status=stopped`; success when `exitstatus=OK`.

## Cloud-Init (`pkg/api/cloudinit`)

```
BuildIPConfig(networks map[string]any) (map[string]string, error)
    BuildIPConfigs(specs []NICSpec, globalNameservers []string) (map[string]string, error)
    BuildIPConfigsFromCPISpec(spec map[string]any) (map[string]string, error)
Attach(ctx, node string, vmid int, storage string, userData []byte) error
AttachWithNetwork(ctx, node string, vmid int, storage string, userData, networkData []byte) error
```

- Attach sets `ide2=<storage>:cloudinit`. If `userData` provided, uploads to `snippets` and sets `cicustom=user=<storage>:snippets/<file>`. `AttachWithNetwork` also uploads network-data and sets `cicustom` with both `user=` and `network=`.
