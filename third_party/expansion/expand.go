// Originally from https://github.com/kubernetes/kubernetes/blob/master/third_party/forked/golang/expansion/expand.go
// Forked under the Apache License 2.0. See LICENSES/Apache-2.0.txt.

package expansion

import (
	"bytes"
	"unicode/utf8"
)

const (
	operator        = '$'
	referenceOpener = '('
	referenceCloser = ')'
)

// MappingFuncFor returns a mapping function for use with Expand that
// implements the expansion semantics defined in the expansion spec; it
// returns the input string wrapped in the expansion syntax if no mapping
// for the input is found.
func MappingFuncFor(context ...map[string]string) func(string) string {
	return func(input string) string {
		for _, vars := range context {
			val, ok := vars[input]
			if ok {
				return val
			}
		}
		return syntaxWrap(input)
	}
}

// syntaxWrap returns the input string wrapped by the expansion syntax.
func syntaxWrap(input string) string {
	return string(operator) + string(referenceOpener) + input + string(referenceCloser)
}

// Expand replaces variable references in the input string according to
// the expansion spec using the given mapping function to resolve the
// values of variables.
//
// Escape semantics:
//   - $(VAR_NAME) is replaced with the mapped value of VAR_NAME.
//   - $$(VAR_NAME) is replaced with the literal string $(VAR_NAME).
//   - References to undefined variables are left as-is.
func Expand(input string, mapping func(string) string) string {
	var buf bytes.Buffer
	checkpoint := 0
	for cursor := 0; cursor < len(input); cursor++ {
		if input[cursor] == operator && cursor+1 < len(input) {
			buf.WriteString(input[checkpoint:cursor])

			read, isVar, advance := tryReadVariableName(input[cursor+1:])

			if isVar {
				buf.WriteString(mapping(read))
			} else {
				buf.WriteString(read)
			}

			cursor += advance
			checkpoint = cursor + 1
		}
	}
	return buf.String() + input[checkpoint:]
}

// tryReadVariableName attempts to read a variable name from the input string
// and returns the content read, whether it is a variable reference, and the
// number of bytes consumed. The input is assumed not to contain the leading $.
func tryReadVariableName(input string) (string, bool, int) {
	r, size := utf8.DecodeRuneInString(input)
	switch r {
	case operator:
		// Escaped operator: $$ → emit a single $.
		return input[0:size], false, size
	case referenceOpener:
		// Scan for the closing paren.
		for i := 1; i < len(input); i++ {
			if input[i] == referenceCloser {
				return input[1:i], true, i + 1
			}
		}
		// Incomplete reference: leave as-is.
		return string(operator) + string(referenceOpener), false, 1
	default:
		// Not an expression opener; emit the operator and the rune.
		return string(operator) + string(r), false, size
	}
}
