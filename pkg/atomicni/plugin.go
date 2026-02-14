package atomicni

import (
	"context"
	"fmt"
	"net"

	"github.com/annis-souames/atomicni/pkg/config"
	"github.com/annis-souames/atomicni/pkg/ipam"
	"github.com/annis-souames/atomicni/pkg/netops"
	"github.com/annis-souames/atomicni/pkg/result"
	"github.com/containernetworking/cni/pkg/skel"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
)

// Plugin is the library entrypoint for CNI operations.
type Plugin struct {
	NetOps netops.NetOps
	IPAM   ipam.Allocator
}

// NewPlugin wires default Linux net operations and file-backed IPAM.
func NewPlugin() *Plugin {
	return &Plugin{
		NetOps: netops.NewNetlinkOps(),
		IPAM:   ipam.NewFileAllocator(),
	}
}

// Add performs CNI ADD for bridge + veth + IPv4 setup and returns CNI result.
func (p *Plugin) Add(ctx context.Context, args *skel.CmdArgs) (*current.Result, error) {
	if p.NetOps == nil {
		return nil, fmt.Errorf("plugin has nil NetOps")
	}
	if p.IPAM == nil {
		return nil, fmt.Errorf("plugin has nil IPAM allocator")
	}

	cfg, err := config.Parse(args.StdinData)
	if err != nil {
		return nil, fmt.Errorf("parse-config: %w", err)
	}

	targetNS, err := ns.GetNS(args.Netns)
	if err != nil {
		return nil, fmt.Errorf("open-netns: %w", err)
	}
	defer targetNS.Close()

	gatewayCIDR := &net.IPNet{IP: cloneIP(cfg.GatewayIP), Mask: cfg.SubnetNet.Mask}
	if err := p.NetOps.EnsureBridge(cfg.Bridge, gatewayCIDR); err != nil {
		return nil, fmt.Errorf("ensure-bridge: %w", err)
	}

	hostVethName := HostVethName(args.ContainerID)
	peerTempName := PeerVethTempName(args.ContainerID)

	rollback := rollbackStack{}
	fail := func(op string, opErr error) (*current.Result, error) {
		rollback.Run()
		return nil, fmt.Errorf("%s: %w", op, opErr)
	}

	if err := p.NetOps.CreateVethPair(hostVethName, peerTempName, cfg.MTU); err != nil {
		return fail("create-veth", err)
	}
	rollback.Push(func() {
		_ = p.NetOps.DeleteLink(hostVethName)
	})

	if err := p.NetOps.AttachHostVethToBridge(hostVethName, cfg.Bridge); err != nil {
		return fail("attach-host-veth", err)
	}

	if err := p.NetOps.MoveToNamespace(peerTempName, targetNS); err != nil {
		return fail("move-peer-to-netns", err)
	}
	rollback.Push(func() {
		_ = p.NetOps.DeleteLinkInNS(targetNS, args.IfName)
		_ = p.NetOps.DeleteLinkInNS(targetNS, peerTempName)
	})

	containerMAC, err := p.NetOps.PrepareContainerLink(targetNS, peerTempName, args.IfName)
	if err != nil {
		return fail("prepare-container-link", err)
	}

	ipReq := ipam.AllocationRequest{
		DataDir:     cfg.IPAM.DataDir,
		Network:     cfg.Name,
		ContainerID: args.ContainerID,
		Subnet:      cfg.SubnetNet,
		Gateway:     cfg.GatewayIP,
		RangeStart:  cfg.RangeStartIP,
		RangeEnd:    cfg.RangeEndIP,
	}
	allocatedIP, err := p.IPAM.Allocate(ctx, ipReq)
	if err != nil {
		return fail("alloc-ip", err)
	}
	rollback.Push(func() {
		_ = p.IPAM.Release(context.Background(), cfg.IPAM.DataDir, cfg.Name, args.ContainerID)
	})

	podCIDR := &net.IPNet{IP: cloneIP(allocatedIP), Mask: cfg.SubnetNet.Mask}
	if err := p.NetOps.AddAddressAndRoute(targetNS, args.IfName, podCIDR, cfg.GatewayIP); err != nil {
		return fail("configure-container-ip", err)
	}

	hostMAC, err := p.NetOps.GetLinkMAC(hostVethName)
	if err != nil {
		return fail("read-host-mac", err)
	}

	res := result.BuildAddResult(
		cfg.CNIVersion,
		hostVethName,
		hostMAC,
		args.IfName,
		containerMAC,
		args.Netns,
		podCIDR,
		cfg.GatewayIP,
	)
	return res, nil
}

// cloneIP returns a detached copy so callers can safely mutate the value.
func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// rollbackStack stores cleanup actions and executes them in reverse order.
type rollbackStack struct {
	fns []func()
}

// Push registers one cleanup function.
func (r *rollbackStack) Push(fn func()) {
	r.fns = append(r.fns, fn)
}

// Run executes all cleanup functions in LIFO order.
func (r *rollbackStack) Run() {
	for i := len(r.fns) - 1; i >= 0; i-- {
		r.fns[i]()
	}
}
