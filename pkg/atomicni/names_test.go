package atomicni

import "testing"

func TestDeterministicNames(t *testing.T) {
	containerID := "1234567890abcdef1234567890abcdef"
	host1 := HostVethName(containerID)
	host2 := HostVethName(containerID)
	peer := PeerVethTempName(containerID)

	if host1 != host2 {
		t.Fatalf("HostVethName should be deterministic: %q != %q", host1, host2)
	}
	if len(host1) > linuxIfNameMaxLen {
		t.Fatalf("host name too long: %d", len(host1))
	}
	if len(peer) > linuxIfNameMaxLen {
		t.Fatalf("peer name too long: %d", len(peer))
	}
	if host1 == peer {
		t.Fatalf("host and peer names should use different prefixes")
	}
}
