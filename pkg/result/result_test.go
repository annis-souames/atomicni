package result

import (
	"net"
	"testing"
)

func TestBuildAddResult(t *testing.T) {
	addr := &net.IPNet{IP: net.ParseIP("10.22.0.10").To4(), Mask: net.CIDRMask(24, 32)}
	gw := net.ParseIP("10.22.0.1").To4()

	res := BuildAddResult(
		"1.1.0",
		"av123",
		"aa:bb:cc:dd:ee:ff",
		"eth0",
		"11:22:33:44:55:66",
		"/var/run/netns/test",
		addr,
		gw,
	)

	if len(res.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(res.Interfaces))
	}
	if res.Interfaces[0].Name != "av123" {
		t.Fatalf("unexpected host interface name: %s", res.Interfaces[0].Name)
	}
	if res.Interfaces[1].Name != "eth0" {
		t.Fatalf("unexpected container interface name: %s", res.Interfaces[1].Name)
	}
	if len(res.IPs) != 1 {
		t.Fatalf("expected 1 IP config, got %d", len(res.IPs))
	}
	if res.IPs[0].Interface == nil || *res.IPs[0].Interface != 1 {
		t.Fatalf("expected IP config interface index 1")
	}
	if res.IPs[0].Gateway.String() != "10.22.0.1" {
		t.Fatalf("unexpected gateway: %s", res.IPs[0].Gateway)
	}
	if len(res.Routes) != 1 || res.Routes[0].Dst.String() != "0.0.0.0/0" {
		t.Fatalf("expected default route in result")
	}
}
