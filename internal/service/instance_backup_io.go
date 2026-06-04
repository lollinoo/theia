package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// computeEncryptionKeyHash returns the SHA-256 hash of the first 8 bytes of the encryption key.
// This allows verifying the correct key is used during restore without exposing the full key.
func computeEncryptionKeyHash(key []byte) string {
	if len(key) < 8 {
		// Key too short; hash what we have
		h := sha256.Sum256(key)
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(key[:8])
	return hex.EncodeToString(h[:])
}

// computeFileHash computes the SHA-256 hash of a file using streaming I/O.
func computeFileHash(path string) (string, error) {
	return computeFileHashContext(context.Background(), path)
}

// computeFileHashContext streams a file hash while honoring restore or backup cancellation.
func computeFileHashContext(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hash: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := copyWithContext(ctx, h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyWithContext copies bytes in chunks and checks cancellation before reads and writes.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			if err := ctx.Err(); err != nil {
				return written, err
			}
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}
