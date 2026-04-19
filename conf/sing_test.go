package conf

import (
	"encoding/json"
	"testing"
)

func TestSingOptionsAcceptLegacyFieldNames(t *testing.T) {
	options := NewSingOptions()
	if err := json.Unmarshal([]byte(`{"TCPFastOpen":true,"SniffEnabled":false}`), options); err != nil {
		t.Fatalf("unmarshal sing options failed: %v", err)
	}
	if !options.TCPFastOpen {
		t.Fatal("expected legacy TCPFastOpen to enable TFO")
	}
	if options.SniffEnabled {
		t.Fatal("expected legacy SniffEnabled=false to disable sniffing")
	}
}

func TestSingOptionsPreferCanonicalFieldNames(t *testing.T) {
	options := NewSingOptions()
	if err := json.Unmarshal([]byte(`{"EnableTFO":false,"TCPFastOpen":true,"EnableSniff":false,"SniffEnabled":true}`), options); err != nil {
		t.Fatalf("unmarshal sing options failed: %v", err)
	}
	if options.TCPFastOpen {
		t.Fatal("expected canonical EnableTFO=false to take precedence")
	}
	if options.SniffEnabled {
		t.Fatal("expected canonical EnableSniff=false to take precedence")
	}
}

func TestSingLogConfigNormalizedDisablesNoneLevel(t *testing.T) {
	cfg := SingLogConfig{
		Level:      "none",
		Output:     "none",
		AccessPath: "/tmp/access.log",
		ErrorPath:  "/tmp/error.log",
	}

	normalized := cfg.Normalized()
	if !normalized.Disabled {
		t.Fatal("expected none level to disable sing log")
	}
	if normalized.Level != "error" {
		t.Fatalf("expected fallback level error, got %q", normalized.Level)
	}
	if normalized.Output != "" {
		t.Fatalf("expected disabled log output to be empty, got %q", normalized.Output)
	}
}

func TestSingLogConfigNormalizedUsesLegacyPath(t *testing.T) {
	cfg := SingLogConfig{
		Level:     "info",
		ErrorPath: "/tmp/sing-error.log",
	}

	normalized := cfg.Normalized()
	if normalized.Disabled {
		t.Fatal("expected info level log to stay enabled")
	}
	if normalized.Output != "/tmp/sing-error.log" {
		t.Fatalf("expected legacy error path to become output, got %q", normalized.Output)
	}
}

func TestSingLogConfigNormalizedMapsWarning(t *testing.T) {
	cfg := SingLogConfig{Level: "warning"}

	normalized := cfg.Normalized()
	if normalized.Level != "warn" {
		t.Fatalf("expected warning to normalize to warn, got %q", normalized.Level)
	}
}
