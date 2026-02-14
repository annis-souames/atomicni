package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
)

const (
	DefaultMTU     = 1500
	DefaultDataDir = "/var/lib/atomicni"
)

// IPAMConfig configures local IP allocation persistence and optional range bounds.
type IPAMConfig struct {
	DataDir    string `json:"dataDir"`
	RangeStart string `json:"rangeStart,omitempty"`
	RangeEnd   string `json:"rangeEnd,omitempty"`
}

// NetworkConfig is AtomicNI plugin configuration loaded from CNI stdin.
type NetworkConfig struct {
	CNIVersion string     `json:"cniVersion"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Bridge     string     `json:"bridge"`
	Subnet     string     `json:"subnet"`
	Gateway    string     `json:"gateway"`
	MTU        int        `json:"mtu"`
	IPAM       IPAMConfig `json:"ipam"`

	SubnetNet    *net.IPNet `json:"-"`
	GatewayIP    net.IP     `json:"-"`
	RangeStartIP net.IP     `json:"-"`
	RangeEndIP   net.IP     `json:"-"`
}

// Parse loads, defaults, and validates the CNI plugin config.
func Parse(stdin []byte) (*NetworkConfig, error) {
	cfg := &NetworkConfig{}
	if err := json.Unmarshal(stdin, cfg); err != nil {
		return nil, fmt.Errorf("parse config json: %w", err)
	}

	if cfg.Bridge == "" {
		return nil, errors.New("bridge is required")
	}
	if cfg.Name == "" {
		return nil, errors.New("name is required")
	}
	if cfg.Subnet == "" {
		return nil, errors.New("subnet is required")
	}
	if cfg.Gateway == "" {
		return nil, errors.New("gateway is required")
	}
	if cfg.MTU == 0 {
		cfg.MTU = DefaultMTU
	}
	if cfg.IPAM.DataDir == "" {
		cfg.IPAM.DataDir = DefaultDataDir
	}

	gatewayIP, err := parseIPv4(cfg.Gateway)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	cfg.GatewayIP = gatewayIP

	_, subnetNet, err := net.ParseCIDR(cfg.Subnet)
	if err != nil {
		return nil, fmt.Errorf("subnet: invalid CIDR: %w", err)
	}
	if subnetNet.IP.To4() == nil {
		return nil, errors.New("subnet: only IPv4 is supported")
	}
	cfg.SubnetNet = subnetNet

	if !subnetNet.Contains(gatewayIP) {
		return nil, errors.New("gateway must be inside subnet")
	}

	networkIP, broadcastIP, err := networkAndBroadcast(subnetNet)
	if err != nil {
		return nil, err
	}
	if gatewayIP.Equal(networkIP) || gatewayIP.Equal(broadcastIP) {
		return nil, errors.New("gateway cannot be network or broadcast address")
	}

	if cfg.IPAM.RangeStart != "" {
		cfg.RangeStartIP, err = parseIPv4(cfg.IPAM.RangeStart)
		if err != nil {
			return nil, fmt.Errorf("ipam.rangeStart: %w", err)
		}
	}
	if cfg.IPAM.RangeEnd != "" {
		cfg.RangeEndIP, err = parseIPv4(cfg.IPAM.RangeEnd)
		if err != nil {
			return nil, fmt.Errorf("ipam.rangeEnd: %w", err)
		}
	}

	if (cfg.IPAM.RangeStart == "") != (cfg.IPAM.RangeEnd == "") {
		return nil, errors.New("ipam.rangeStart and ipam.rangeEnd must be set together")
	}

	if cfg.RangeStartIP == nil && cfg.RangeEndIP == nil {
		cfg.RangeStartIP, cfg.RangeEndIP, err = defaultRange(subnetNet)
		if err != nil {
			return nil, err
		}
	}

	if !subnetNet.Contains(cfg.RangeStartIP) || !subnetNet.Contains(cfg.RangeEndIP) {
		return nil, errors.New("ipam range must be inside subnet")
	}
	if ipv4ToUint(cfg.RangeStartIP) > ipv4ToUint(cfg.RangeEndIP) {
		return nil, errors.New("ipam rangeStart must be <= rangeEnd")
	}
	if cfg.RangeStartIP.Equal(networkIP) || cfg.RangeStartIP.Equal(broadcastIP) {
		return nil, errors.New("ipam rangeStart cannot be network or broadcast")
	}
	if cfg.RangeEndIP.Equal(networkIP) || cfg.RangeEndIP.Equal(broadcastIP) {
		return nil, errors.New("ipam rangeEnd cannot be network or broadcast")
	}

	return cfg, nil
}

func parseIPv4(value string) (net.IP, error) {
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, errors.New("invalid IP address")
	}
	ip = ip.To4()
	if ip == nil {
		return nil, errors.New("only IPv4 is supported")
	}
	return ip, nil
}

func defaultRange(subnet *net.IPNet) (net.IP, net.IP, error) {
	ones, bits := subnet.Mask.Size()
	if bits-ones < 2 {
		return nil, nil, errors.New("subnet does not provide usable host addresses")
	}

	networkIP, broadcastIP, err := networkAndBroadcast(subnet)
	if err != nil {
		return nil, nil, err
	}
	start := uintToIPv4(ipv4ToUint(networkIP) + 1)
	end := uintToIPv4(ipv4ToUint(broadcastIP) - 1)
	if ipv4ToUint(start) > ipv4ToUint(end) {
		return nil, nil, errors.New("subnet does not provide usable host addresses")
	}
	return start, end, nil
}

func networkAndBroadcast(subnet *net.IPNet) (net.IP, net.IP, error) {
	networkIP := subnet.IP.Mask(subnet.Mask).To4()
	if networkIP == nil {
		return nil, nil, errors.New("only IPv4 subnet is supported")
	}
	mask := net.IP(subnet.Mask).To4()
	if mask == nil {
		return nil, nil, errors.New("invalid subnet mask")
	}
	broadcast := make(net.IP, len(networkIP))
	for i := range networkIP {
		broadcast[i] = networkIP[i] | ^mask[i]
	}
	return networkIP, broadcast, nil
}

func ipv4ToUint(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uintToIPv4(value uint32) net.IP {
	return net.IPv4(
		byte(value>>24),
		byte(value>>16),
		byte(value>>8),
		byte(value),
	).To4()
}
