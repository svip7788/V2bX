package conf

import (
	"testing"
)

func TestConf_LoadFromPath(t *testing.T) {
	c := New()
	if err := c.LoadFromPath("../example/config.json"); err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if len(c.NodeConfig) == 0 {
		t.Fatal("node config should not be empty")
	}
}

func TestConf_Watch_ReturnsErrorForMissingFile(t *testing.T) {
	c := New()
	if err := c.Watch("./not-exist.json", "", "", func() {}); err == nil {
		t.Fatal("expected watch error for missing file")
	}
}
