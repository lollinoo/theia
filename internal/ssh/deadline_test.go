package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	gossh "golang.org/x/crypto/ssh"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDefaultDialerBoundsSilentHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	done := make(chan error, 1)
	go func() {
		_, err := (&DefaultDialer{}).Dial(listener.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 80 * time.Millisecond})
		done <- err
	}()
	conn := <-accepted
	defer conn.Close()
	select {
	case err := <-done:
		var timeout net.Error
		if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &timeout) || !timeout.Timeout()) {
			t.Fatalf("err=%v, want timeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSH handshake exceeded dial timeout")
	}
}

func TestDefaultDialerCancelsSilentHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (&DefaultDialer{}).DialContext(ctx, listener.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Minute})
		done <- err
	}()
	conn := <-accepted
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handshake ignored cancellation")
	}
}

func TestClientCancelsBlockedChannelOperations(t *testing.T) {
	for _, operation := range []string{"command", "sftp-memory", "sftp-disk"} {
		t.Run(operation, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := gossh.NewSignerFromKey(key)
			if err != nil {
				t.Fatal(err)
			}
			config := &gossh.ServerConfig{NoClientAuth: true}
			config.AddHostKey(signer)
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				sshConn, channels, requests, err := gossh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer sshConn.Close()
				go gossh.DiscardRequests(requests)
				for range channels { /* intentionally never accept channel opens */
				}
			}()
			host, portString, _ := net.SplitHostPort(listener.Addr().String())
			port, _ := strconv.Atoi(portString)
			client, err := NewClient(&DefaultDialer{}, host, port, "test", "test", time.Second, gossh.InsecureIgnoreHostKey())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			path := filepath.Join(t.TempDir(), "backup")
			done := make(chan error, 1)
			go func() {
				switch operation {
				case "command":
					_, err = client.RunCommand(ctx, "blocked")
				case "sftp-memory":
					_, err = client.DownloadFile(ctx, "/backup")
				case "sftp-disk":
					err = client.DownloadFileToDisk(ctx, "/backup", path)
				}
				done <- err
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("err=%v", err)
				}
			case <-time.After(2 * time.Second):
				client.Close()
				t.Fatal("operation ignored deadline")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("cancelled transfer published file: %v", err)
			}
			select {
			case <-serverDone:
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled transport remained open")
			}
		})
	}
}
