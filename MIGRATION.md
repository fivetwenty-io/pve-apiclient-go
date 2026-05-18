# Migration notes

## Existing v3 users

The module path (`github.com/fivetwenty-io/pve-apiclient-go/v3`) is unchanged.
There are no breaking changes to the hand-written packages (`pkg/api/qemu`,
`pkg/api/lxc`, `pkg/api/network`, `pkg/api/cloudinit`, `pkg/api/storage`,
`pkg/api/tasks`, `pkg/client`, `pkg/auth`, `pkg/errors`, `pkg/websocket`).

### New generated packages are additive

The packages `pkg/api/access`, `pkg/api/cluster`, `pkg/api/clusterstorage`,
`pkg/api/nodes`, and `pkg/api/pools` are new. Importing them is opt-in.
Existing code that does not import these packages is unaffected.

### Boolean form encoding changed

The form encoder now serializes `bool` fields as `1` and `0`, matching
the encoding Proxmox VE expects. Previously, the encoder produced the
strings `true` and `false`. If your code constructed form values by
hand using the string literals `"true"` or `"false"` and passed them
through the client's `Post`/`Put` methods, those calls will now send
`"true"` or `"false"` verbatim (as string parameters, not booleans).
To get the correct encoding, use `bool`-typed fields in a struct or
pass `1`/`0` as integers.

### API token format enforced at construction

`NewAPITokenAuthenticatorFromString` and `ParseAPIToken` validate the
token string format (`USER@REALM!TOKENID=SECRET`) at call time and return
an error for malformed input. Previously, a malformed token could be stored
and would produce a bad `Authorization` header on first use. Update any
code that was tolerating or working around a silent bad-header condition.

### WebSocket write concurrency

If you embedded or extended `pkg/websocket.Client` and call `Send` or
`SendText` from multiple goroutines, the serialization is now handled
internally by `writeMu`. No caller-side locking is needed or recommended.
Remove any external mutex that was guarding writes to avoid a deadlock.
