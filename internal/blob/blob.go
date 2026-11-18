package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

type Blob interface {
	Open(ctx context.Context) (io.ReadCloser, error)
}

type Sized interface {
	Blob
	Size() int64
}

type Digested interface {
	Sized
	Digest() digest.Digest
}

type fetchDescriptor struct {
	fetcher    content.Fetcher
	descriptor ocispec.Descriptor
}

func FromDescriptor(fetcher content.Fetcher, desc ocispec.Descriptor) Digested {
	return &fetchDescriptor{
		fetcher:    fetcher,
		descriptor: desc,
	}
}

func (a *fetchDescriptor) Size() int64 {
	return a.descriptor.Size
}

func (a *fetchDescriptor) Digest() digest.Digest {
	return a.descriptor.Digest
}

func (a *fetchDescriptor) Open(ctx context.Context) (io.ReadCloser, error) {
	return a.fetcher.Fetch(ctx, a.descriptor)
}

type bytesBlob struct {
	data   []byte
	digest digest.Digest
}

func FromBytes(data []byte) Digested {
	d := digest.FromBytes(data)
	return &bytesBlob{
		data:   data,
		digest: d,
	}
}

func (a *bytesBlob) Size() int64 {
	return int64(len(a.data))
}

func (a *bytesBlob) Digest() digest.Digest {
	return a.digest
}

func (a *bytesBlob) Open(_ context.Context) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(a.data)), nil
}

func FromJSON(v any) (Blob, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return FromBytes(data), nil
}

var (
	EmptyJSON Blob
)

func init() {
	var err error
	EmptyJSON, err = FromJSON(struct{}{})
	if err != nil {
		panic(err)
	}
}

type sizeInfo struct {
	size int64
}

func (i *sizeInfo) Size() int64 {
	return i.size
}

type file string

func (f file) Open(_ context.Context) (io.ReadCloser, error) {
	return os.Open(string(f))
}

func FromFile(filename string) Blob {
	return file(filename)
}

type sizedFile struct {
	sizeInfo
	file
}

func SizedFromFile(filename string) (Sized, error) {
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	return &sizedFile{
		size: stat.Size(),
		file: file(filename),
	}, nil
}

type digestedFile struct {
	digestedFileInfo
	filename string
}

func (f *digestedFile) Open(_ context.Context) (io.ReadCloser, error) {
	return os.Open(f.filename)
}

func DigestedFromFile(filename string) (Digested, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	digester := digest.Canonical.Digester()
	size, err := io.Copy(digester.Hash(), f)
	if err != nil {
		return nil, fmt.Errorf("digesting file %q: %w", filename, err)
	}

	return &digestedFile{
		digest:   digester.Digest(),
		size:     size,
		filename: filename,
	}, nil
}

type digestedFileInfo struct {
	sizeInfo
	digest digest.Digest
}

func (i *digestedFileInfo) Digest() digest.Digest {
	return i.digest
}

func DigestedFromFSFile(fileSys fs.FS, filename string) (Digested, error) {
	f, err := fileSys.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	digester := digest.Canonical.Digester()
	size, err := io.Copy(digester.Hash(), f)
	if err != nil {
		return nil, fmt.Errorf("digesting file %q: %w", filename, err)
	}

	return &digestedFSFile{
		digest: digester.Digest(),
		size:   size,
		fs:     fileSys,
		name:   filename,
	}, nil
}

type digestedFSFile struct {
	digestedFileInfo
	fs   fs.FS
	name string
}

func (f *digestedFSFile) Open(_ context.Context) (io.ReadCloser, error) {
	return f.fs.Open(f.name)
}

type sized struct {
	Blob
	size int64
}

func ToSized(ctx context.Context, b Blob) (Sized, error) {
	switch s := b.(type) {
	case Sized:
		return s, nil
	case interface{ Size() int64 }:
		return &sized{b, s.Size()}, nil
	case interface{ Size() (int64, error) }:
		size, err := s.Size()
		if err != nil {
			return nil, fmt.Errorf("getting size of blob %q: %w", b, err)
		}
		return &sized{b, size}, nil
	case interface{ Stat() (fs.FileInfo, error) }:
		stat, err := s.Stat()
		if err != nil {
			return nil, fmt.Errorf("stating blob: %w", err)
		}
		return &sized{b, stat.Size()}, nil
	default:
	}

	rc, err := b.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening blob: %w", err)
	}
	defer func() { _ = rc.Close() }()

	size, err := io.Copy(io.Discard, rc)
	if err != nil {
		return nil, fmt.Errorf("reading blob: %w", err)
	}
	return &sized{b, size}, nil
}

func (s *sized) Size() int64 {
	return s.size
}

func ToDigested(ctx context.Context, b Blob) (Digested, error) {
	if d, ok := b.(Digested); ok {
		return d, nil
	}

	rc, err := b.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	digester := digest.Canonical.Digester()
	if _, err := io.Copy(digester.Hash(), rc); err != nil {
		return nil, fmt.Errorf("digesting blob: %w", err)
	}

	return &digested{
		Blob:   b,
		digest: digester.Digest(),
	}, nil
}

type digested struct {
	Blob
	size   int64
	digest digest.Digest
}

func (w *digested) Size() int64 {
	return w.size
}

func (w *digested) Digest() digest.Digest {
	return w.digest
}
