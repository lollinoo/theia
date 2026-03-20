package ssh

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

// DefaultDialer connects via TCP using the standard ssh.Dial.
type DefaultDialer struct{}

// Dial connects to addr using the given SSH config.
func (d *DefaultDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	return ssh.Dial("tcp", addr, config)
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
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	client, err := dialer.Dial(addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &Client{client: client}, nil
}

// NewClientWithKey creates an SSH client using private key authentication.
func NewClientWithKey(dialer Dialer, host string, port int, username string, privateKey []byte, timeout time.Duration, hostKeyCallback ssh.HostKeyCallback) (*Client, error) {
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
	client, err := dialer.Dial(addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &Client{client: client}, nil
}

// RunCommand executes a command on the remote host and returns stdout.
func (c *Client) RunCommand(ctx context.Context, command string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		return "", ctx.Err()
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("command %q: %w (stderr: %s)", command, err, stderr.String())
		}
		return stdout.String(), nil
	}
}

// DownloadFile retrieves a file from the remote host via SFTP.
func (c *Client) DownloadFile(ctx context.Context, remotePath string) ([]byte, error) {
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return nil, fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()

	type result struct {
		data []byte
		err  error
	}

	done := make(chan result, 1)
	go func() {
		f, err := sftpClient.Open(remotePath)
		if err != nil {
			done <- result{nil, fmt.Errorf("opening remote file %q: %w", remotePath, err)}
			return
		}
		defer f.Close()

		data, err := io.ReadAll(f)
		if err != nil {
			done <- result{nil, fmt.Errorf("reading remote file %q: %w", remotePath, err)}
			return
		}
		done <- result{data, nil}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.data, r.err
	}
}

// DownloadFileToDisk streams an SFTP file directly to a local file path.
// Uses a temp file + rename for atomic writes.
func (c *Client) DownloadFileToDisk(ctx context.Context, remotePath, localPath string) error {
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		return fmt.Errorf("creating SFTP client: %w", err)
	}
	defer sftpClient.Close()

	type result struct {
		err error
	}

	done := make(chan result, 1)
	go func() {
		remoteFile, err := sftpClient.Open(remotePath)
		if err != nil {
			done <- result{fmt.Errorf("opening remote file %q: %w", remotePath, err)}
			return
		}
		defer remoteFile.Close()

		dir := filepath.Dir(localPath)
		tmpFile, err := os.CreateTemp(dir, ".theia-download-*")
		if err != nil {
			done <- result{fmt.Errorf("creating temp file: %w", err)}
			return
		}
		tmpPath := tmpFile.Name()

		if _, err := io.Copy(tmpFile, remoteFile); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			done <- result{fmt.Errorf("downloading file: %w", err)}
			return
		}
		if err := tmpFile.Close(); err != nil {
			os.Remove(tmpPath)
			done <- result{fmt.Errorf("closing temp file: %w", err)}
			return
		}

		if err := os.Rename(tmpPath, localPath); err != nil {
			os.Remove(tmpPath)
			done <- result{fmt.Errorf("renaming temp file: %w", err)}
			return
		}
		done <- result{nil}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case r := <-done:
		return r.err
	}
}

// Close closes the underlying SSH connection.
func (c *Client) Close() error {
	return c.client.Close()
}
