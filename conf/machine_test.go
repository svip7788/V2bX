package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineConfigLoad(t *testing.T) {
	c := New()
	data := `{"Log":{"Level":"error"},"Machine":{"ApiHost":"https://p.example","MachineID":3,"Token":"tk","Core":"auto","NodeDefaults":{"ListenIP":"1.2.3.4","CertConfig":{"CertMode":"file"}}}}`
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadFromPath(p); err != nil {
		t.Fatal(err)
	}
	if !c.MachineConfig.Enabled() || c.MachineConfig.MachineID != 3 {
		t.Fatalf("machine config not loaded: %+v", c.MachineConfig)
	}

	nc, err := c.MachineConfig.NodeConfig(7, "vless", "xray")
	if err != nil {
		t.Fatal(err)
	}
	if nc.ApiConfig.NodeID != 7 || nc.ApiConfig.Key != "tk" || nc.ApiConfig.APIHost != "https://p.example" {
		t.Fatalf("bad api config: %+v", nc.ApiConfig)
	}
	if nc.Options.Core != "xray" || nc.Options.XrayOptions == nil {
		t.Fatalf("xray options missing: %+v", nc.Options)
	}
	if nc.Options.ListenIP != "1.2.3.4" {
		t.Fatalf("NodeDefaults not applied: %s", nc.Options.ListenIP)
	}
	if nc.Options.CertConfig.CertMode != "file" || nc.Options.CertConfig.CertFile != "/etc/V2bX/cert/node7.crt" {
		t.Fatalf("CertConfig not merged: %+v", nc.Options.CertConfig)
	}
}

func TestMachineCoreSelection(t *testing.T) {
	both := []string{"xray", "sing"}
	cases := []struct {
		mode, nodeType string
		available      []string
		want           string
	}{
		{"auto", "vless", both, "xray"},
		{"auto", "hysteria2", both, "sing"},
		{"auto", "tuic", both, "sing"},
		{"auto", "anytls", both, "xray"},
		{"auto", "anytls", []string{"sing"}, "sing"},
		{"sing", "vmess", both, "sing"},
		{"xray", "trojan", both, "xray"},
		{"auto", "vless", []string{"sing"}, "sing"},
	}
	for _, tc := range cases {
		m := &MachineConfig{Core: tc.mode}
		if got := m.CoreTypeFor(tc.nodeType, tc.available); got != tc.want {
			t.Errorf("%s/%s: got %s want %s", tc.mode, tc.nodeType, got, tc.want)
		}
	}
}

func TestMachineSingNodeOptions(t *testing.T) {
	m := &MachineConfig{APIHost: "http://p/", MachineID: 1, Token: "t"}
	nc, err := m.NodeConfig(5, "hysteria2", "sing")
	if err != nil {
		t.Fatal(err)
	}
	if nc.ApiConfig.APIHost != "http://p" {
		t.Fatalf("trailing slash not trimmed: %s", nc.ApiConfig.APIHost)
	}
	if nc.Options.SingOptions == nil || nc.Options.ListenIP != "::" || nc.Options.CertConfig.CertMode != "self" {
		t.Fatalf("bad sing options: %+v %+v", nc.Options, nc.Options.CertConfig)
	}
}

func TestMachineXrayAnyTLSNodeOptions(t *testing.T) {
	m := &MachineConfig{APIHost: "http://p", MachineID: 1, Token: "t"}
	nc, err := m.NodeConfig(6, "anytls", "xray")
	if err != nil {
		t.Fatal(err)
	}
	if nc.Options.XrayOptions == nil || nc.Options.ListenIP != "0.0.0.0" || nc.Options.CertConfig.CertMode != "self" {
		t.Fatalf("bad xray anytls options: %+v %+v", nc.Options, nc.Options.CertConfig)
	}
}

func TestNormalizeMachineNodeType(t *testing.T) {
	if NormalizeMachineNodeType("V2ray") != "vmess" || NormalizeMachineNodeType("socks") != "" {
		t.Fatal("normalize failed")
	}
}
