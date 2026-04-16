package xray

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestResolveOptionalConfigPathPrefersExplicitPath(t *testing.T) {
	cfg := &conf.XrayConfig{
		AssetPath: "/tmp/assets",
	}
	cfg.DnsConfigPath = "/tmp/custom.json"

	got, auto := cfg.ResolveDNSConfigPath()
	if got != "/tmp/custom.json" {
		t.Fatalf("expected explicit path to win, got %q", got)
	}
	if auto {
		t.Fatal("explicit path should not be marked as auto discovered")
	}
}

func TestResolveOptionalConfigPathAutoDiscoversBundledFile(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "dns.json")
	if err := os.WriteFile(candidate, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write dns config failed: %v", err)
	}

	cfg := &conf.XrayConfig{
		AssetPath: dir,
	}
	got, auto := cfg.ResolveDNSConfigPath()
	if got != candidate {
		t.Fatalf("expected %q, got %q", candidate, got)
	}
	if !auto {
		t.Fatal("expected auto discovered path")
	}
}

func TestResolveOptionalConfigPathReturnsEmptyWhenMissing(t *testing.T) {
	cfg := &conf.XrayConfig{
		AssetPath: t.TempDir(),
	}
	got, auto := cfg.ResolveDNSConfigPath()
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
	if auto {
		t.Fatal("missing file should not be marked as auto discovered")
	}
}

func TestDefaultDNSServersPreferLocalResolver(t *testing.T) {
	servers := defaultDNSServers()
	if len(servers) == 0 {
		t.Fatal("expected built-in dns servers")
	}
	if servers[0] == nil {
		t.Fatal("first dns server should not be nil")
	}
}
