package cmd

import (
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestRun(t *testing.T) {
	Run()
}

func TestApplyServerRuntimeOverridesSetsPprofListen(t *testing.T) {
	old := pprofListen
	pprofListen = "127.0.0.1:6060"
	t.Cleanup(func() {
		pprofListen = old
	})

	c := conf.New()
	applyServerRuntimeOverrides(c)
	if c.LogConfig.PprofListen != "127.0.0.1:6060" {
		t.Fatalf("expected pprof listen to be overridden, got %q", c.LogConfig.PprofListen)
	}
}

func TestApplyServerRuntimeOverridesLeavesConfigUntouchedWithoutFlag(t *testing.T) {
	old := pprofListen
	pprofListen = ""
	t.Cleanup(func() {
		pprofListen = old
	})

	c := conf.New()
	c.LogConfig.PprofListen = "127.0.0.1:7070"
	applyServerRuntimeOverrides(c)
	if c.LogConfig.PprofListen != "127.0.0.1:7070" {
		t.Fatalf("expected pprof listen to stay unchanged, got %q", c.LogConfig.PprofListen)
	}
}
