package snmp

import (
	"fmt"
	"time"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/gosnmp/gosnmp"
)

// Client wraps gosnmp.GoSNMP to provide a simplified interface for the application.
type Client struct {
	target  string
	creds   domain.SNMPCredentials
	timeout time.Duration
	retries int
	snmp    *gosnmp.GoSNMP
}

// NewClient creates a new SNMP client configured with the given credentials.
func NewClient(target string, creds domain.SNMPCredentials, timeout time.Duration, retries int) (*Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	gs := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Timeout:   timeout,
		Retries:   retries,
		MaxOids:   gosnmp.MaxOids,
	}

	if creds.Version == domain.SNMPVersionV2c {
		if creds.V2c == nil {
			return nil, fmt.Errorf("SNMPv2c configured but credentials are nil")
		}
		gs.Version = gosnmp.Version2c
		gs.Community = creds.V2c.Community
	} else if creds.Version == domain.SNMPVersionV3 {
		if creds.V3 == nil {
			return nil, fmt.Errorf("SNMPv3 configured but credentials are nil")
		}
		gs.Version = gosnmp.Version3
		gs.SecurityModel = gosnmp.UserSecurityModel

		msgFlags := gosnmp.NoAuthNoPriv
		if creds.V3.SecurityLevel == "authNoPriv" {
			msgFlags = gosnmp.AuthNoPriv
		} else if creds.V3.SecurityLevel == "authPriv" {
			msgFlags = gosnmp.AuthPriv
		}
		gs.MsgFlags = msgFlags

		user := &gosnmp.UsmSecurityParameters{
			UserName: creds.V3.Username,
		}

		if gs.MsgFlags&gosnmp.AuthNoPriv == gosnmp.AuthNoPriv {
			switch creds.V3.AuthProtocol {
			case "MD5":
				user.AuthenticationProtocol = gosnmp.MD5
			case "SHA":
				user.AuthenticationProtocol = gosnmp.SHA
			case "SHA224":
				user.AuthenticationProtocol = gosnmp.SHA224
			case "SHA256":
				user.AuthenticationProtocol = gosnmp.SHA256
			case "SHA384":
				user.AuthenticationProtocol = gosnmp.SHA384
			case "SHA512":
				user.AuthenticationProtocol = gosnmp.SHA512
			default:
				return nil, fmt.Errorf("unsupported auth protocol: %s", creds.V3.AuthProtocol)
			}
			user.AuthenticationPassphrase = creds.V3.AuthPassword
		}

		if gs.MsgFlags&gosnmp.AuthPriv == gosnmp.AuthPriv {
			switch creds.V3.PrivProtocol {
			case "DES":
				user.PrivacyProtocol = gosnmp.DES
			case "AES":
				user.PrivacyProtocol = gosnmp.AES
			case "AES192":
				user.PrivacyProtocol = gosnmp.AES192
			case "AES256":
				user.PrivacyProtocol = gosnmp.AES256
			case "AES192C":
				user.PrivacyProtocol = gosnmp.AES192C
			case "AES256C":
				user.PrivacyProtocol = gosnmp.AES256C
			default:
				return nil, fmt.Errorf("unsupported priv protocol: %s", creds.V3.PrivProtocol)
			}
			user.PrivacyPassphrase = creds.V3.PrivPassword
		}

		gs.SecurityParameters = user
	} else {
		return nil, fmt.Errorf("unsupported SNMP version: %v", creds.Version)
	}

	return &Client{
		target:  target,
		creds:   creds,
		timeout: timeout,
		retries: retries,
		snmp:    gs,
	}, nil
}

// Connect opens the connection to the SNMP agent.
func (c *Client) Connect() error {
	return c.snmp.Connect()
}

// Close closes the connection to the SNMP agent.
func (c *Client) Close() error {
	return c.snmp.Conn.Close()
}

// Get performs an SNMP GET request for the specified OIDs.
func (c *Client) Get(oids []string) ([]gosnmp.SnmpPDU, error) {
	result, err := c.snmp.Get(oids)
	if err != nil {
		return nil, err
	}
	return result.Variables, nil
}

// BulkWalk performs an SNMP BULKWALK request (or WALK for v1/v2c fallback) for the specified root OID.
func (c *Client) BulkWalk(rootOid string) ([]gosnmp.SnmpPDU, error) {
	var variables []gosnmp.SnmpPDU
	err := c.snmp.BulkWalk(rootOid, func(pdu gosnmp.SnmpPDU) error {
		variables = append(variables, pdu)
		return nil
	})
	if err != nil {
		// Fallback to normal walk
		variables = []gosnmp.SnmpPDU{}
		err = c.snmp.Walk(rootOid, func(pdu gosnmp.SnmpPDU) error {
			variables = append(variables, pdu)
			return nil
		})
	}
	return variables, err
}
