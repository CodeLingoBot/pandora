// Copyright (c) 2017 Yandex LLC. All rights reserved.
// Use of this source code is governed by a MPL 2.0
// license that can be found in the LICENSE file.
// Author: Vladimir Skipor <skipor@yandex-team.ru>

package uri

import (
	"bytes"
	"context"
	"net/http"
	"sync"

	"github.com/facebookgo/stackerr"

	"github.com/yandex/pandora/ammo"
)

type decoder struct {
	sink chan<- ammo.Ammo
	ctx  context.Context

	ammoNum  int
	header   http.Header
	ammoPool sync.Pool
}

func newDecoder(sink chan<- ammo.Ammo, ctx context.Context) *decoder {
	return &decoder{
		sink:   sink,
		header: http.Header{},
		ammoPool: sync.Pool{
			New: func() interface{} {
				return &ammo.SimpleHTTP{}
			},
		},
		ctx: ctx,
	}
}

func (d *decoder) Decode(line []byte) error {
	if len(line) == 0 {
		return stackerr.Newf("empty line")
	}
	switch line[0] {
	case '/':
		return d.decodeURI(line)
	case '[':
		return d.decodeHeader(line)
	}
	return stackerr.Newf("every line should begin with '[' or '/'")
}

func (d *decoder) decodeURI(line []byte) error {
	// OPTIMIZE: reuse *http.Request, http.Header. Benchmark both variants.
	req, err := http.NewRequest("GET", string(line), nil)
	if err != nil {
		return stackerr.Newf("uri decode error: ", err)
	}
	for k, v := range d.header {
		// http.Request.Write sends Host header based on Host or URL.Host.
		if k == "Host" {
			req.URL.Host = v[0]
			req.Host = v[0]
		} else {
			req.Header[k] = v
		}
	}
	sh := d.ammoPool.Get().(*ammo.SimpleHTTP)
	sh.Reset(req, "REQUEST")
	select {
	case d.sink <- sh:
		d.ammoNum++
		return nil
	case <-d.ctx.Done():
		return d.ctx.Err()
	}
}

func (d *decoder) decodeHeader(line []byte) error {
	if len(line) < 3 || line[0] != '[' || line[len(line)-1] != ']' {
		return stackerr.Newf("header line should be like '[key: value]")
	}
	line = line[1 : len(line)-1]
	colonIdx := bytes.IndexByte(line, ':')
	if colonIdx < 0 {
		return stackerr.Newf("missing colon")
	}
	key := string(bytes.TrimSpace(line[:colonIdx]))
	val := string(bytes.TrimSpace(line[colonIdx+1:]))
	if key == "" {
		return stackerr.Newf("missing header key")
	}
	d.header.Set(key, val)
	return nil
}

func (d *decoder) ResetHeader() {
	for k := range d.header {
		delete(d.header, k)
	}
}