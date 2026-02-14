package config

import (
	"strings"
	"testing"
)

func TestParseValidConfigDefaults(t *testing.T) {
	stdin := []byte(`{
		"cniVersion":"1.1.0",
		"name":"atomic-net",
		"type":"atomicni",
		"bridge":"atomic0",
		"subnet":"10.22.0.0/24",
		"gateway":"10.22.0.1"
	}`)

	cfg, err := Parse(stdin)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.MTU != DefaultMTU {
		t.Fatalf("expected default MTU %d, got %d", DefaultMTU, cfg.MTU)
	}
	if cfg.IPAM.DataDir != DefaultDataDir {
		t.Fatalf("expected default data dir %q, got %q", DefaultDataDir, cfg.IPAM.DataDir)
	}
	if cfg.RangeStartIP.String() != "10.22.0.1" {
		t.Fatalf("expected default rangeStart 10.22.0.1, got %s", cfg.RangeStartIP)
	}
	if cfg.RangeEndIP.String() != "10.22.0.254" {
		t.Fatalf("expected default rangeEnd 10.22.0.254, got %s", cfg.RangeEndIP)
	}
}

func TestParseRejectsGatewayOutsideSubnet(t *testing.T) {
	stdin := []byte(`{
		"cniVersion":"1.1.0",
		"name":"atomic-net",
		"type":"atomicni",
		"bridge":"atomic0",
		"subnet":"10.22.0.0/24",
		"gateway":"10.23.0.1"
	}`)

	_, err := Parse(stdin)
	if err == nil {
		t.Fatalf("expected Parse() to fail")
	}
	if !strings.Contains(err.Error(), "gateway must be inside subnet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsInvalidRangeOrder(t *testing.T) {
	stdin := []byte(`{
		"cniVersion":"1.1.0",
		"name":"atomic-net",
		"type":"atomicni",
		"bridge":"atomic0",
		"subnet":"10.22.0.0/24",
		"gateway":"10.22.0.1",
		"ipam":{"rangeStart":"10.22.0.50","rangeEnd":"10.22.0.10"}
	}`)

	_, err := Parse(stdin)
	if err == nil {
		t.Fatalf("expected Parse() to fail")
	}
	if !strings.Contains(err.Error(), "rangeStart must be <= rangeEnd") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsPartialRange(t *testing.T) {
	stdin := []byte(`{
		"cniVersion":"1.1.0",
		"name":"atomic-net",
		"type":"atomicni",
		"bridge":"atomic0",
		"subnet":"10.22.0.0/24",
		"gateway":"10.22.0.1",
		"ipam":{"rangeStart":"10.22.0.10"}
	}`)

	_, err := Parse(stdin)
	if err == nil {
		t.Fatalf("expected Parse() to fail")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("unexpected error: %v", err)
	}
}
