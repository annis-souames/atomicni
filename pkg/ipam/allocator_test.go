package ipam

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", cidr, err)
	}
	return n
}

func mustIP(t *testing.T, ip string) net.IP {
	t.Helper()
	parsed := net.ParseIP(ip).To4()
	if parsed == nil {
		t.Fatalf("invalid IPv4: %q", ip)
	}
	return parsed
}

func TestAllocateSequentialAndRelease(t *testing.T) {
	alloc := NewFileAllocator()
	dir := t.TempDir()
	req := AllocationRequest{
		DataDir:    dir,
		Network:    "atomic-net",
		Subnet:     mustCIDR(t, "10.22.0.0/29"),
		Gateway:    mustIP(t, "10.22.0.1"),
		RangeStart: mustIP(t, "10.22.0.2"),
		RangeEnd:   mustIP(t, "10.22.0.6"),
	}

	req.ContainerID = "c1"
	ip1, err := alloc.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("Allocate(c1): %v", err)
	}
	if ip1.String() != "10.22.0.2" {
		t.Fatalf("expected 10.22.0.2, got %s", ip1)
	}

	req.ContainerID = "c2"
	ip2, err := alloc.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("Allocate(c2): %v", err)
	}
	if ip2.String() != "10.22.0.3" {
		t.Fatalf("expected 10.22.0.3, got %s", ip2)
	}

	if err := alloc.Release(context.Background(), dir, "atomic-net", "c1"); err != nil {
		t.Fatalf("Release(c1): %v", err)
	}

	req.ContainerID = "c3"
	ip3, err := alloc.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("Allocate(c3): %v", err)
	}
	if ip3.String() != "10.22.0.4" {
		t.Fatalf("expected next-fit 10.22.0.4, got %s", ip3)
	}
}

func TestAllocateIdempotentPerContainer(t *testing.T) {
	alloc := NewFileAllocator()
	dir := t.TempDir()
	req := AllocationRequest{
		DataDir:     dir,
		Network:     "atomic-net",
		ContainerID: "same",
		Subnet:      mustCIDR(t, "10.22.0.0/24"),
		Gateway:     mustIP(t, "10.22.0.1"),
		RangeStart:  mustIP(t, "10.22.0.10"),
		RangeEnd:    mustIP(t, "10.22.0.20"),
	}

	ip1, err := alloc.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	ip2, err := alloc.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("second Allocate: %v", err)
	}
	if !ip1.Equal(ip2) {
		t.Fatalf("expected same IP for same container, got %s and %s", ip1, ip2)
	}
}

func TestAllocatePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	req := AllocationRequest{
		DataDir:     dir,
		Network:     "atomic-net",
		ContainerID: "c1",
		Subnet:      mustCIDR(t, "10.22.0.0/24"),
		Gateway:     mustIP(t, "10.22.0.1"),
		RangeStart:  mustIP(t, "10.22.0.10"),
		RangeEnd:    mustIP(t, "10.22.0.20"),
	}

	first := NewFileAllocator()
	ip1, err := first.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	second := NewFileAllocator()
	ip2, ok, err := second.GetByContainer(context.Background(), dir, "atomic-net", "c1")
	if err != nil {
		t.Fatalf("GetByContainer: %v", err)
	}
	if !ok {
		t.Fatalf("expected allocation to persist")
	}
	if !ip1.Equal(ip2) {
		t.Fatalf("expected persisted IP %s, got %s", ip1, ip2)
	}

	stateFile := filepath.Join(dir, "atomic-net.json")
	if _, err := loadState(stateFile); err != nil {
		t.Fatalf("state file should be readable: %v", err)
	}
}

func TestAllocateConcurrentUnique(t *testing.T) {
	alloc := NewFileAllocator()
	dir := t.TempDir()
	subnet := mustCIDR(t, "10.22.0.0/24")
	gw := mustIP(t, "10.22.0.1")
	rangeStart := mustIP(t, "10.22.0.10")
	rangeEnd := mustIP(t, "10.22.0.200")

	const n = 12
	results := make(chan string, n)
	errorsCh := make(chan error, n)
	wg := sync.WaitGroup{}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := AllocationRequest{
				DataDir:     dir,
				Network:     "atomic-net",
				ContainerID: fmt.Sprintf("c-%d", i),
				Subnet:      subnet,
				Gateway:     gw,
				RangeStart:  rangeStart,
				RangeEnd:    rangeEnd,
			}
			ip, err := alloc.Allocate(context.Background(), req)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- ip.String()
		}(i)
	}

	wg.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		t.Fatalf("Allocate concurrent error: %v", err)
	}

	seen := map[string]bool{}
	for ip := range results {
		if seen[ip] {
			t.Fatalf("duplicate IP allocated: %s", ip)
		}
		seen[ip] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d allocated IPs, got %d", n, len(seen))
	}
}
