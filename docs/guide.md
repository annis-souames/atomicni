# AtomicNI Code Guide

This document explains how the AtomicNI CNI plugin is structured and what happens during execution.

## 1. High-level architecture

AtomicNI is split into small packages with clear responsibilities:

- `main.go`: CNI executable entrypoint.
- `cmd/`: maps CNI lifecycle commands (`ADD`, `DEL`, `CHECK`) to library calls.
- `pkg/atomicni/`: orchestrates the full CNI add workflow.
- `pkg/config/`: parses and validates CNI JSON config from stdin.
- `pkg/netops/`: performs Linux network actions using `ip` commands.
- `pkg/ipam/`: allocates/releases pod IPv4 addresses with persistent local state.
- `pkg/result/`: builds a CNI-compliant result object for runtime output.

## 2. Runtime command flow

### Step 1: CNI invokes executable

`main.go` registers command handlers using CNI skeleton helpers:

- `cmd.Add`
- `cmd.Del`
- `cmd.Check`

For now, the implemented path is `ADD`.

### Step 2: `cmd.Add` calls library plugin

`cmd.Add` creates `atomicni.NewPlugin()` and calls `plugin.Add(...)`.

The plugin is used as a library entrypoint and composes:

- `NetOps`: concrete Linux networking operations.
- `IPAM`: file-backed address allocator.

### Step 3: config is parsed and validated

`config.Parse` reads stdin JSON and validates:

- required fields (`name`, `bridge`, `subnet`, `gateway`)
- IPv4-only restrictions
- gateway inside subnet and not network/broadcast
- optional allocation range validity
- defaults:
  - `mtu` defaults to `1500`
  - `ipam.dataDir` defaults to `/var/lib/atomicni`
  - range defaults to first/last usable host of subnet

### Step 4: target network namespace is opened

The plugin opens container netns path from `args.Netns` using CNI ns helpers.

### Step 5: bridge is prepared

`NetOps.EnsureBridge(...)` ensures the bridge exists, is up, and has the configured gateway CIDR.

### Step 6: veth pair is created and moved

The plugin computes deterministic interface names from container ID:

- host side: `HostVethName(...)`
- peer temp name: `PeerVethTempName(...)`

Then it:

- creates the veth pair
- attaches host veth to bridge
- moves peer side into container netns
- renames peer to CNI interface name (usually `eth0`) inside netns

### Step 7: IP address is allocated

`IPAM.Allocate(...)` allocates one IPv4 for this container/network pair.

Important behavior:

- allocation is idempotent per container ID
- allocator stores state on disk
- allocation uses next-fit cursor via `LastReserved`
- network, broadcast, gateway, and already-used IPs are skipped

### Step 8: pod interface is configured

Inside container netns, AtomicNI configures:

- pod IPv4 address
- default route via configured gateway

### Step 9: CNI result is produced

`result.BuildAddResult(...)` builds CNI result with:

- host and container interfaces
- allocated IP/gateway
- default route (`0.0.0.0/0`)

`cmd.Add` prints this result to stdout via CNI types API.

## 3. Rollback and failure safety

`pkg/atomicni/plugin.go` uses an internal rollback stack.

When a step fails after partial setup, cleanup handlers run in reverse order:

- delete created links
- release allocated IP

This keeps host/container networking and IPAM state consistent after errors.

## 4. IPAM persistence model

`pkg/ipam/store.go` manages on-disk state:

- one lock file per network: `<network>.lock`
- one state file per network: `<network>.json`

State maps:

- `containerToIP`: container ID -> IP
- `ipToContainer`: IP -> container ID
- `lastReserved`: cursor anchor for next-fit allocation

Consistency details:

- state updates are guarded by `flock` on the lock file
- writes are atomic (write temp file then rename)

This enables concurrent CNI calls without duplicate allocations.

## 5. Test coverage overview

- `pkg/config/config_test.go`: validation/defaulting rules.
- `pkg/ipam/allocator_test.go`: sequential, idempotent, persistent, concurrent allocation scenarios.
- `pkg/atomicni/plugin_test.go`: verifies rollback on configuration failure.
- `pkg/result/result_test.go`: validates generated CNI result shape.
- `pkg/atomicni/names_test.go`: deterministic and length-safe interface naming.

## 6. Current limitations

- `DEL` and `CHECK` command handlers are placeholders.
- Network implementation is Linux-specific and uses the `ip` tool.
- IPv6 is not implemented.

## 7. Suggested next extension path

1. Implement `Plugin.Del(...)` to delete links and release IPs.
2. Implement `Plugin.Check(...)` to verify desired state.
3. Add integration tests in a dedicated network namespace fixture.
