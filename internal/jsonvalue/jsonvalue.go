// Package jsonvalue reads loosely typed JSON decoded into any.
//
// Much of the Bronto API is consumed without a fixed schema: field names vary
// between endpoints and versions (id vs monitor_id, from vs logIds), and numbers
// may arrive as integers or floats. These helpers give that data the forgiving,
// truthiness-based reading the tools need, so a missing or oddly typed field
// reads as empty rather than failing the whole call.
//
// Values are expected to come from a decoder with UseNumber set, so numbers are
// json.Number and keep their exact text, but float64 is accepted too.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// Decode reads JSON into any, keeping numbers as json.Number so that large ids
// survive unchanged and integers are not re-encoded as floats.
func Decode(r io.Reader) (any, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var v any
	if err := decoder.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// Map returns v as an object, or nil when it is not one.
func Map(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// Slice returns v as an array, or nil when it is not one.
func Slice(v any) []any {
	s, _ := v.([]any)
	return s
}

// List reads the array a response carries under key. Bronto wraps most lists in
// an object ({"monitors": [...]}) but some endpoints answer with the bare array,
// so both are accepted. Anything else reads as an empty list.
func List(body any, key string) []any {
	if m, ok := body.(map[string]any); ok {
		return Slice(m[key])
	}
	return Slice(body)
}

// String renders a scalar as text: a string as itself, a number in its
// original notation. nil reads as "", and objects and arrays as their JSON.
func String(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return Text(v)
	}
}

// Float reads a number, or a string holding one. Anything else reads as 0.
func Float(v any) float64 {
	switch v := v.(type) {
	case json.Number:
		f, _ := v.Float64()
		return f
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	}
	return 0
}

// Int reads a number as an integer, truncating any fraction.
func Int(v any) int64 {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
	}
	return int64(Float(v))
}

// Truthy reports whether v is set to something other than its empty value:
// not nil, false, 0, "", [] or {}.
func Truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case json.Number, float64:
		return Float(v) != 0
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

// FirstTruthy returns the first of values that is Truthy, or nil.
func FirstTruthy(values ...any) any {
	for _, v := range values {
		if Truthy(v) {
			return v
		}
	}
	return nil
}

// Text renders v as compact JSON without HTML escaping, the form a text filter
// matches against.
func Text(v any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
