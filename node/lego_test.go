package node

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestLego_parseParams(t *testing.T) {
	l := &Lego{
		config: &conf.CertConfig{
			CertDomain: "example.com",
			Email:      "test@example.com",
		},
	}
	if got, want := l.parseParams("./cert/{domain}/{email}.pem"), "./cert/example.com/test@example.com.pem"; got != want {
		t.Fatalf("parseParams mismatch: got %q, want %q", got, want)
	}
}

func TestLego_CreateCertByDns(t *testing.T) {
	if os.Getenv("V2BX_RUN_ACME_TESTS") != "1" {
		t.Skip("skip ACME integration test, set V2BX_RUN_ACME_TESTS=1 to enable")
	}
	token := os.Getenv("CF_DNS_API_TOKEN")
	domain := os.Getenv("V2BX_ACME_DOMAIN")
	if token == "" || domain == "" {
		t.Skip("ACME env missing: need CF_DNS_API_TOKEN and V2BX_ACME_DOMAIN")
	}
	email := os.Getenv("V2BX_ACME_EMAIL")
	if email == "" {
		email = "test@example.com"
	}

	tmpDir := t.TempDir()
	l, err := NewLego(&conf.CertConfig{
		CertMode:   "dns",
		Email:      email,
		CertDomain: domain,
		Provider:   "cloudflare",
		DNSEnv: map[string]string{
			"CF_DNS_API_TOKEN": token,
		},
		CertFile: filepath.Join(tmpDir, "cert.pem"),
		KeyFile:  filepath.Join(tmpDir, "key.pem"),
	})
	if err != nil {
		t.Fatalf("new lego failed: %v", err)
	}
	if err := l.CreateCert(); err != nil {
		t.Fatalf("create cert failed: %v", err)
	}
}

func TestLego_RenewCert(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	if err := generateSelfSslCertificate("domain.com", certPath, keyPath); err != nil {
		t.Fatalf("generate self cert failed: %v", err)
	}
	l := &Lego{
		config: &conf.CertConfig{
			CertDomain: "domain.com",
			CertFile:   certPath,
			KeyFile:    keyPath,
		},
	}
	if err := l.RenewCert(); err != nil {
		t.Fatalf("renew cert failed: %v", err)
	}
}
