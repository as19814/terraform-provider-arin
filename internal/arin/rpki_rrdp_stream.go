package arin

import "io"

// Keep repository-wide disk/network/index limits separate from the 4 MiB
// per-object bound and the 8 MiB XML token bound. The latter also bounds XML
// comments and attributes before encoding/xml can allocate their full contents.
type rrdpStreamLimits struct {
	Encoded, Decoded int64
	Objects          int
}

func (l rrdpStreamLimits) valid() bool {
	return l.Encoded > 0 && l.Encoded <= 2<<30 && l.Decoded > 0 && l.Decoded <= 1<<30 && l.Objects > 0 && l.Objects <= 1000000
}

type rrdpStreamReader struct {
	input                     io.Reader
	remaining, tokenRemaining int64
}

func (r *rrdpStreamReader) Read(p []byte) (int, error) {
	// Read one extra byte to distinguish EOF at a bound from oversized input.
	limit := min(r.remaining, r.tokenRemaining)
	if int64(len(p)) > limit+1 {
		p = p[:limit+1]
	}
	n, err := r.input.Read(p)
	if int64(n) > limit {
		return 0, errRPKIRRDP
	}
	r.remaining -= int64(n)
	r.tokenRemaining -= int64(n)
	for _, b := range p[:n] {
		if b > 127 {
			return 0, errRPKIRRDP
		}
	}
	return n, err
}
