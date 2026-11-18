package local

import (
	"fmt"
	"io"
	"os"

	"github.com/ironcore-dev/metal-operator/internal/content"
)

// readerat implements io.ReaderAt in a completely stateless manner by opening
// the referenced file for each call to ReadAt.
type sizeReaderAt struct {
	size int64
	fp   *os.File
}

// OpenReader creates ReaderAt from a file
func OpenReader(p string) (content.ReaderAt, error) {
	fi, err := os.Stat(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		return nil, fmt.Errorf("blob %w", content.ErrNotFound)
	}

	fp, err := os.Open(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		return nil, fmt.Errorf("blob %w", content.ErrNotFound)
	}

	return sizeReaderAt{size: fi.Size(), fp: fp}, nil
}

func (ra sizeReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	return ra.fp.ReadAt(p, offset)
}

func (ra sizeReaderAt) Size() int64 {
	return ra.size
}

func (ra sizeReaderAt) Close() error {
	return ra.fp.Close()
}

func (ra sizeReaderAt) Reader() io.Reader {
	return io.LimitReader(ra.fp, ra.size)
}
