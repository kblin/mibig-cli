package tree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	mibig "go.kblin.dev/go-mibig"
)

// Replace replaces the value of node n with the JSON in data. The JSON is
// put into the encoded entry, which is then decoded as a whole, so that the
// value is checked exactly like a value in an entry file, e.g. null elements
// in lists are rejected like they are when reading a file. On error, the
// entry is unchanged. The tree must be rebuilt afterwards.
func (t *Tree) Replace(n *Node, data []byte) error {
	data = bytes.TrimSpace(data)
	if !json.Valid(data) {
		var probe any
		return fmt.Errorf("invalid JSON: %w", json.Unmarshal(data, &probe))
	}
	if bytes.Equal(data, []byte("null")) {
		switch n.value.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Interface:
			// optional object, list, class, module or domain, or an
			// unknown key's raw value; the entry's decoding checks the rest
		default:
			return errors.New("null is not allowed here")
		}
	}
	doc, err := mibig.Marshal(t.Entry)
	if err != nil {
		return err
	}
	var chain []*Node
	for p := n; p.Parent != nil; p = p.Parent {
		chain = append(chain, p)
	}
	slices.Reverse(chain)
	doc, err = splice(doc, chain, data)
	if err != nil {
		return err
	}
	entry, err := mibig.Parse(doc)
	if err != nil {
		return err
	}
	*t.Entry = *entry
	return nil
}

// splice replaces the value at the location given by chain (the nodes from
// the root's child down to the target) in the JSON document doc. Object keys
// keep their order; a missing key is appended.
func splice(doc []byte, chain []*Node, value []byte) ([]byte, error) {
	if len(chain) == 0 {
		return value, nil
	}
	n := chain[0]
	if n.Index >= 0 {
		if n.Index == 0 && n.Parent.value.Type() == coreSeqType && isJSONString(doc) {
			// a core sequence written as a single string, not a list
			return splice(doc, chain[1:], value)
		}
		var items []json.RawMessage
		if err := json.Unmarshal(doc, &items); err != nil {
			return nil, fmt.Errorf("%s: %w", n.Parent.Path, err)
		}
		if n.Index >= len(items) {
			return nil, fmt.Errorf("%s: index out of range", n.Path)
		}
		item, err := splice(items[n.Index], chain[1:], value)
		if err != nil {
			return nil, err
		}
		items[n.Index] = item
		// join the raw elements, so that untouched ones stay as they are
		var buf bytes.Buffer
		buf.WriteByte('[')
		for j, item := range items {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.Write(item)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	}

	fields, err := objectFields(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", displayParent(n), err)
	}
	keys := append([]string{n.Key}, n.aliases...)
	i, seen := -1, 0
	for j, f := range fields {
		if !slices.Contains(keys, f.key) {
			continue
		}
		// repeated unknown keys: the node's occurrence
		if seen == n.occurrence {
			i = j
			break
		}
		seen++
	}
	if i < 0 && n.occurrence > 0 {
		return nil, fmt.Errorf("%s: key not found", n.Path)
	}
	if i < 0 {
		fields = append(fields, field{key: n.Key, value: []byte("null")})
		i = len(fields) - 1
	}
	fv, err := splice(fields[i].value, chain[1:], value)
	if err != nil {
		return nil, err
	}
	fields[i].value = fv

	var buf bytes.Buffer
	buf.WriteByte('{')
	for j, f := range fields {
		if j > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(quoteKey(f.key))
		buf.WriteByte(':')
		buf.Write(f.value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func isJSONString(data []byte) bool {
	data = bytes.TrimSpace(data)
	return len(data) > 0 && data[0] == '"'
}

// quoteKey encodes a key as a JSON string, without HTML escaping.
func quoteKey(key string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(key)
	return strings.TrimSuffix(buf.String(), "\n")
}

func displayParent(n *Node) string {
	if n.Parent == nil || n.Parent.Path == "" {
		return "entry"
	}
	return n.Parent.Path
}

type field struct {
	key   string
	value json.RawMessage
}

// objectFields returns the fields of a JSON object in order. A null value is
// treated as an empty object, so that optional objects can be filled in.
func objectFields(data []byte) ([]field, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, errors.New("expected a JSON object")
	}
	var fields []field
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("expected an object key, got " + strconv.Quote(fmt.Sprint(tok)))
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		fields = append(fields, field{key: key, value: value})
	}
	return fields, nil
}
