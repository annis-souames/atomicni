package result

import (
	"net"

	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
)

// BuildAddResult returns a CNI result for a successful ADD operation.
func BuildAddResult(
	cniVersion string,
	hostName string,
	hostMAC string,
	containerName string,
	containerMAC string,
	netnsPath string,
	address *net.IPNet,
	gateway net.IP,
) *current.Result {
	containerInterfaceIndex := 1
	return &current.Result{
		CNIVersion: cniVersion,
		Interfaces: []*current.Interface{
			{Name: hostName, Mac: hostMAC},
			{Name: containerName, Mac: containerMAC, Sandbox: netnsPath},
		},
		IPs: []*current.IPConfig{
			{
				Address:   *address,
				Gateway:   gateway,
				Interface: &containerInterfaceIndex,
			},
		},
		Routes: []*types.Route{
			{
				Dst: net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
				GW:  gateway,
			},
		},
	}
}
