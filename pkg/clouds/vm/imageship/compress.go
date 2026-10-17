// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package imageship

import (
	"io"
	"sync/atomic"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

// compressStream returns a reader over the compressed form of src, and a function
// reporting how many compressed bytes have been read out of it so far.
//
// The compression runs in its own goroutine against an io.Pipe so that the whole
// transfer stays a single stream: the local daemon is still writing the tar while
// the far end is already loading layers out of it, and neither side ever holds the
// image on disk. An error on the compressing side closes the pipe with that error,
// so the consumer (the SSH session's stdin) sees it instead of a truncated stream.
func compressStream(src io.Reader, compression string) (io.ReadCloser, func() int64) {
	if compression == CompressionNone {
		counted := &countingReader{r: src}
		return io.NopCloser(counted), counted.count
	}

	pr, pw := io.Pipe()
	counted := &countingWriter{w: pw}
	go func() {
		// CloseWithError(nil) is CloseWithError(io.EOF) — a clean end of stream.
		_ = pw.CloseWithError(encodeInto(counted, src, compression))
	}()
	return pr, counted.count
}

func encodeInto(dst io.Writer, src io.Reader, compression string) error {
	enc, err := newEncoder(dst, compression)
	if err != nil {
		return err
	}
	if _, err := io.Copy(enc, src); err != nil {
		_ = enc.Close()
		return err
	}
	// The Close is what flushes the final frame; skipping it on the happy path
	// produces a stream `docker load` rejects as a truncated archive.
	return enc.Close()
}

func newEncoder(dst io.Writer, compression string) (io.WriteCloser, error) {
	if compression == CompressionGzip {
		return gzip.NewWriterLevel(dst, gzip.DefaultCompression)
	}
	// SpeedDefault (level 3) rather than anything higher: an image layer is mostly
	// already-compressed blobs, so the extra CPU buys very little and the link is
	// the bottleneck this exists to relieve.
	return zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedDefault))
}

// The two counters are read from the shipping goroutine while the compressing
// goroutine writes them, so both are atomic — a plain int64 here is a data race that
// only shows up under -race, in a byte count nobody would think to distrust.

type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingReader) count() int64 {
	return c.n.Load()
}

type countingWriter struct {
	w io.Writer
	n atomic.Int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n.Add(int64(n))
	return n, err
}

func (c *countingWriter) count() int64 {
	return c.n.Load()
}
