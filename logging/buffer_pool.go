// Copyright IBM Corp. 2015, 2025
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"bytes"
	"sync"
)

var BufferPool = newBufferPool()

type bufferPool struct {
	sync.Pool
}

func newBufferPool() *bufferPool {
	return &bufferPool{
		Pool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, MaxResponseBodyLen))
			},
		},
	}
}

func (p *bufferPool) Get() *bytes.Buffer {
	return p.Pool.Get().(*bytes.Buffer)
}

func (p *bufferPool) Put(v *bytes.Buffer) {
	v.Reset()
	p.Pool.Put(v)
}
