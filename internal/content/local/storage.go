package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ironcore-dev/metal-operator/internal/content"
	"github.com/opencontainers/go-digest"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0777)
}

func copyBufferVerifying(dst io.Writer, src io.Reader, buf []byte, dgst digest.Digest) (written int64, err error) {
	digester := dgst.Algorithm().Digester()
	written, err = io.CopyBuffer(io.MultiWriter(digester.Hash(), dst), src, buf)
	if err != nil {
		return written, err
	}

	actualDigest := digester.Digest()
	if actualDigest != dgst {
		return written, fmt.Errorf("mismatched digest: expected %s, got %s", dgst, actualDigest)
	}
	return written, nil
}

type Store struct {
	dir     string
	alg     digest.Algorithm
	bufPool *sync.Pool
}

func NewStore(dir string) *Store {
	return &Store{
		dir: dir,
		alg: digest.Canonical,
		bufPool: &sync.Pool{
			New: func() any {
				buffer := make([]byte, 1<<20)
				return &buffer
			},
		},
	}
}

func (s *Store) blobPath(dgst digest.Digest) (string, error) {
	if err := dgst.Validate(); err != nil {
		return "", fmt.Errorf("cannot calculate blob path from invalid digest %s: %w",
			dgst.String(), err)
	}
	return filepath.Join(s.dir, "blobs", dgst.Algorithm().String(), dgst.Encoded()), nil
}

func (s *Store) ingestDir() string {
	return filepath.Join(s.dir, "ingest")
}

func (s *Store) ingestRefDir(uid string) string {
	return filepath.Join(s.ingestDir(), uid)
}

func (s *Store) ingestRefDataPath(ingestRefDir string) string {
	return filepath.Join(ingestRefDir, "data")
}

func (s *Store) ingestRefStartedAtPath(ingestRefDir string) string {
	return filepath.Join(ingestRefDir, "startedat")
}

func (s *Store) ingestRefUpdatedAtPath(ingestRefDir string) string {
	return filepath.Join(ingestRefDir, "updatedat")
}

func (s *Store) ingestRefTotalPath(ingestRefDir string) string {
	return filepath.Join(ingestRefDir, "total")
}

func (s *Store) Writer(_ context.Context, opts ...content.WriterOption) (content.Writer, error) {
	var wOpts content.WriterOptions
	for _, opt := range opts {
		if err := opt(&wOpts); err != nil {
			return nil, err
		}
	}

	if wOpts.Expected != "" {
		p, err := s.blobPath(wOpts.Expected)
		if err != nil {
			return nil, fmt.Errorf("calculating blob path for writer: %w", err)
		}
		if _, err := os.Stat(p); err == nil {
			return nil, fmt.Errorf("content %s: %w", wOpts.Expected, content.ErrAlreadyExists)
		}
	}

	var (
		uid          = uuid.NewString()
		digester     = s.alg.Digester()
		ingestRefDir = s.ingestRefDir(uid)
	)

	if err := os.MkdirAll(ingestRefDir, 0777); err != nil {
		return nil, fmt.Errorf("creating ingest ref dir: %w", err)
	}

	f, err := os.OpenFile(s.ingestRefDataPath(ingestRefDir), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, fmt.Errorf("opening data file: %w", err)
	}

	return &writer{
		s:            s,
		ingestRefDir: ingestRefDir,
		file:         f,
		digester:     digester,
		total:        wOpts.Total,
	}, nil
}

func (s *Store) Info(_ context.Context, dgst digest.Digest) (content.Info, error) {
	p, err := s.blobPath(dgst)
	if err != nil {
		return content.Info{}, fmt.Errorf("calculating blob info path: %w", err)
	}

	fi, err := os.Stat(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return content.Info{}, err
		}
		return content.Info{}, fmt.Errorf("content %v: %w", dgst, content.ErrNotFound)
	}
	return content.Info{
		Digest: dgst,
		Size:   fi.Size(),
	}, nil
}

type writer struct {
	s            *Store
	ingestRefDir string
	ref          string
	file         *os.File
	digester     digest.Digester
	offset       int64
	total        int64
}

func (w *writer) Write(p []byte) (n int, err error) {
	n, err = w.file.Write(p)
	w.digester.Hash().Write(p[:n])
	w.offset += int64(n)
	return n, err
}

func (w *writer) Digest() digest.Digest {
	return w.digester.Digest()
}

func (w *writer) Status() (content.Status, error) {
	return content.Status{
		Offset: w.offset,
		Total:  w.total,
	}, nil
}

func (w *writer) Commit(_ context.Context, size int64, expected digest.Digest) error {
	f := w.file
	w.file = nil

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync failed: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat-ing ingest file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing ingest file: %w", err)
	}

	if size > 0 && size != fi.Size() {
		return fmt.Errorf("unexpected commit size %d, expected %d", fi.Size(), size)
	}

	dgst := w.digester.Digest()
	if expected != "" && expected != dgst {
		return fmt.Errorf("unexpected commit digest %s: expected %s", dgst, expected)
	}

	target, _ := w.s.blobPath(dgst)

	if _, err := os.Stat(target); err == nil {
		_ = os.RemoveAll(w.ingestRefDir)
		return fmt.Errorf("content %v: %w", dgst, content.ErrAlreadyExists)
	}

	if err := ensureDir(filepath.Dir(target)); err != nil {
		return err
	}

	if err := os.Rename(f.Name(), target); err != nil {
		return err
	}

	// Clean up ingest directory
	_ = os.RemoveAll(w.ingestRefDir)

	if err := os.Chmod(target, 0444); err != nil {
		return fmt.Errorf("chmod readonly: %w", err)
	}

	return nil
}

func (w *writer) Close() error {
	if w.file != nil {
		_ = w.file.Sync()
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (s *Store) ingest(dgst digest.Digest, content io.Reader) (path string, err error) {
	if err := ensureDir(s.ingestDir()); err != nil {
		return "", fmt.Errorf("ensuring ingest dir: %w", err)
	}

	fp, err := os.CreateTemp(s.ingestDir(), dgst.Encoded()+"_*")
	if err != nil {
		return "", err
	}
	filename := fp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(filename)
		}
	}()

	buf := s.bufPool.Get().(*[]byte)
	defer s.bufPool.Put(buf)

	if _, err := copyBufferVerifying(fp, content, *buf, dgst); err != nil {
		_ = fp.Close()
		return "", fmt.Errorf("ingesting: %w", err)
	}

	if err := fp.Close(); err != nil {
		return "", fmt.Errorf("closing ingest: %w", err)
	}

	if err := os.Chmod(filename, 0444); err != nil {
		return "", fmt.Errorf("chmod readonly: %w", err)
	}

	return filename, nil
}

func (s *Store) ReaderAt(_ context.Context, dgst digest.Digest) (content.ReaderAt, error) {
	p, err := s.blobPath(dgst)
	if err != nil {
		return nil, err
	}

	return OpenReader(p)
}

func (s *Store) Fetch(_ context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	target, err := s.blobPath(dgst)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(target)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("%s: %w", dgst, content.ErrNotFound)
	}
	return f, nil
}

func readFileString(path string) (string, error) {
	p, err := os.ReadFile(path)
	return string(p), err
}

// readFileTimestamp reads a file with just a timestamp present.
func readFileTimestamp(p string) (time.Time, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("%s: %w", err.Error(), content.ErrNotFound)
		}
		return time.Time{}, err
	}

	var t time.Time
	if err := t.UnmarshalText(b); err != nil {
		return time.Time{}, fmt.Errorf("could not parse timestamp file %v: %w", p, err)
	}

	return t, nil
}
