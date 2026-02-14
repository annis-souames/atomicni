package atomicni

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/annis-souames/atomicni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/plugins/pkg/ns"
)

type mockNetOps struct {
	calls []string
}

func (m *mockNetOps) EnsureBridge(name string, gateway *net.IPNet) error {
	m.calls = append(m.calls, "EnsureBridge")
	return nil
}

func (m *mockNetOps) CreateVethPair(hostName, peerName string, mtu int) error {
	m.calls = append(m.calls, "CreateVethPair")
	return nil
}

func (m *mockNetOps) AttachHostVethToBridge(hostName, bridgeName string) error {
	m.calls = append(m.calls, "AttachHostVethToBridge")
	return nil
}

func (m *mockNetOps) MoveToNamespace(linkName string, target ns.NetNS) error {
	m.calls = append(m.calls, "MoveToNamespace")
	return nil
}

func (m *mockNetOps) PrepareContainerLink(target ns.NetNS, currentName, targetName string) (string, error) {
	m.calls = append(m.calls, "PrepareContainerLink")
	return "11:22:33:44:55:66", nil
}

func (m *mockNetOps) AddAddressAndRoute(target ns.NetNS, ifName string, addr *net.IPNet, gateway net.IP) error {
	m.calls = append(m.calls, "AddAddressAndRoute")
	return errors.New("boom")
}

func (m *mockNetOps) DeleteLink(name string) error {
	m.calls = append(m.calls, "DeleteLink")
	return nil
}

func (m *mockNetOps) DeleteLinkInNS(target ns.NetNS, name string) error {
	m.calls = append(m.calls, "DeleteLinkInNS")
	return nil
}

func (m *mockNetOps) GetLinkMAC(name string) (string, error) {
	m.calls = append(m.calls, "GetLinkMAC")
	return "aa:bb:cc:dd:ee:ff", nil
}

type mockAllocator struct {
	calls []string
}

func (m *mockAllocator) Allocate(_ context.Context, req ipam.AllocationRequest) (net.IP, error) {
	m.calls = append(m.calls, "Allocate")
	return net.ParseIP("10.22.0.10").To4(), nil
}

func (m *mockAllocator) Release(_ context.Context, dataDir, network, containerID string) error {
	m.calls = append(m.calls, "Release")
	return nil
}

func (m *mockAllocator) GetByContainer(_ context.Context, dataDir, network, containerID string) (net.IP, bool, error) {
	m.calls = append(m.calls, "GetByContainer")
	return nil, false, nil
}

func TestAddRollsBackOnConfigureFailure(t *testing.T) {
	nsPath, err := ns.GetCurrentNS()
	if err != nil {
		t.Fatalf("GetCurrentNS: %v", err)
	}
	defer nsPath.Close()

	netOps := &mockNetOps{}
	alloc := &mockAllocator{}
	p := &Plugin{NetOps: netOps, IPAM: alloc}

	args := &skel.CmdArgs{
		ContainerID: "test-container",
		Netns:       nsPath.Path(),
		IfName:      "eth0",
		StdinData: []byte(`{
			"cniVersion":"1.1.0",
			"name":"atomic-net",
			"type":"atomicni",
			"bridge":"atomic0",
			"subnet":"10.22.0.0/24",
			"gateway":"10.22.0.1",
			"ipam":{"dataDir":"/tmp/atomicni-test","rangeStart":"10.22.0.10","rangeEnd":"10.22.0.20"}
		}`),
	}

	_, err = p.Add(context.Background(), args)
	if err == nil {
		t.Fatalf("expected Add() failure")
	}

	if len(alloc.calls) < 2 || alloc.calls[0] != "Allocate" || alloc.calls[1] != "Release" {
		t.Fatalf("expected allocator rollback Allocate->Release, calls: %v", alloc.calls)
	}

	foundDeleteLink := false
	foundDeleteInNS := false
	for _, c := range netOps.calls {
		if c == "DeleteLink" {
			foundDeleteLink = true
		}
		if c == "DeleteLinkInNS" {
			foundDeleteInNS = true
		}
	}
	if !foundDeleteLink || !foundDeleteInNS {
		t.Fatalf("expected link cleanup calls, got %v", netOps.calls)
	}
}
