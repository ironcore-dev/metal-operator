package initrd

import (
	"context"
	"io"

	"github.com/ironcore-dev/metal-operator/internal/blob"
)

func Merge(ctx context.Context, w io.Writer, initrds []blob.Blob) error {
	for _, initrd := range initrds {
		if err := func() error {
			rc, err := initrd.Open(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = rc.Close() }()

			_, err = io.Copy(w, rc)
			return err
		}(); err != nil {
			return err
		}
	}
	return nil
}

func MergeReader(ctx context.Context, initrds []blob.Blob) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		err := Merge(ctx, pw, initrds)
		_ = pw.CloseWithError(err)
	}()
	return pr
}
