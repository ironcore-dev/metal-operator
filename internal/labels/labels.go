package labels

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"

	"k8s.io/apimachinery/pkg/api/validate/content"
)

func FormatValue(s string) string {
	if len(content.IsLabelValue(s)) == 0 {
		return s
	}

	h := fnv.New32a()
	_, err := h.Write([]byte(s))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("hash_%s_z", base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(h.Sum(nil)))
}
