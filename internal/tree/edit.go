package tree

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	mibig "go.kblin.dev/go-mibig"
)

// summaryKeys are the fields used to describe an object in one line, in
// order of preference.
var summaryKeys = []string{"name", "accession", "method", "function", "gene", "id", "type", "class", "subclass"}

// Text returns the value of a scalar node as text, "" if it is empty or
// unset.
func (n *Node) Text() string {
	v := n.target
	if n.Optional {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch n.Kind {
	case String, Number:
		return v.String()
	case Int:
		if v.CanInt() {
			return strconv.FormatInt(v.Int(), 10)
		}
		return strconv.FormatUint(v.Uint(), 10)
	case Bool:
		return strconv.FormatBool(v.Bool())
	}
	return ""
}

// IsSet reports whether a scalar node has a value: optional values are set
// if not nil, other values if not zero. Booleans that are not optional are
// always set.
func (n *Node) IsSet() bool {
	if n.Optional {
		return !n.target.IsNil()
	}
	if n.Kind == Bool {
		return true
	}
	return !n.target.IsZero()
}

// Summary describes the node in one line, for display next to its label.
func (n *Node) Summary() string {
	switch n.Kind {
	case List:
		switch len(n.Children) {
		case 0:
			return "empty"
		case 1:
			return "1 item"
		}
		return fmt.Sprintf("%d items", len(n.Children))
	case Object:
		var parts []string
		if n.Variant != "" {
			parts = append(parts, n.Variant)
		}
		if text := n.describingField(); text != "" {
			parts = append(parts, text)
		}
		return strings.Join(parts, " · ")
	case Raw:
		if n.Unknown {
			return "unknown key"
		}
		if n.Variant != "" {
			return n.Variant + " (unknown, raw JSON)"
		}
		return "raw JSON"
	case Absent:
		return "not set"
	}
	if !n.IsSet() {
		return ""
	}
	return n.Text()
}

// describingField returns the value of the first field of an object that
// describes it, e.g. its name.
func (n *Node) describingField() string {
	for _, key := range summaryKeys {
		for _, child := range n.Children {
			if child.Key == key && child.IsScalar() {
				if text := child.Text(); text != "" && text != n.Variant {
					return text
				}
			}
		}
	}
	return ""
}

// Label is the name of the node: its key, or its index for list elements.
func (n *Node) Label() string {
	if n.Index >= 0 {
		return "[" + strconv.Itoa(n.Index) + "]"
	}
	if n.Parent == nil {
		return "entry"
	}
	return n.Key
}

// discard drops the preserved input form of the field of n, so that the
// field is written from its Go value: after an edit, the user's value wins
// over e.g. a "comment": null in the input.
func (n *Node) discard() {
	if n.extras != nil && n.Key != "" {
		n.extras.Discard(n.Key)
	}
}

// SetText sets a scalar node from text. For optional values, "" unsets the
// value; for others it sets the zero value.
func (n *Node) SetText(text string) error {
	if !n.IsScalar() {
		return fmt.Errorf("%s is not a single value", n.Kind)
	}
	target := n.target
	if n.Optional {
		if text == "" {
			target.SetZero()
			n.discard()
			return nil
		}
		ptr := reflect.New(target.Type().Elem())
		if err := setScalar(n.Kind, ptr.Elem(), text); err != nil {
			return err
		}
		target.Set(ptr)
		n.discard()
		return nil
	}
	if err := setScalar(n.Kind, target, text); err != nil {
		return err
	}
	n.discard()
	return nil
}

// ParseText checks text for the node without setting it.
func (n *Node) ParseText(text string) error {
	if n.Optional && text == "" {
		return nil
	}
	typ := n.target.Type()
	if n.Optional {
		typ = typ.Elem()
	}
	return setScalar(n.Kind, reflect.New(typ).Elem(), text)
}

func setScalar(kind Kind, v reflect.Value, text string) error {
	switch kind {
	case String:
		v.SetString(text)
	case Number:
		text = strings.TrimSpace(text)
		if text == "" {
			v.SetZero()
			return nil
		}
		if v.Kind() == reflect.String {
			// json.Number: check the syntax, keep the text as written
			var num json.Number
			if err := json.Unmarshal([]byte(text), &num); err != nil {
				return errors.New("not a number")
			}
			v.SetString(text)
			return nil
		}
		f, err := strconv.ParseFloat(text, v.Type().Bits())
		if err != nil {
			return errors.New("not a number")
		}
		v.SetFloat(f)
	case Int:
		text = strings.TrimSpace(text)
		if text == "" {
			v.SetZero()
			return nil
		}
		if v.CanInt() {
			i, err := strconv.ParseInt(text, 10, v.Type().Bits())
			if err != nil {
				return errors.New("not a whole number")
			}
			v.SetInt(i)
		} else {
			u, err := strconv.ParseUint(text, 10, v.Type().Bits())
			if err != nil {
				return errors.New("not a non-negative whole number")
			}
			v.SetUint(u)
		}
	case Bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return errors.New("not true or false")
		}
		v.SetBool(b)
	default:
		return fmt.Errorf("cannot set a %s from text", kind)
	}
	return nil
}

// Variants returns the types to choose from when creating the node (if it
// is an absent class, module or domain) or adding an element to it (if it
// is a list of them). It returns nil if there is nothing to choose.
func (n *Node) Variants() []string {
	switch n.Kind {
	case List:
		return variantsOf(n.listValue().Type().Elem())
	case Absent:
		return variantsOf(n.value.Type())
	}
	return nil
}

func variantsOf(typ reflect.Type) []string {
	var tags []string
	switch typ {
	case classType:
		for _, name := range mibig.ClassNames() {
			tags = append(tags, string(name))
		}
	case moduleType:
		for _, t := range mibig.ModuleTypes() {
			tags = append(tags, string(t))
		}
	case domainType:
		for _, t := range mibig.DomainTypes() {
			tags = append(tags, string(t))
		}
	}
	return tags
}

// newValue returns a new, empty value of type typ, of the given variant for
// interfaces.
func newValue(typ reflect.Type, variant string) (reflect.Value, error) {
	var value any
	switch typ {
	case classType:
		value = mibig.NewClass(mibig.ClassName(variant))
	case moduleType:
		value = mibig.NewModule(mibig.ModuleType(variant))
	case domainType:
		value = mibig.NewDomain(mibig.DomainType(variant))
	default:
		if typ.Kind() == reflect.Pointer {
			return reflect.New(typ.Elem()), nil
		}
		return reflect.New(typ).Elem(), nil
	}
	if value == nil {
		return reflect.Value{}, fmt.Errorf("unknown type %q", variant)
	}
	v := reflect.New(typ).Elem()
	v.Set(reflect.ValueOf(value))
	return v, nil
}

// CanCreate reports whether Create applies to the node.
func (n *Node) CanCreate() bool {
	return n.Kind == Absent
}

// Create sets an absent optional object to a new, empty value, of the given
// variant for classes, modules and domains.
func (n *Node) Create(variant string) error {
	if n.Kind != Absent {
		return errors.New("value already exists")
	}
	v, err := newValue(n.value.Type(), variant)
	if err != nil {
		return err
	}
	n.value.Set(v)
	n.discard()
	return nil
}

// CanAdd reports whether Add applies to the node: lists, and elements of
// lists (to add a sibling).
func (n *Node) CanAdd() bool {
	return n.Kind == List || n.Index >= 0
}

// AddTarget returns the list that Add adds to and the position the new
// element is inserted at: the end of a list, or after a list element.
func (n *Node) AddTarget() (list *Node, at int) {
	if n.Kind == List {
		return n, len(n.Children)
	}
	if n.Index >= 0 {
		return n.Parent, n.Index + 1
	}
	return nil, -1
}

// Add inserts a new, empty element into the list node at position at, of
// the given variant for lists of classes, modules and domains. It returns
// the path of the new element.
func (n *Node) Add(at int, variant string) (string, error) {
	if n.Kind != List {
		return "", errors.New("not a list")
	}
	slice := n.listValue()
	if at < 0 || at > slice.Len() {
		return "", fmt.Errorf("position %d out of range", at)
	}
	elem, err := newValue(slice.Type().Elem(), variant)
	if err != nil {
		return "", err
	}
	grown := reflect.MakeSlice(slice.Type(), 0, slice.Len()+1)
	grown = reflect.AppendSlice(grown, slice.Slice(0, at))
	grown = reflect.Append(grown, elem)
	grown = reflect.AppendSlice(grown, slice.Slice(at, slice.Len()))
	slice.Set(grown)
	n.discard()
	return n.Path + "[" + strconv.Itoa(at) + "]", nil
}

// listValue returns the slice of a list node.
func (n *Node) listValue() reflect.Value {
	v := reflect.Indirect(n.value)
	if v.Type() == coreSeqType {
		return v.FieldByIndex(sequencesIndex)
	}
	return v
}

// CanRemove reports whether Remove applies to the node.
func (n *Node) CanRemove() bool {
	if n.Parent == nil {
		return false
	}
	if n.Index >= 0 || n.Unknown || n.hasPreserved() {
		return true
	}
	switch n.Kind {
	case Absent:
		return false
	case List:
		// an empty list that is present can be removed too
		return len(n.Children) > 0 || !n.listValue().IsNil()
	case Object:
		// only optional objects can be removed
		k := n.value.Kind()
		return k == reflect.Pointer || k == reflect.Interface
	}
	return n.IsSet()
}

// hasPreserved reports whether the field of n is written in a preserved
// input form, e.g. an empty value like "comment": null.
func (n *Node) hasPreserved() bool {
	if n.extras == nil || n.Key == "" || n.Unknown {
		return false
	}
	_, ok := n.extras.Preserved(n.Key)
	return ok
}

// Remove deletes a list element or unknown key, unsets an optional value,
// empties a list or resets a value to its zero value. A preserved empty
// value of the field is dropped too, so that the key is removed from the
// file if it is optional.
func (n *Node) Remove() error {
	if !n.CanRemove() {
		return errors.New("nothing to remove")
	}
	if n.Unknown {
		// only this occurrence of a repeated key
		seen := 0
		for i, f := range n.extras.Unknown {
			if f.Key != n.Key {
				continue
			}
			if seen == n.occurrence {
				n.extras.Unknown = slices.Delete(n.extras.Unknown, i, i+1)
				return nil
			}
			seen++
		}
		return errors.New("unknown key not found")
	}
	if n.Index >= 0 {
		slice := n.Parent.listValue()
		shrunk := reflect.MakeSlice(slice.Type(), 0, slice.Len()-1)
		shrunk = reflect.AppendSlice(shrunk, slice.Slice(0, n.Index))
		shrunk = reflect.AppendSlice(shrunk, slice.Slice(n.Index+1, slice.Len()))
		slice.Set(shrunk)
		n.Parent.discard()
		return nil
	}
	if n.Kind == List {
		n.listValue().SetZero()
	} else if n.IsScalar() {
		n.target.SetZero()
	} else {
		n.value.SetZero()
	}
	n.discard()
	return nil
}

// Move moves a list element by delta positions and returns its new path.
func (n *Node) Move(delta int) (string, error) {
	if n.Index < 0 {
		return "", errors.New("not a list element")
	}
	to := n.Index + delta
	if to < 0 || to >= len(n.Parent.Children) {
		return n.Path, nil
	}
	reflect.Swapper(n.Parent.listValue().Interface())(n.Index, to)
	return n.Parent.Path + "[" + strconv.Itoa(to) + "]", nil
}

// MarshalJSON encodes the value of the node like it is written to the
// entry's file.
func (n *Node) MarshalJSON() ([]byte, error) {
	return mibig.Marshal(n.value.Interface())
}
