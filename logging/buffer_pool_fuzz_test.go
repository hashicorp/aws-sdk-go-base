// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type fuzzReadCloser struct {
	data []byte
	err  error
}

func (r *fuzzReadCloser) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		if r.err == nil {
			return 0, io.EOF
		}
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *fuzzReadCloser) Close() error {
	return nil
}

func FuzzPooledBodyReads(f *testing.F) {
	f.Add([]byte("captured"), []byte("source"), []byte{1, 2, 3}, byte(0))
	f.Add([]byte{}, []byte("source"), []byte{1}, byte(1))
	f.Add([]byte("captured"), []byte{}, []byte{8, 1}, byte(2))

	f.Fuzz(func(t *testing.T, capturedBytes, sourceBytes, readSizes []byte, errorMode byte) {
		pool := newBufferPool(len(capturedBytes))
		captured := pool.Get()
		captured.Write(capturedBytes)
		var sourceErr error
		mode := errorMode % 3
		if mode != 0 {
			sourceErr = errors.New("source read")
		}
		if mode == 1 {
			sourceBytes = nil
		}
		original := &fuzzReadCloser{data: sourceBytes, err: sourceErr}
		body := NewPooledBody(pool, captured, original)

		var got []byte
		for i := 0; ; i++ {
			size := 1
			if len(readSizes) > 0 {
				size = int(readSizes[i%len(readSizes)]%32) + 1
			}
			chunk := make([]byte, size)
			n, err := body.Read(chunk)
			got = append(got, chunk[:n]...)
			if err == io.EOF {
				break
			}
			if err != nil {
				if sourceErr != nil && !errors.Is(err, sourceErr) {
					t.Fatalf("Read() error = %v, want %v", err, sourceErr)
				}
				if sourceErr == nil {
					t.Fatalf("Read() error = %v", err)
				}
				break
			}
		}

		want := append(append([]byte{}, capturedBytes...), sourceBytes...)
		if !bytes.Equal(got, want) {
			t.Fatalf("PooledBody bytes = %q, want %q", got, want)
		}
		if err := body.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
}
