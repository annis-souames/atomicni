package ipam

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// AllocationRequest describes one IPv4 allocation request.
type AllocationRequest struct {
	DataDir     string
	Network     string
	ContainerID string
	Subnet      *net.IPNet
	Gateway     net.IP
	RangeStart  net.IP
	RangeEnd    net.IP
}

// Allocator manages per-network IPv4 allocation.
type Allocator interface {
	Allocate(ctx context.Context, req AllocationRequest) (net.IP, error)
	Release(ctx context.Context, dataDir, network, containerID string) error
	GetByContainer(ctx context.Context, dataDir, network, containerID string) (net.IP, bool, error)
}

// FileAllocator keeps allocation state on local disk.
type FileAllocator struct{}

func NewFileAllocator() *FileAllocator {
	return &FileAllocator{}
}

func (a *FileAllocator) Allocate(_ context.Context, req AllocationRequest) (net.IP, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	lockFile, statePath, err := lockNetwork(req.DataDir, req.Network)
	if err != nil {
		return nil, err
	}
	defer unlockNetwork(lockFile)

	st, err := loadState(statePath)
	if err != nil {
		return nil, err
	}

	if existing, ok := st.ContainerToIP[req.ContainerID]; ok {
		ip := net.ParseIP(existing).To4()
		if ip == nil {
			return nil, fmt.Errorf("stored IP for container %q is invalid: %q", req.ContainerID, existing)
		}
		st.IPToContainer[ip.String()] = req.ContainerID
		if err := saveState(statePath, st); err != nil {
			return nil, err
		}
		return ip, nil
	}

	selected, err := a.findNextIP(st, req)
	if err != nil {
		return nil, err
	}

	selectedStr := selected.String()
	st.ContainerToIP[req.ContainerID] = selectedStr
	st.IPToContainer[selectedStr] = req.ContainerID
	st.LastReserved = selectedStr
	if err := saveState(statePath, st); err != nil {
		return nil, err
	}

	return selected, nil
}

func (a *FileAllocator) Release(_ context.Context, dataDir, network, containerID string) error {
	if network == "" || containerID == "" {
		return errors.New("network and containerID are required")
	}

	lockFile, statePath, err := lockNetwork(dataDir, network)
	if err != nil {
		return err
	}
	defer unlockNetwork(lockFile)

	st, err := loadState(statePath)
	if err != nil {
		return err
	}

	ip, ok := st.ContainerToIP[containerID]
	if !ok {
		return nil
	}
	delete(st.ContainerToIP, containerID)
	delete(st.IPToContainer, ip)

	return saveState(statePath, st)
}

func (a *FileAllocator) GetByContainer(_ context.Context, dataDir, network, containerID string) (net.IP, bool, error) {
	if network == "" || containerID == "" {
		return nil, false, errors.New("network and containerID are required")
	}

	lockFile, statePath, err := lockNetwork(dataDir, network)
	if err != nil {
		return nil, false, err
	}
	defer unlockNetwork(lockFile)

	st, err := loadState(statePath)
	if err != nil {
		return nil, false, err
	}

	ipStr, ok := st.ContainerToIP[containerID]
	if !ok {
		return nil, false, nil
	}
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return nil, false, fmt.Errorf("stored IP for container %q is invalid: %q", containerID, ipStr)
	}
	return ip, true, nil
}

func (a *FileAllocator) findNextIP(st *state, req AllocationRequest) (net.IP, error) {
	start := ipv4ToUint(req.RangeStart)
	end := ipv4ToUint(req.RangeEnd)
	count := end - start + 1

	cursor := start
	if st.LastReserved != "" {
		last := net.ParseIP(st.LastReserved).To4()
		if last != nil {
			lastUint := ipv4ToUint(last)
			if lastUint >= start && lastUint <= end {
				cursor = lastUint + 1
			}
		}
	}
	if cursor > end {
		cursor = start
	}

	networkIP, broadcastIP := networkAndBroadcast(req.Subnet)
	gateway := req.Gateway.To4()

	for i := uint32(0); i < count; i++ {
		candidate := cursor + i
		if candidate > end {
			candidate = start + (candidate - end - 1)
		}

		ip := uintToIPv4(candidate)
		if ip.Equal(networkIP) || ip.Equal(broadcastIP) || ip.Equal(gateway) {
			continue
		}
		if _, inUse := st.IPToContainer[ip.String()]; inUse {
			continue
		}
		return ip, nil
	}

	return nil, errors.New("no available IP addresses")
}

func validateRequest(req AllocationRequest) error {
	if req.DataDir == "" {
		return errors.New("dataDir is required")
	}
	if req.Network == "" {
		return errors.New("network is required")
	}
	if req.ContainerID == "" {
		return errors.New("containerID is required")
	}
	if req.Subnet == nil {
		return errors.New("subnet is required")
	}
	if req.Subnet.IP.To4() == nil {
		return errors.New("only IPv4 subnets are supported")
	}
	if req.Gateway.To4() == nil {
		return errors.New("gateway must be IPv4")
	}
	if req.RangeStart.To4() == nil || req.RangeEnd.To4() == nil {
		return errors.New("range bounds must be IPv4")
	}
	if !req.Subnet.Contains(req.RangeStart) || !req.Subnet.Contains(req.RangeEnd) {
		return errors.New("allocation range must be inside subnet")
	}
	if ipv4ToUint(req.RangeStart) > ipv4ToUint(req.RangeEnd) {
		return errors.New("rangeStart must be <= rangeEnd")
	}
	return nil
}

func networkAndBroadcast(subnet *net.IPNet) (net.IP, net.IP) {
	network := subnet.IP.Mask(subnet.Mask).To4()
	mask := net.IP(subnet.Mask).To4()
	broadcast := make(net.IP, len(network))
	for i := range network {
		broadcast[i] = network[i] | ^mask[i]
	}
	return network, broadcast
}

func ipv4ToUint(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uintToIPv4(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4()
}
