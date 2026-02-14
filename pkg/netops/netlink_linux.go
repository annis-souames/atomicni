package netops

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
)

// NetOps defines host/container link operations required by the plugin.
type NetOps interface {
	EnsureBridge(name string, gateway *net.IPNet) error
	CreateVethPair(hostName, peerName string, mtu int) error
	AttachHostVethToBridge(hostName, bridgeName string) error
	MoveToNamespace(linkName string, target ns.NetNS) error
	PrepareContainerLink(target ns.NetNS, currentName, targetName string) (string, error)
	AddAddressAndRoute(target ns.NetNS, ifName string, addr *net.IPNet, gateway net.IP) error
	DeleteLink(name string) error
	DeleteLinkInNS(target ns.NetNS, name string) error
	GetLinkMAC(name string) (string, error)
}

// NetlinkOps is a Linux implementation of NetOps backed by iproute2 commands.
type NetlinkOps struct{}

func NewNetlinkOps() *NetlinkOps {
	return &NetlinkOps{}
}

func (n *NetlinkOps) EnsureBridge(name string, gateway *net.IPNet) error {
	if !linkExists(name) {
		if _, err := runIP("link", "add", "name", name, "type", "bridge"); err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("create bridge: %w", err)
		}
	}
	if _, err := runIP("link", "set", "dev", name, "up"); err != nil {
		return fmt.Errorf("set bridge up: %w", err)
	}
	if gateway == nil {
		return nil
	}

	existing, err := runIP("addr", "show", "dev", name)
	if err != nil {
		return fmt.Errorf("read bridge addresses: %w", err)
	}
	if strings.Contains(existing, gateway.String()) {
		return nil
	}
	if _, err := runIP("addr", "add", gateway.String(), "dev", name); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("assign gateway to bridge: %w", err)
	}
	return nil
}

func (n *NetlinkOps) CreateVethPair(hostName, peerName string, mtu int) error {
	if hostName == "" || peerName == "" {
		return errors.New("host and peer names are required")
	}
	if mtu <= 0 {
		mtu = 1500
	}

	if linkExists(hostName) {
		return nil
	}
	if _, err := runIP("link", "add", hostName, "type", "veth", "peer", "name", peerName); err != nil {
		return fmt.Errorf("create veth pair: %w", err)
	}
	if _, err := runIP("link", "set", "dev", hostName, "mtu", fmt.Sprintf("%d", mtu)); err != nil {
		return fmt.Errorf("set host veth mtu: %w", err)
	}
	if _, err := runIP("link", "set", "dev", peerName, "mtu", fmt.Sprintf("%d", mtu)); err != nil {
		return fmt.Errorf("set peer veth mtu: %w", err)
	}
	return nil
}

func (n *NetlinkOps) AttachHostVethToBridge(hostName, bridgeName string) error {
	if _, err := runIP("link", "set", "dev", hostName, "master", bridgeName); err != nil {
		return fmt.Errorf("attach host veth to bridge: %w", err)
	}
	if _, err := runIP("link", "set", "dev", hostName, "up"); err != nil {
		return fmt.Errorf("set host veth up: %w", err)
	}
	return nil
}

func (n *NetlinkOps) MoveToNamespace(linkName string, target ns.NetNS) error {
	if !linkExists(linkName) {
		return nil
	}
	if _, err := runIP("link", "set", "dev", linkName, "netns", target.Path()); err != nil {
		return fmt.Errorf("move link %q to netns: %w", linkName, err)
	}
	return nil
}

func (n *NetlinkOps) PrepareContainerLink(target ns.NetNS, currentName, targetName string) (string, error) {
	var mac string
	if err := target.Do(func(_ ns.NetNS) error {
		if linkExists(currentName) {
			if _, err := runIP("link", "set", "dev", currentName, "name", targetName); err != nil {
				return fmt.Errorf("rename link to %q: %w", targetName, err)
			}
		}

		if !linkExists(targetName) {
			return fmt.Errorf("lookup link %q", targetName)
		}
		if _, err := runIP("link", "set", "dev", targetName, "up"); err != nil {
			return fmt.Errorf("set container link up: %w", err)
		}
		linkMAC, err := readMAC(targetName)
		if err != nil {
			return fmt.Errorf("read container link mac: %w", err)
		}
		mac = linkMAC
		return nil
	}); err != nil {
		return "", err
	}
	return mac, nil
}

func (n *NetlinkOps) AddAddressAndRoute(target ns.NetNS, ifName string, addr *net.IPNet, gateway net.IP) error {
	return target.Do(func(_ ns.NetNS) error {
		if _, err := runIP("addr", "add", addr.String(), "dev", ifName); err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("assign IP address: %w", err)
		}

		if _, err := runIP("route", "add", "default", "via", gateway.String(), "dev", ifName); err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("add default route: %w", err)
		}
		return nil
	})
}

func (n *NetlinkOps) DeleteLink(name string) error {
	if _, err := runIP("link", "del", "dev", name); err != nil {
		if isLinkNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete link %q: %w", name, err)
	}
	return nil
}

func (n *NetlinkOps) DeleteLinkInNS(target ns.NetNS, name string) error {
	return target.Do(func(_ ns.NetNS) error {
		if _, err := runIP("link", "del", "dev", name); err != nil {
			if isLinkNotFound(err) {
				return nil
			}
			return fmt.Errorf("delete link %q in netns: %w", name, err)
		}
		return nil
	})
}

func (n *NetlinkOps) GetLinkMAC(name string) (string, error) {
	return readMAC(name)
}

func runIP(args ...string) (string, error) {
	cmd := exec.Command("ip", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		if output == "" {
			output = err.Error()
		}
		return "", fmt.Errorf("%s (%s)", output, strings.Join(args, " "))
	}
	return output, nil
}

func linkExists(name string) bool {
	_, err := runIP("link", "show", "dev", name)
	return err == nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "File exists")
}

func isLinkNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrNotExist) ||
		strings.Contains(err.Error(), "Cannot find device") ||
		strings.Contains(err.Error(), "does not exist")
}

func readMAC(ifName string) (string, error) {
	content, err := os.ReadFile(filepath.Join("/sys/class/net", ifName, "address"))
	if err != nil {
		return "", fmt.Errorf("read MAC for %q: %w", ifName, err)
	}
	return strings.TrimSpace(string(content)), nil
}
