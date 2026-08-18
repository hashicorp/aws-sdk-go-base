// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"io"
	"sync"
)

var BufferPool = newBufferPool()

type BufferPoolType struct {
	sync.Pool
}

func newBufferPool() *BufferPoolType {
	return &BufferPoolType{
		Pool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, MaxResponseBodyLen))
			},
		},
	}
}

func (p *BufferPoolType) Get() *bytes.Buffer {
	return p.Pool.Get().(*bytes.Buffer)
}

func (p *BufferPoolType) Put(v *bytes.Buffer) {
	v.Reset()
	p.Pool.Put(v)
}

// PooledBody is an io.ReadCloser that wraps a restored HTTP body alongside the
// pool buffer whose backing array is referenced by the reader. The buffer is
// returned to pool only when Close is called, ensuring it outlives the body read.
type PooledBody struct {
	io.Reader
	pool *BufferPoolType
	buf  *bytes.Buffer
	orig io.ReadCloser
}

// NewPooledBody constructs a PooledBody. pool is the buffer pool to return
// captured to on Close; captured is the pool buffer that was used to tee the
// body; orig is the original ReadCloser. The Reader presented to callers
// replays captured followed by any remaining bytes in orig.
func NewPooledBody(pool *BufferPoolType, captured *bytes.Buffer, orig io.ReadCloser) *PooledBody {
	return &PooledBody{
		Reader: io.MultiReader(bytes.NewReader(captured.Bytes()), orig),
		pool:   pool,
		buf:    captured,
		orig:   orig,
	}
}

func (b *PooledBody) Close() error {
	b.pool.Put(b.buf)
	return b.orig.Close()
}
