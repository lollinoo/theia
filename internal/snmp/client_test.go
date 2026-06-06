package snmp

// This file exercises client behavior so refactors preserve the documented contract.

import (
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/lollinoo/theia/internal/domain"
)

func TestNewClient_V2c(t *testing.T) {
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV2c,
		V2c: &domain.SNMPv2cCredentials{
			Community: "public",
		},
	}

	c, err := NewClient("10.0.0.1", creds, 2*time.Second, 1)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if c.snmp.Target != "10.0.0.1" {
		t.Errorf("expected target 10.0.0.1, got %s", c.snmp.Target)
	}
	if c.snmp.Version != gosnmp.Version2c {
		t.Errorf("expected Version2c, got %v", c.snmp.Version)
	}
	if c.snmp.Community != "public" {
		t.Errorf("expected community 'public', got %s", c.snmp.Community)
	}
	if c.snmp.Timeout != 2*time.Second {
		t.Errorf("expected timeout 2s, got %v", c.snmp.Timeout)
	}
	if c.snmp.Retries != 1 {
		t.Errorf("expected retries 1, got %d", c.snmp.Retries)
	}
}

func TestNewClient_V3(t *testing.T) {
	creds := domain.SNMPCredentials{
		Version: domain.SNMPVersionV3,
		V3: &domain.SNMPv3Credentials{
			Username:      "admin",
			AuthProtocol:  "SHA",
			AuthPassword:  "auth1234",
			PrivProtocol:  "AES",
			PrivPassword:  "priv1234",
			SecurityLevel: "authPriv",
		},
	}

	c, err := NewClient("10.0.0.2", creds, 5*time.Second, 3)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if c.snmp.Version != gosnmp.Version3 {
		t.Errorf("expected Version3, got %v", c.snmp.Version)
	}

	if c.snmp.MsgFlags != gosnmp.AuthPriv {
		t.Errorf("expected AuthPriv flag, got %v", c.snmp.MsgFlags)
	}

	usm, ok := c.snmp.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		t.Fatalf("expected SecurityParameters to be UsmSecurityParameters")
	}

	if usm.UserName != "admin" {
		t.Errorf("expected user 'admin', got %s", usm.UserName)
	}
	if usm.AuthenticationProtocol != gosnmp.SHA {
		t.Errorf("expected SHA auth protocol")
	}
	if usm.AuthenticationPassphrase != "auth1234" {
		t.Errorf("expected auth passphrase 'auth1234'")
	}
	if usm.PrivacyProtocol != gosnmp.AES {
		t.Errorf("expected AES priv protocol")
	}
	if usm.PrivacyPassphrase != "priv1234" {
		t.Errorf("expected priv passphrase 'priv1234'")
	}
}

func TestNewClient_Invalid(t *testing.T) {
	// Missing V2c config
	_, err := NewClient("10.0.0.3", domain.SNMPCredentials{Version: domain.SNMPVersionV2c}, 0, 0)
	if err == nil {
		t.Error("expected error for missing v2c credentials")
	}

	// Missing V3 config
	_, err = NewClient("10.0.0.4", domain.SNMPCredentials{Version: domain.SNMPVersionV3}, 0, 0)
	if err == nil {
		t.Error("expected error for missing v3 credentials")
	}
}
