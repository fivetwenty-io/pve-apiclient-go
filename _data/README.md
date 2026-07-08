# Proxmox API Specification Data

This directory holds the upstream Proxmox API specifications used as input
to the typed client codegen pipeline at `cmd/pvegen`: `apidoc.json` for
Proxmox VE, `pbs-apidoc.json` for Proxmox Backup Server, and
`pdm-apidoc.json` for Proxmox Datacenter Manager. For PVE, the
generator emits typed bindings for all six top-level namespaces:

- `/version`  → `pkg/api/version/`
- `/access`   → `pkg/api/access/`
- `/pools`    → `pkg/api/pools/`
- `/cluster`  → `pkg/api/cluster/`
- `/storage`  → `pkg/api/clusterstorage/` (renamed to avoid clashing with the
  hand-written `pkg/api/storage/` helpers that target
  `/nodes/{node}/storage/...`)
- `/nodes`    → `pkg/api/nodes/`

## Files

- `apidoc.json` — Recursive endpoint tree extracted from PVE
  (`pve-docs/api-viewer/apidoc.js`). Root is a JSON array of node objects;
  each node has `path`, `text`, `leaf`, `info` (map of HTTP method to
  endpoint definition), and optional `children`.

  **Current pin: PVE 9.2 — 444 endpoints / 675 method-operations.**

## Provenance

The spec is sourced from the published `pve.proxmox.com/pve-docs/api-viewer/`
static asset (or an equivalent running PVE deployment). It is the same data
the upstream API viewer uses to render its documentation, so it is the
canonical machine-readable definition of the REST surface.

## Regenerating

To refresh against a newer PVE release:

1. Fetch the upstream JS bundle:

   ```sh
   curl -sSL https://pve.proxmox.com/pve-docs/api-viewer/apidoc.js \
     -o /tmp/apidoc.js
   ```

2. Extract the JSON payload. The JS file assigns the schema to
   `const apiSchema = [ ... ];` and is followed by additional JavaScript
   (the api-viewer renderer), so a line-oriented `sed` strip will not work —
   the array must be bracket-matched from the assignment to its closing `]`.
   Extract and validate it as JSON in one step:

   ```sh
   python3 - <<'PY'
   import json, re
   src = open('/tmp/apidoc.js', encoding='utf-8').read()
   start = src.index('[', src.index('const apiSchema'))
   depth = 0
   for i in range(start, len(src)):
       depth += {'[': 1, ']': -1}.get(src[i], 0)
       if depth == 0:
           end = i + 1
           break
   schema = json.loads(src[start:end])  # raises if malformed
   json.dump(schema, open('_data/apidoc.json', 'w'))
   print(f'wrote {len(schema)} top-level nodes')
   PY
   ```

   (Adjust the variable name if upstream renames `apiSchema`. The
   `json.loads` call fails loudly if the extracted text is not valid JSON,
   so a clean exit means the payload parsed.)

3. Regenerate Go bindings:

   ```sh
   make generate
   ```

4. Run the verification target to confirm the tree is idempotent:

   ```sh
   make verify-generated
   ```

5. Run the full test suite:

   ```sh
   make check
   ```

## PBS specification (`pbs-apidoc.json`)

The same tree format published by the Proxmox Backup Server API viewer.
`cmd/pvegen --dialect pbs` reads it and emits `pkg/pbs/<ns>/` bindings for
ten namespaces (`access`, `admin`, `config`, `nodes`, `ping`, `pull`,
`push`, `status`, `tape`, `version`), skipping the `/backup` and `/reader`
HTTP/2 chunk-protocol endpoints and the `GET /` directory index.

**Current pin: fetched 2026-07-07 — 232 paths / 349 method-operations in
the API tree (346 generated).**

Dialect differences from the PVE spec (all tolerated by the generator):
`additionalProperties` is a JSON boolean rather than 0/1, `format` is a
nested schema object rather than a format-name string, `typetext` is
absent, and streaming endpoints carry `method: DOWNLOAD`/`UPLOAD` under
their GET/POST verb keys.

To refresh against a newer PBS release, follow the PVE steps above with:

- Source: `https://pbs.proxmox.com/docs/api-viewer/apidoc.js`
- Assignment to look for: `var apiSchema = [ ... ];` (not `const`)
- Output: `_data/pbs-apidoc.json`

The extracted array has three top-level nodes: the `/` API tree plus the
`/backup/_upgrade_` and `/reader/_upgrade_` protocol trees. Keep all three
— the generator skips the protocol trees itself, and dropping them would
make future spec diffs noisier.

## PDM specification (`pdm-apidoc.json`)

The same tree format published by the Proxmox Datacenter Manager API
viewer. `cmd/pvegen --dialect pdm` reads it and emits `pkg/pdm/<ns>/`
bindings for all thirteen namespaces (`access`, `auto-install` → package
`autoinstall`, `ceph`, `config`, `nodes`, `pbs`, `ping`, `pve`, `remotes`,
`resources`, `sdn`, `subscriptions`, `version`), skipping only the `GET /`
directory index. The `/pve` and `/pbs` trees are PDM's proxied per-remote
operations against managed PVE/PBS instances.

**Current pin: fetched 2026-07-08 from
`https://pdm.proxmox.com/docs/api-viewer/apidoc.js` — PDM 1.1.6, 327
method-operations.**

Dialect notes: the PDM spec shares the PBS encoding conventions (boolean
`additionalProperties`, nested `format` schemas) and adds an `unstable`
boolean field on endpoint info entries, which the generator ignores. The
extracted array has a single top-level node (the `/` API tree) — PDM has
no extra protocol trees.

To refresh against a newer PDM release, follow the PVE steps above with:

- Source: `https://pdm.proxmox.com/docs/api-viewer/apidoc.js`
- Assignment to look for: `var apiSchema = [ ... ];` (not `const`). The
  array is followed by a `;` and a license comment blob, so extract with a
  raw JSON decode starting after the `apiSchema = ` match (see the PVE
  bracket-matching script — either approach works).
- Output: `_data/pdm-apidoc.json`

## Versioning

`apidoc.json`, `pbs-apidoc.json`, and `pdm-apidoc.json` are treated as vendored inputs. A bump
to a newer PVE/PBS/PDM spec is a deliberate, reviewed change: it produces a
diff in `pkg/api/**/*_gen.go`, `pkg/pbs/**/*_gen.go`, or `pkg/pdm/**/*_gen.go` that callers can
inspect for breaking changes.
