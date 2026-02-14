package atomicni

import (
	"crypto/sha1"
	"encoding/hex"
)

const linuxIfNameMaxLen = 15

// HostVethName returns deterministic host-side veth name for a container ID.
func HostVethName(containerID string) string {
	return deterministicName("av", containerID)
}

// PeerVethTempName returns deterministic temporary peer veth name before netns rename.
func PeerVethTempName(containerID string) string {
	return deterministicName("cv", containerID)
}

func deterministicName(prefix, key string) string {
	hash := sha1.Sum([]byte(key))
	hexHash := hex.EncodeToString(hash[:])
	maxHashLen := linuxIfNameMaxLen - len(prefix)
	if maxHashLen < 1 {
		maxHashLen = 1
	}
	return prefix + hexHash[:maxHashLen]
}
