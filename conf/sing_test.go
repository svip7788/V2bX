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
