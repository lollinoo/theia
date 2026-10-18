package instance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestS3DestinationVerifiesDownloadedBytesAndRejectsCorruption(t *testing.T) {
	var mu sync.Mutex
	var uploaded []byte
	corrupt := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") == "" {
			t.Error("S3 request not signed")
		}
		switch r.Method {
		case http.MethodPut:
			var err error
			uploaded, err = io.ReadAll(r.Body)
			if r.Header.Get("Content-Encoding") == "aws-chunked" {
				uploaded, err = io.ReadAll(httputil.NewChunkedReader(bytes.NewReader(uploaded)))
			}
			if err != nil {
				t.Error(err)
			}
			w.Header().Set("ETag", `"test-etag"`)
		case http.MethodGet:
			if r.URL.Query().Has("location") {
				_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
				return
			}
			data := append([]byte(nil), uploaded...)
			if corrupt && len(data) > 0 {
				data[len(data)-1] ^= 1
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Header().Set("Last-Modified", "Thu, 08 Oct 2026 12:00:00 GMT")
			w.Header().Set("ETag", `"test-etag"`)
			_, _ = w.Write(data)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected S3 method %s", r.Method)
		}
	}))
	defer server.Close()
	destination, err := NewS3Destination(S3Config{Endpoint: server.URL, Bucket: "test-backups", Region: "us-east-1", Prefix: "instance", AccessKey: "test-access", SecretKey: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "backup.age")
	data := []byte("verified-archive-bytes")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	if err := destination.PutVerified(context.Background(), "one.age", file, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	corrupt = true
	mu.Unlock()
	if err := destination.PutVerified(context.Background(), "two.age", file, hex.EncodeToString(hash[:])); err == nil {
		t.Fatal("corrupt external bytes accepted")
	}
	if err := destination.Delete(context.Background(), "one.age"); err != nil {
		t.Fatal(err)
	}
}
