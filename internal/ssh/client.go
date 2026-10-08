package ssh

// This file defines client SSH connectivity and host-key trust behavior.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Dialer abstracts SSH connection establishment for testability.
type Dialer interface {
	Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error)
}

// DefaultDialer bounds both TCP connection establishment and the SSH handshake.
type DefaultDialer struct{}

// Dial connects to addr using the given SSH config.
func (d *DefaultDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	return d.DialContext(context.Background(), addr, config)
}

// DialContext applies the shorter of the caller's deadline and the dial timeout
// to TCP and SSH negotiation, then clears the transport deadline for later work.
func (d *DefaultDialer) DialContext(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			conn.Close()
			return nil, err
		}
	}
	sshConn, channels, requests, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		sshConn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		sshConn.Close()
		return nil, err
	}
	return ssh.NewClient(sshConn, channels, requests), nil
}

type contextDialer interface {
	DialContext(context.Context, string, *ssh.ClientConfig) (*ssh.Client, error)
}

func dialContext(ctx context.Context, dialer Dialer, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d, ok := dialer.(contextDialer); ok {
		return d.DialContext(ctx, addr, config)
	}
	// Legacy injected dialers retain their existing interface. The production
	// DefaultDialer implements cancellation during connection establishment.
	client, err := dialer.Dial(addr, config)
	if ctx.Err() != nil {
		if client != nil {
			client.Close()
		}
		return nil, ctx.Err()
	}
	return client, err
}

// Client wraps an SSH connection and provides command execution.
type Client struct {
	client *ssh.Client
}

// SSHClient returns the underlying crypto/ssh client for direct access.
// Used by backup service to create SFTP clients for stat polling.
func (c *Client) SSHClient() *ssh.Client {
	return c.client
}

// NewClient creates an SSH client using password authentication.
func NewClient(dialer Dialer, host string, port int, username, password string, timeout time.Duration, hostKeyCallback ssh.HostKeyCallback) (*Client, error) {
	return NewClientContext(context.Background(), dialer, host, port, username, password, timeout, hostKeyCallback)
}

// NewClientContext creates a password-authenticated connection within the caller's deadline.
func NewClientContext(ctx context.Context, dialer Dialer, host string, port int, username, password string, timeout time.Duration, hostKeyCallback ssh.HostKeyCallback) (*Client, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := dialContext(ctx, dialer, addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &Client{client: client}, nil
}

// NewClientWithKey creates an SSH client using private key authentication.
func NewClientWithKey(dialer Dialer, host string, port int, username string, privateKey []byte, timeout time.Duration, hostKeyCallback ssh.HostKeyCallback) (*Client, error) {
	return NewClientWithKeyContext(context.Background(), dialer, host, port, username, privateKey, timeout, hostKeyCallback)
}

// NewClientWithKeyContext creates a key-authenticated connection within the caller's deadline.
func NewClientWithKeyContext(ctx context.Context, dialer Dialer, host string, port int, username string, privateKey []byte, timeout time.Duration, hostKeyCallback ssh.HostKeyCallback) (*Client, error) {
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := dialContext(ctx, dialer, addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &Client{client: client}, nil
}

// RunCommand executes a command on the remote host and returns stdout.
func (c *Client) RunCommand(ctx context.Context, command string) (string, error) {
	var stdout bytes.Buffer
	if err := c.RunCommandToWriter(ctx, command, &stdout); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// RunCommandToWriter executes a command and streams stdout into the provided writer.
func (c *Client) RunCommandToWriter(ctx context.Context, command string, stdout io.Writer) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	if stdout == nil {
		stdout = io.Discard
	}
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	var stderr bytes.Buffer
	session.Stdout = stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		return fmt.Errorf("command %q: %w (stderr: %s)", command, err, stderr.String())
	}
	return nil
}

// DownloadFileToDisk streams an SFTP file to a temporary file and renames it only
// after a complete transfer. Cancellation closes the transport and drains I/O.
func (c *Client) DownloadFileToDisk(ctx context.Context, remotePath, localPath string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("opening remote file %q: %w", remotePath, err)
	}
	defer remoteFile.Close()
	tmpFile, err := os.CreateTemp(filepath.Dir(localPath), ".theia-download-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	if _, err := io.Copy(tmpFile, remoteFile); err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpFile.Name(), localPath); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// Close closes the underlying SSH connection.
func (c *Client) Close() error {
	return c.client.Close()
}
