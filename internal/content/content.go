package content

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2/errdef"
)

var (
	ErrAlreadyExists = errors.New("already exists")
	ErrNotFound      = errors.New("not found")
)

type Writer interface {
	io.WriteCloser

	// Digest may return empty digest or panics until committed.
	Digest() digest.Digest

	// Commit commits the blob (but no roll-back is guaranteed on an error).
	// size and expected can be zero-value when unknown.
	// Commit always closes the writer, even on error.
	// ErrAlreadyExists aborts the writer.
	Commit(ctx context.Context, size int64, expected digest.Digest) error

	Status() (Status, error)
}

type Info struct {
	Digest digest.Digest
	Size   int64
}

type ReaderAt interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

func NewReader(ra ReaderAt) io.Reader {
	if rd, ok := ra.(interface{ Reader() io.Reader }); ok {
		return rd.Reader()
	}
	return io.NewSectionReader(ra, 0, ra.Size())
}

func ReadAll(ra ReaderAt) ([]byte, error) {
	rd := NewReader(ra)
	return io.ReadAll(io.LimitReader(rd, ra.Size()))
}

type Provider interface {
	ReaderAt(ctx context.Context, dgst digest.Digest) (ReaderAt, error)
}

type Ingester interface {
	Writer(ctx context.Context, opts ...WriterOption) (Writer, error)
}

type InfoProvider interface {
	Info(ctx context.Context, dgst digest.Digest) (Info, error)
}

type Store interface {
	InfoProvider
	Provider
	Ingester
}

type Status struct {
	Offset int64
	Total  int64
}

type WriterOptions struct {
	Expected digest.Digest
	Total    int64
}

type WriterOption func(*WriterOptions) error

func WithExpected(digest digest.Digest) WriterOption {
	return func(o *WriterOptions) error {
		o.Expected = digest
		return nil
	}
}

func WithTotal(size int64) WriterOption {
	return func(o *WriterOptions) error {
		o.Total = size
		return nil
	}
}

func WriteBuild(ctx context.Context, w Writer, build func(w io.Writer) error) (dgst digest.Digest, size int64, err error) {
	defer func() { _ = w.Close() }()

	if err := build(w); err != nil {
		return "", 0, err
	}

	status, err := w.Status()
	if err != nil {
		return "", 0, err
	}

	if err := w.Commit(ctx, status.Total, ""); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return "", 0, err
	}

	return w.Digest(), status.Total, nil
}

func IngestBuild(ctx context.Context, ing Ingester, build func(w io.Writer) error) (dgst digest.Digest, size int64, err error) {
	w, err := ing.Writer(ctx)
	if err != nil {
		return "", 0, err
	}

	return WriteBuild(ctx, w, build)
}

func IngestKnown(ctx context.Context, ing Ingester, dgst digest.Digest, size int64, open func() (io.ReadCloser, error)) error {
	w, err := ing.Writer(ctx,
		WithTotal(size),
		WithExpected(dgst),
	)
	if err != nil {
		if !errors.Is(err, ErrAlreadyExists) {
			return err
		}
		return nil
	}
	defer func() { _ = w.Close() }()

	rc, err := open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	if err := Copy(w, rc, size); err != nil {
		return err
	}
	if err := w.Commit(ctx, size, dgst); err != nil && !errors.Is(err, ErrAlreadyExists) {
		return err
	}
	return nil
}

func Copy(cw Writer, r io.Reader, expectedSize int64) error {
	copied, err := io.Copy(cw, r)
	if err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}
	if expectedSize != 0 && copied != expectedSize {
		// Short writes would return its own error, this indicates a read failure
		return fmt.Errorf("failed to read expected number of bytes: %w", io.ErrUnexpectedEOF)
	}
	return nil
}
