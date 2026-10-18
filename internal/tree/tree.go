// Package tree presents a MIBiG entry as a tree of nodes that can be shown
// and edited generically. The tree is derived from the Go types of the
// go-mibig package by reflection, so that every part of the schema can be
// edited without code specific to it. Node paths use the same format as the
// paths of validation issues, e.g. "loci[0].evidence[1].method".
//
// A tree is a snapshot: after any change to the entry, build a new tree.
package tree

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	mibig "go.kblin.dev/go-mibig"
)

// Kind is the kind of a node.
type Kind int

const (
	// Object is a JSON object; its children are its fields.
	Object Kind = iota
	// List is a JSON array; its children are its elements.
	List
	// String, Int, Number and Bool are editable scalar values.
	String
	Int
	Number
	Bool
	// Raw is JSON that the Go types do not model, e.g. unknown class
	// variants. It can only be edited as JSON.
	Raw
	// Absent is an optional object that is not set, e.g. a missing
	// "genes" section. It can be created.
	Absent
)

func (k Kind) String() string {
	switch k {
	case Object:
		return "object"
	case List:
		return "list"
	case String:
		return "string"
	case Int:
		return "integer"
	case Number:
		return "number"
	case Bool:
		return "boolean"
	case Raw:
		return "raw JSON"
	case Absent:
		return "not set"
	}
	return "unknown"
}

// Node is a value in the entry.
type Node struct {
	// Key is the JSON key of a field, or "" for list elements and the root.
	Key string
	// Index is the position of a list element, -1 for other nodes.
	Index int
	// Path is the location of the node, in the format of issue paths.
	Path string
	Kind Kind
	// Optional is set for scalar values that can be unset (pointer types),
	// e.g. a boolean that is neither true nor false.
	Optional bool
	// Variant is the class, module or domain type of polymorphic objects.
	Variant string
	// Unknown is set for keys that are not part of the schema. Their values
	// are Raw.
	Unknown bool
	// occurrence counts earlier unknown keys of the same name in the object:
	// JSON objects can repeat keys, and the decoder keeps unknown ones.
	occurrence int

	Parent   *Node
	Children []*Node

	// aliases are other keys of the field accepted when reading, e.g.
	// "comments" for "comment". Issues use the key as written.
	aliases []string

	// value is the Go value of the node, as stored in its parent: the
	// pointer or interface for optional values. It is addressable.
	value reflect.Value
	// target is the value scalar edits apply to, usually value.
	target reflect.Value
	// extras of the struct that holds the field, if any.
	extras *mibig.Extras
}

// Tree is a tree built from an entry.
type Tree struct {
	Root  *Node
	Entry *mibig.Entry
	nodes map[string]*Node
}

var (
	classType      = reflect.TypeFor[mibig.Class]()
	moduleType     = reflect.TypeFor[mibig.Module]()
	domainType     = reflect.TypeFor[mibig.Domain]()
	numberType     = reflect.TypeFor[json.Number]()
	entryType      = reflect.TypeFor[mibig.Entry]()
	rawType        = reflect.TypeFor[json.RawMessage]()
	extrasType     = reflect.TypeFor[mibig.Extras]()
	coreSeqType    = reflect.TypeFor[mibig.CoreSequence]()
	bioNameType    = reflect.TypeFor[mibig.BioactivityName]()
	unknownClass   = reflect.TypeFor[*mibig.UnknownClass]()
	unknownModule  = reflect.TypeFor[*mibig.UnknownModule]()
	unknownDomain  = reflect.TypeFor[*mibig.UnknownDomain]()
	sequencesIndex = mustField(coreSeqType, "Sequences")
	activityIndex  = mustField(bioNameType, "Activity")
)

func mustField(t reflect.Type, name string) []int {
	f, ok := t.FieldByName(name)
	if !ok {
		panic("tree: " + t.String() + " has no field " + name)
	}
	return f.Index
}

// Build builds the tree of an entry.
func Build(e *mibig.Entry) *Tree {
	t := &Tree{Entry: e, nodes: map[string]*Node{}}
	t.Root = &Node{Index: -1, Kind: Object}
	t.Root.value = reflect.ValueOf(e).Elem()
	t.Root.target = t.Root.value
	t.addFields(t.Root, t.Root.value)
	t.index(t.Root)
	// paths through alias keys, for issues about fields written with an
	// alias; they never shadow canonical paths
	t.indexAliases(t.Root, nil)
	return t
}

func (t *Tree) index(n *Node) {
	t.nodes[n.Path] = n
	for _, child := range n.Children {
		t.index(child)
	}
}

// indexAliases registers the paths of n and its descendants that go through
// alias keys. alts are the alternative paths of n.
func (t *Tree) indexAliases(n *Node, alts []string) {
	for _, alt := range alts {
		if _, ok := t.nodes[alt]; !ok {
			t.nodes[alt] = n
		}
	}
	for _, child := range n.Children {
		suffix := child.Path[len(n.Path):]
		var childAlts []string
		for _, alt := range alts {
			childAlts = append(childAlts, alt+suffix)
		}
		for _, alias := range child.aliases {
			childAlts = append(childAlts, JoinPath(n.Path, alias))
			for _, alt := range alts {
				childAlts = append(childAlts, JoinPath(alt, alias))
			}
		}
		t.indexAliases(child, childAlts)
	}
}

// Node returns the node at path, or nil.
func (t *Tree) Node(path string) *Node {
	return t.nodes[path]
}

// Nearest returns the node at path, or its closest existing ancestor, e.g.
// for issues about values that are missing.
func (t *Tree) Nearest(path string) *Node {
	for {
		if n, ok := t.nodes[path]; ok {
			return n
		}
		if path == "" {
			return t.Root
		}
		path = ParentPath(path)
	}
}

// Walk calls fn for n and all its descendants, depth first.
func (n *Node) Walk(fn func(*Node)) {
	fn(n)
	for _, child := range n.Children {
		child.Walk(fn)
	}
}

// Depth returns the number of ancestors of n, not counting the root.
func (n *Node) Depth() int {
	d := 0
	for p := n.Parent; p != nil && p.Parent != nil; p = p.Parent {
		d++
	}
	return d
}

// IsContainer reports whether the node has (or can have) children.
func (n *Node) IsContainer() bool {
	return n.Kind == Object || n.Kind == List
}

// IsScalar reports whether the node can be edited as a single value.
func (n *Node) IsScalar() bool {
	switch n.Kind {
	case String, Int, Number, Bool:
		return true
	}
	return false
}

// Type returns the Go type of the node's value.
func (n *Node) Type() reflect.Type {
	return n.value.Type()
}

// addFields adds the JSON fields of struct value sv as children of parent.
func (t *Tree) addFields(parent *Node, sv reflect.Value) {
	var extras *mibig.Extras
	if f := sv.FieldByName("Extra"); f.IsValid() && f.Type() == extrasType {
		extras = f.Addr().Interface().(*mibig.Extras)
	}
	t.addStructFields(parent, sv, extras)
	if extras == nil {
		return
	}
	seen := map[string]int{}
	for i := range extras.Unknown {
		field := &extras.Unknown[i]
		child := &Node{Key: field.Key, Index: -1, Parent: parent, Kind: Raw, Unknown: true, extras: extras}
		child.occurrence = seen[field.Key]
		seen[field.Key]++
		child.Path = JoinPath(parent.Path, field.Key)
		if child.occurrence > 0 {
			// repeated keys need distinct paths; issues point at the first
			child.Path += "#" + strconv.Itoa(child.occurrence+1)
		}
		child.value = reflect.ValueOf(&field.Value).Elem()
		child.target = child.value
		parent.Children = append(parent.Children, child)
	}
}

func (t *Tree) addStructFields(parent *Node, sv reflect.Value, extras *mibig.Extras) {
	st := sv.Type()
	for i := range st.NumField() {
		sf := st.Field(i)
		tag := sf.Tag.Get("json")
		if sf.Anonymous && tag == "" && sf.Type.Kind() == reflect.Struct {
			t.addStructFields(parent, sv.Field(i), extras)
			continue
		}
		if !sf.IsExported() || tag == "-" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")
		if key == "" {
			key = sf.Name
		}
		child := t.newNode(parent, key, -1, sv.Field(i))
		child.extras = extras
		for _, opt := range strings.Split(sf.Tag.Get("mibig"), ",") {
			if alias, ok := strings.CutPrefix(opt, "alias="); ok {
				child.aliases = append(child.aliases, alias)
			}
		}
		parent.Children = append(parent.Children, child)
	}
}

// newNode creates the node for value v, a field or list element of parent.
func (t *Tree) newNode(parent *Node, key string, index int, v reflect.Value) *Node {
	n := &Node{Key: key, Index: index, Parent: parent, value: v, target: v}
	if index >= 0 {
		n.Path = parent.Path + "[" + strconv.Itoa(index) + "]"
	} else {
		n.Path = JoinPath(parent.Path, key)
	}
	t.classify(n, v)
	return n
}

// classify sets the kind of n and adds its children.
func (t *Tree) classify(n *Node, v reflect.Value) {
	typ := v.Type()
	switch {
	case typ == numberType:
		n.Kind = Number
		return
	case typ == rawType:
		n.Kind = Raw
		return
	case typ == coreSeqType:
		// a list of sequences; the legacy flag is kept as it is
		n.Kind = List
		t.addElements(n, v.FieldByIndex(sequencesIndex))
		return
	case typ == bioNameType:
		name := v.Interface().(mibig.BioactivityName)
		if !name.Legacy && name.Comment == "" && name.Extra.IsZero() {
			n.Kind = String
			n.target = v.FieldByIndex(activityIndex)
			return
		}
		n.Kind = Object
		t.addFields(n, v)
		return
	}

	switch typ.Kind() {
	case reflect.String:
		n.Kind = String
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n.Kind = Int
	case reflect.Float32, reflect.Float64:
		n.Kind = Number
	case reflect.Bool:
		n.Kind = Bool
	case reflect.Struct:
		n.Kind = Object
		t.addFields(n, v)
	case reflect.Slice:
		n.Kind = List
		t.addElements(n, v)
	case reflect.Pointer:
		elem := typ.Elem()
		if elem.Kind() != reflect.Struct {
			// optional scalar, e.g. *bool
			t.classify(n, reflect.New(elem).Elem())
			n.Optional = true
			n.target = v
			return
		}
		if v.IsNil() {
			n.Kind = Absent
			return
		}
		t.classify(n, v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			n.Kind = Absent
			return
		}
		t.classifyVariant(n, v.Elem())
	default:
		n.Kind = Raw
	}
}

// classifyVariant handles a class, module or domain held by an interface.
func (t *Tree) classifyVariant(n *Node, v reflect.Value) {
	n.Variant = variantTag(v.Interface())
	switch v.Type() {
	case unknownClass, unknownModule, unknownDomain:
		n.Kind = Raw
		return
	}
	if v.Kind() == reflect.Pointer && v.IsNil() {
		n.Kind = Absent
		return
	}
	n.Kind = Object
	t.addFields(n, reflect.Indirect(v))
}

func (t *Tree) addElements(n *Node, slice reflect.Value) {
	for i := range slice.Len() {
		n.Children = append(n.Children, t.newNode(n, "", i, slice.Index(i)))
	}
}

func variantTag(v any) string {
	switch x := v.(type) {
	case mibig.Class:
		return string(x.ClassName())
	case mibig.Module:
		return string(x.ModuleType())
	case mibig.Domain:
		return string(x.DomainType())
	}
	return ""
}

// JoinPath appends an object key to a path, quoting keys that are not plain
// words like the go-mibig package does, e.g. `a["odd.key"]`.
func JoinPath(path, key string) string {
	plain := key != "" && !strings.ContainsFunc(key, func(r rune) bool {
		return !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	if !plain {
		// like go-mibig, without HTML escaping
		return path + "[" + quoteKey(key) + "]"
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

// occurrenceSuffix matches the suffix of repeated unknown keys, e.g. "#2".
// Keys containing "#" are quoted, so an unquoted suffix is unambiguous.
var occurrenceSuffix = regexp.MustCompile(`#[0-9]+$`)

// ParentPath removes the last segment of a path.
func ParentPath(path string) string {
	if loc := occurrenceSuffix.FindStringIndex(path); loc != nil {
		path = path[:loc[0]]
	}
	if strings.HasSuffix(path, "]") {
		// an index or a quoted key; quoted keys may contain brackets, so
		// find the opening bracket outside of the quotes
		if i := strings.LastIndex(path, `["`); i >= 0 && strings.HasSuffix(path, `"]`) {
			var key string
			if json.Unmarshal([]byte(path[i+1:len(path)-1]), &key) == nil {
				return path[:i]
			}
		}
		if i := strings.LastIndexByte(path, '['); i >= 0 {
			return path[:i]
		}
	}
	if i := strings.LastIndexAny(path, ".["); i >= 0 {
		return path[:i]
	}
	return ""
}
