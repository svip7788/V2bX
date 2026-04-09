package node

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_generateSelfSslCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "test-cert.pem")
	keyPath := filepath.Join(tmpDir, "test-key.pem")
	if err := generateSelfSslCertificate("domain.com", certPath, keyPath); err != nil {
		t.Fatalf("generate self cert failed: %v", err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert file not created: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("key file not created: %v", err)
	}
}
