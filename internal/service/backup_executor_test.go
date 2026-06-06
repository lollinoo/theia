package service

// This file exercises backup executor behavior so refactors preserve the documented contract.

import (
	"bytes"
	"errors"
	"testing"
)

type shortWriterForTest struct {
	limit int
}

func (w *shortWriterForTest) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return w.limit, errors.New("short write")
	}
	return len(p), nil
}

func TestCountingWriterCountsBytesAcceptedByWrappedWriter(t *testing.T) {
	var buf bytes.Buffer
	counter := &countingWriter{w: &buf}

	n, err := counter.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 5 || counter.n != 5 || buf.String() != "hello" {
		t.Fatalf("Write result n=%d counter=%d buf=%q, want 5/5/hello", n, counter.n, buf.String())
	}
}

func TestCountingWriterCountsPartialWrites(t *testing.T) {
	counter := &countingWriter{w: &shortWriterForTest{limit: 2}}

	n, err := counter.Write([]byte("hello"))
	if err == nil {
		t.Fatal("Write error = nil, want wrapped writer error")
	}
	if n != 2 || counter.n != 2 {
		t.Fatalf("Write result n=%d counter=%d, want 2/2", n, counter.n)
	}
}
