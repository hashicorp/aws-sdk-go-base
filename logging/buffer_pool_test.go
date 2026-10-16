// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

type testReadCloser struct {
	io.Reader
	err        error
	closeCalls int
}

func (r *testReadCloser) Close() error {
	r.closeCalls++
	return r.err
}

func TestPooledBody(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close body")
	original := &testReadCloser{Reader: bytes.NewReader([]byte("source")), err: closeErr}
	pool := &countingBufferPool{buffer: &bytes.Buffer{}}
	captured := pool.Get()
	captured.WriteString("captured")
	body := NewPooledBody(pool, captured, original)

	partial := make([]byte, 3)
	if n, err := body.Read(partial); n != len(partial) || err != nil {
		t.Fatalf("Read() = (%d, %v), want (%d, nil)", n, err, len(partial))
	}
	if got, want := string(partial), "cap"; got != want {
		t.Errorf("partial read = %q, want %q", got, want)
	}
	remaining, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(remaining), "turedsource"; got != want {
		t.Errorf("remaining = %q, want %q", got, want)
	}
	if pool.puts != 0 {
		t.Errorf("pool Put() calls before Close() = %d, want 0", pool.puts)
	}

	if err := body.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first Close() error = %v, want %v", err, closeErr)
	}
	if pool.puts != 1 {
		t.Errorf("pool Put() calls after first Close() = %d, want 1", pool.puts)
	}
	if err := body.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("second Close() error = %v, want %v", err, closeErr)
	}
	if pool.puts != 1 {
		t.Errorf("pool Put() calls after second Close() = %d, want 1", pool.puts)
	}
	if original.closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", original.closeCalls)
	}
}

func TestPooledBodyCloseConcurrent(t *testing.T) {
	closeErr := errors.New("close body")
	original := &testReadCloser{Reader: bytes.NewReader(nil), err: closeErr}
	pool := newBufferPool(16)
	captured := pool.Get()
	body := NewPooledBody(pool, captured, original)

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			<-start
			errs <- body.Close()
		}()
	}
	close(start)
	wait.Wait()
	close(errs)

	for err := range errs {
		if !errors.Is(err, closeErr) {
			t.Errorf("Close() error = %v, want %v", err, closeErr)
		}
	}
	if original.closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", original.closeCalls)
	}
}
