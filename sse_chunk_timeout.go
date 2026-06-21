package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// ─── SSE Chunk Timeout (MiMo-Code 6) ───────────────────────────────────────
//
// Wraps SSE response streams with a per-chunk timeout mechanism.
// Prevents indefinite hangs on stalled SSE connections.
//
// MiMo-Code source: provider/provider.ts (41-100 lines)

const (
	// DefaultChunkTimeout is the default timeout between SSE chunks
	DefaultChunkTimeout = 8 * time.Minute
)

// SSEChunkReader wraps an io.Reader with per-chunk timeout.
type SSEChunkReader struct {
	reader  io.Reader
	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewSSEChunkReader creates a new SSE chunk reader with timeout.
func NewSSEChunkReader(reader io.Reader, timeout time.Duration) *SSEChunkReader {
	if timeout <= 0 {
		timeout = DefaultChunkTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &SSEChunkReader{
		reader:  reader,
		timeout: timeout,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Read reads data with a per-read timeout.
func (r *SSEChunkReader) Read(p []byte) (int, error) {
	type result struct {
		n   int
		err error
	}

	ch := make(chan result, 1)
	go func() {
		n, err := r.reader.Read(p)
		ch <- result{n, err}
	}()

	select {
	case <-r.ctx.Done():
		return 0, fmt.Errorf("SSE read cancelled")
	case res := <-ch:
		return res.n, res.err
	case <-time.After(r.timeout):
		r.cancel()
		return 0, fmt.Errorf("SSE read timed out after %v", r.timeout)
	}
}

// Close cancels the reader context.
func (r *SSEChunkReader) Close() error {
	r.cancel()
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// SSEChunkTimeoutConfig holds SSE chunk timeout configuration.
type SSEChunkTimeoutConfig struct {
	Enabled bool          `json:"enabled"`
	Timeout time.Duration `json:"timeout"`
}

// NewSSEChunkTimeoutConfig creates a new config with defaults.
func NewSSEChunkTimeoutConfig() *SSEChunkTimeoutConfig {
	return &SSEChunkTimeoutConfig{
		Enabled: true,
		Timeout: DefaultChunkTimeout,
	}
}

// WrapReader wraps an io.Reader with SSE chunk timeout.
func WrapReader(reader io.Reader, config *SSEChunkTimeoutConfig) io.Reader {
	if config == nil || !config.Enabled {
		return reader
	}
	return NewSSEChunkReader(reader, config.Timeout)
}
