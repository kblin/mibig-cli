package tree

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mibig "go.kblin.dev/go-mibig"
)

func load(t *testing.T, name string) *mibig.Entry {
	t.Helper()
	entry, err := mibig.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// checkPaths checks that the tree has a node for every controlled field and
// that issue paths resolve to a node.
func checkPaths(t *testing.T, name string, entry *mibig.Entry) {
	t.Helper()
	tr := Build(entry)
	for path := range entry.Vocabularies() {
		n := tr.Node(path)
		if n == nil {
			t.Errorf("%s: no node for vocabulary path %s", name, path)
			continue
		}
		if !n.IsScalar() {
			t.Errorf("%s: controlled field %s is a %s", name, path, n.Kind)
		}
	}
	for _, issue := range entry.ValidateAt(mibig.QualityHigh) {
		if tr.Nearest(issue.Path) == nil {
			t.Errorf("%s: issue path %s does not resolve", name, issue.Path)
		}
		switch issue.Code {
		case mibig.CodeUnknownField, mibig.CodeEmptyValue:
			// these are fixed by removing the node
			if n := tr.Node(issue.Path); n == nil || !n.CanRemove() {
				t.Errorf("%s: cannot remove %s for %s", name, issue.Path, issue.Code)
			}
		}
	}
	tr.Root.Walk(func(n *Node) {
		if tr.Node(n.Path) != n {
			t.Errorf("%s: path %q is not unique", name, n.Path)
		}
	})
}

func TestPaths(t *testing.T) {
	for _, name := range []string{"BGC0000370.json", "BGC0000553.json"} {
		checkPaths(t, name, load(t, name))
	}
}

// TestDataset checks all entries of a MIBiG release, if MIBIG_DATA points
// to a directory of entries.
func TestDataset(t *testing.T) {
	dir := os.Getenv("MIBIG_DATA")
	if dir == "" {
		t.Skip("set MIBIG_DATA to a directory of MIBiG entries")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no entries in %s: %v", dir, err)
	}
	for _, file := range files {
		entry, err := mibig.ReadFile(file)
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		checkPaths(t, filepath.Base(file), entry)
		// a no-op raw edit of the whole entry keeps it the same
		before, _ := mibig.Marshal(entry)
		tr := Build(entry)
		data, err := tr.Root.MarshalJSON()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if err := tr.Replace(tr.Root, data); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		after, _ := mibig.Marshal(entry)
		if !bytes.Equal(before, after) {
			t.Errorf("%s: raw round trip changed the entry", file)
		}
	}
}

func TestParentPath(t *testing.T) {
	for path, want := range map[string]string{
		"":                                 "",
		"loci":                             "",
		"loci[0]":                          "loci",
		"loci[0].evidence":                 "loci[0]",
		`a["odd.key"]`:                     "a",
		`a["x[1]"]`:                        "a",
		`a.b["c"][2]`:                      `a.b["c"]`,
		"biosynthesis.modules[3].a_domain": "biosynthesis.modules[3]",
	} {
		if got := ParentPath(path); got != want {
			t.Errorf("ParentPath(%q) = %q, want %q", path, got, want)
		}
	}
	if got := JoinPath("a", "odd.key"); got != `a["odd.key"]` {
		t.Errorf("JoinPath = %s", got)
	}
}

func TestSetText(t *testing.T) {
	entry := load(t, "BGC0000370.json")
	tr := Build(entry)

	if err := tr.Node("comment").SetText("checked"); err != nil {
		t.Fatal(err)
	}
	if entry.Comment != "checked" {
		t.Errorf("comment = %q", entry.Comment)
	}

	version := tr.Node("version")
	if version.Kind != Int {
		t.Fatalf("version is a %s", version.Kind)
	}
	if err := version.SetText("x"); err == nil {
		t.Error("expected an error for a non-number")
	}
	if err := version.SetText("7"); err != nil || entry.Version != 7 {
		t.Errorf("version = %d, %v", entry.Version, err)
	}
	if err := tr.Node("compounds").SetText("x"); err == nil {
		t.Error("expected an error for setting a list from text")
	}
}

func TestOptionalBool(t *testing.T) {
	entry := &mibig.Entry{Compounds: []mibig.Compound{{Bioactivities: []mibig.Bioactivity{{}}}}}
	tr := Build(entry)
	n := tr.Node("compounds[0].bioactivities[0].observed")
	if n == nil || n.Kind != Bool || !n.Optional {
		t.Fatalf("observed = %+v", n)
	}
	if n.IsSet() {
		t.Error("unset boolean reported as set")
	}
	if err := n.SetText("false"); err != nil {
		t.Fatal(err)
	}
	if got := entry.Compounds[0].Bioactivities[0].Observed; got == nil || *got {
		t.Errorf("observed = %v", got)
	}
	tr = Build(entry)
	n = tr.Node("compounds[0].bioactivities[0].observed")
	if !n.IsSet() || n.Text() != "false" {
		t.Errorf("observed text = %q", n.Text())
	}
	if err := n.SetText(""); err != nil || entry.Compounds[0].Bioactivities[0].Observed != nil {
		t.Errorf("unsetting failed: %v", err)
	}
}

func TestBioactivityName(t *testing.T) {
	entry := &mibig.Entry{Compounds: []mibig.Compound{{Bioactivities: []mibig.Bioactivity{{}}}}}
	n := Build(entry).Node("compounds[0].bioactivities[0].name")
	if n == nil || n.Kind != String {
		t.Fatalf("name = %+v", n)
	}
	if err := n.SetText("antibacterial"); err != nil {
		t.Fatal(err)
	}
	if got := entry.Compounds[0].Bioactivities[0].Name.Activity; got != "antibacterial" {
		t.Errorf("activity = %q", got)
	}
}

func TestListOperations(t *testing.T) {
	entry := &mibig.Entry{}
	tr := Build(entry)
	classes := tr.Node("biosynthesis.classes")
	if !slices.Contains(classes.Variants(), "NRPS") {
		t.Fatalf("variants = %q", classes.Variants())
	}
	path, err := classes.Add(0, "NRPS")
	if err != nil || path != "biosynthesis.classes[0]" {
		t.Fatalf("Add = %q, %v", path, err)
	}
	if _, err := Build(entry).Node("biosynthesis.classes").Add(1, "terpene"); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(entry).Node("biosynthesis.classes").Add(0, "nope"); err == nil {
		t.Error("expected an error for an unknown variant")
	}

	tr = Build(entry)
	if n := tr.Node("biosynthesis.classes[1]"); n == nil || n.Variant != "terpene" || n.Kind != Object {
		t.Fatalf("second class = %+v", n)
	}
	if path, err := tr.Node("biosynthesis.classes[1]").Move(-1); err != nil || path != "biosynthesis.classes[0]" {
		t.Fatalf("Move = %q, %v", path, err)
	}
	if entry.Biosynthesis.Classes[0].ClassName() != mibig.ClassTerpene {
		t.Errorf("classes after move: %v", entry.Biosynthesis.Classes)
	}

	tr = Build(entry)
	if err := tr.Node("biosynthesis.classes[0]").Remove(); err != nil {
		t.Fatal(err)
	}
	if len(entry.Biosynthesis.Classes) != 1 || entry.Biosynthesis.Classes[0].ClassName() != mibig.ClassNRPS {
		t.Errorf("classes after remove: %v", entry.Biosynthesis.Classes)
	}

	// adding an element after an element
	tr = Build(entry)
	list, at := tr.Node("biosynthesis.classes[0]").AddTarget()
	if list.Path != "biosynthesis.classes" || at != 1 {
		t.Errorf("AddTarget = %s, %d", list.Path, at)
	}
}

func TestCreateOptional(t *testing.T) {
	entry := &mibig.Entry{}
	tr := Build(entry)
	genes := tr.Node("genes")
	if genes.Kind != Absent || !genes.CanCreate() || genes.CanRemove() {
		t.Fatalf("genes = %+v", genes)
	}
	if err := genes.Create(""); err != nil || entry.Genes == nil {
		t.Fatalf("Create: %v", err)
	}
	genes = Build(entry).Node("genes")
	if genes.Kind != Object || !genes.CanRemove() {
		t.Fatalf("genes after create = %+v", genes)
	}
	if err := genes.Remove(); err != nil || entry.Genes != nil {
		t.Errorf("Remove: %v", err)
	}
}

func TestReplaceVariant(t *testing.T) {
	entry := &mibig.Entry{Biosynthesis: mibig.Biosynthesis{Classes: mibig.Classes{mibig.NewClass(mibig.ClassNRPS)}}}
	tr := Build(entry)
	n := tr.Node("biosynthesis.classes[0]")
	if err := tr.Replace(n, []byte(`{"class": "PKS", "subclass": "Type I"}`)); err != nil {
		t.Fatal(err)
	}
	pks, ok := entry.Biosynthesis.Classes[0].(*mibig.PKSClass)
	if !ok || pks.Subclass != "Type I" {
		t.Errorf("class = %#v", entry.Biosynthesis.Classes[0])
	}
	if err := Build(entry).Replace(n, []byte(`{"class": `)); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestEditDiscardsPreservedNull(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"accession": "BGC0000001", "comment": null}`))
	if err != nil {
		t.Fatal(err)
	}
	n := Build(entry).Node("comment")
	if err := n.SetText(""); err != nil {
		t.Fatal(err)
	}
	data, err := mibig.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"comment"`)) {
		t.Errorf("cleared comment still written:\n%s", data)
	}
}

func TestRemoveUnknownAndEmpty(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"accession": "BGC0000001", "comment": null, "odd": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	removable := func() []string {
		var paths []string
		for _, issue := range entry.Validate() {
			if issue.Code == mibig.CodeUnknownField || issue.Code == mibig.CodeEmptyValue {
				paths = append(paths, issue.Path)
			}
		}
		return paths
	}
	paths := removable()
	if len(paths) != 2 {
		t.Fatalf("issues to fix: %q", paths)
	}
	odd := Build(entry).Node("odd")
	if odd == nil || !odd.Unknown || odd.Kind != Raw {
		t.Fatalf("unknown key node = %+v", odd)
	}
	if data, err := odd.MarshalJSON(); err != nil || string(data) != "1" {
		t.Errorf("unknown value = %s, %v", data, err)
	}
	for _, path := range paths {
		if err := Build(entry).Node(path).Remove(); err != nil {
			t.Fatalf("Remove(%s): %v", path, err)
		}
	}
	if paths := removable(); len(paths) != 0 {
		t.Errorf("still to fix: %q", paths)
	}
	data, _ := mibig.Marshal(entry)
	for _, key := range []string{"comment", "odd"} {
		if bytes.Contains(data, []byte(`"`+key+`"`)) {
			t.Errorf("%s still written:\n%s", key, data)
		}
	}
}

func TestCoreSequence(t *testing.T) {
	entry := &mibig.Entry{Biosynthesis: mibig.Biosynthesis{Classes: mibig.Classes{
		&mibig.RibosomalClass{Precursors: []mibig.RiPPPrecursor{{}}},
	}}}
	n := Build(entry).Node("biosynthesis.classes[0].precursors[0].core_sequence")
	if n == nil || n.Kind != List {
		t.Fatalf("core_sequence = %+v", n)
	}
	if v := n.Variants(); v != nil {
		t.Errorf("Variants() = %q", v)
	}
	if _, err := n.Add(0, ""); err != nil {
		t.Fatal(err)
	}
	if err := Build(entry).Node("biosynthesis.classes[0].precursors[0].core_sequence[0]").SetText("MKV"); err != nil {
		t.Fatal(err)
	}
	class := entry.Biosynthesis.Classes[0].(*mibig.RibosomalClass)
	if got := class.Precursors[0].CoreSequence.Sequences; len(got) != 1 || got[0] != "MKV" {
		t.Errorf("sequences = %q", got)
	}
}

func TestReplaceNull(t *testing.T) {
	entry := load(t, "BGC0000370.json")
	tr := Build(entry)
	if err := tr.Replace(tr.Root, []byte("null")); err == nil {
		t.Error("null accepted for the entry")
	}
	if entry.Accession != "BGC0000370" {
		t.Errorf("entry changed: accession %q", entry.Accession)
	}
	if err := tr.Replace(tr.Node("loci[0]"), []byte(" null ")); err == nil {
		t.Error("null accepted for a locus")
	}
	// like in a file, a null class decodes and is reported by validation
	if err := tr.Replace(tr.Node("biosynthesis.classes[0]"), []byte("null")); err != nil {
		t.Errorf("null class: %v", err)
	}
	if !slices.ContainsFunc(entry.Validate(), func(i mibig.Issue) bool { return i.Path == "biosynthesis.classes[0]" }) {
		t.Error("null class not reported")
	}
	tr = Build(entry)
	if err := tr.Replace(tr.Node("genes"), []byte("null")); err != nil || entry.Genes != nil {
		t.Errorf("optional genes: %v", err)
	}
}

func TestAliasPaths(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"biosynthesis": {"classes": [{"class": "NRPS", "release_type": [{"name": ""}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	want := tr.Node("biosynthesis.classes[0].release_types[0].name")
	if want == nil {
		t.Fatal("no canonical node")
	}
	if got := tr.Nearest("biosynthesis.classes[0].release_type[0].name"); got != want {
		t.Errorf("alias path resolves to %s", got.Path)
	}
	for _, issue := range entry.ValidateAt(mibig.QualityHigh) {
		if strings.Contains(issue.Path, "release_type[") && tr.Node(issue.Path) == nil {
			t.Errorf("issue path %s does not resolve exactly", issue.Path)
		}
	}
}

func TestOddKeyPaths(t *testing.T) {
	if got := JoinPath("", "<odd>&"); got != `["<odd>&"]` {
		t.Errorf("JoinPath = %s", got)
	}
	entry, err := mibig.Parse([]byte(`{"accession": "BGC0000001", "<odd>": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	found := false
	for _, issue := range entry.Validate() {
		if issue.Code == mibig.CodeUnknownField {
			found = true
			if n := tr.Node(issue.Path); n == nil || !n.Unknown {
				t.Errorf("issue path %s does not resolve to the unknown key", issue.Path)
			}
		}
	}
	if !found {
		t.Error("no unknown-field issue")
	}
}

func TestReplaceUsesEntryDecoding(t *testing.T) {
	entry := load(t, "BGC0000370.json")
	before, _ := mibig.Marshal(entry)
	for path, data := range map[string]string{
		"see_also": `[null]`,
		"loci":     `[null]`,
		"version":  `"seven"`,
	} {
		tr := Build(entry)
		if err := tr.Replace(tr.Node(path), []byte(data)); err == nil {
			t.Errorf("%s = %s accepted", path, data)
		}
	}
	if after, _ := mibig.Marshal(entry); !bytes.Equal(before, after) {
		t.Error("failed replacements changed the entry")
	}

	// absent sections can be filled in, unknown keys edited
	entry, err := mibig.Parse([]byte(`{"accession": "BGC0000001", "odd": 1, "comments": null}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	if err := tr.Replace(tr.Node("genes"), []byte(`{"to_add": []}`)); err != nil || entry.Genes == nil {
		t.Fatalf("filling in genes: %v", err)
	}
	tr = Build(entry)
	if err := tr.Replace(tr.Node("odd"), []byte(`{"x": 2}`)); err != nil {
		t.Fatal(err)
	}
	if v, _ := entry.Extra.Get("odd"); string(v) != `{"x":2}` && string(v) != `{"x": 2}` {
		t.Errorf("odd = %s", v)
	}
	data, _ := mibig.Marshal(entry)
	if !bytes.Contains(data, []byte(`"accession": "BGC0000001"`)) {
		t.Errorf("other fields lost:\n%s", data)
	}
}

func TestRepeatedUnknownKeys(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"accession": "BGC0000001", "odd": 1, "odd": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	first, second := tr.Node("odd"), tr.Node("odd#2")
	if first == nil || second == nil || first == second {
		t.Fatalf("nodes: %v, %v", first, second)
	}
	if err := tr.Replace(second, []byte("3")); err != nil {
		t.Fatal(err)
	}
	data, _ := mibig.Marshal(entry)
	if !bytes.Contains(data, []byte(`"odd": 1`)) || !bytes.Contains(data, []byte(`"odd": 3`)) {
		t.Errorf("after replacing the second key:\n%s", data)
	}
	if err := Build(entry).Node("odd#2").Remove(); err != nil {
		t.Fatal(err)
	}
	if got := entry.Extra.Unknown; len(got) != 1 || string(got[0].Value) != "1" {
		t.Errorf("after removing the second key: %v", got)
	}
}

func TestReplaceCoreSequenceString(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"biosynthesis": {"classes": [{"class": "ribosomal", "precursors": [{"core_sequence": "MKV"}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	n := tr.Node("biosynthesis.classes[0].precursors[0].core_sequence[0]")
	if n == nil {
		t.Fatal("no sequence node")
	}
	if err := tr.Replace(n, []byte(`"AAA"`)); err != nil {
		t.Fatal(err)
	}
	class := entry.Biosynthesis.Classes[0].(*mibig.RibosomalClass)
	if seq := class.Precursors[0].CoreSequence; len(seq.Sequences) != 1 || seq.Sequences[0] != "AAA" || seq.Legacy {
		t.Errorf("core sequence = %+v", seq)
	}
}

func TestReplaceKeepsSiblings(t *testing.T) {
	entry, err := mibig.Parse([]byte(`{"loci": [{"accession": "A"}, {"accession": "B", "x": "<y>&"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	tr := Build(entry)
	if err := tr.Replace(tr.Node("loci[0].accession"), []byte(`"C"`)); err != nil {
		t.Fatal(err)
	}
	if v, _ := entry.Loci[1].Extra.Get("x"); string(v) != `"<y>&"` {
		t.Errorf("sibling's unknown value changed to %s", v)
	}
}

func TestReplaceOptionalInterfaceWithNull(t *testing.T) {
	entry := &mibig.Entry{Biosynthesis: mibig.Biosynthesis{Modules: mibig.Modules{
		&mibig.CALModule{CAL: mibig.NewDomain(mibig.DomainAMPBinding)},
	}}}
	tr := Build(entry)
	if err := tr.Replace(tr.Node("biosynthesis.modules[0].cal"), []byte("null")); err != nil {
		t.Fatal(err)
	}
	if m := entry.Biosynthesis.Modules[0].(*mibig.CALModule); m.CAL != nil {
		t.Errorf("cal = %#v", m.CAL)
	}
}

func TestParentPathOccurrence(t *testing.T) {
	for path, want := range map[string]string{
		"odd#2":              "",
		`genes["odd.key"]#2`: "genes",
		"loci[0].x#3":        "loci[0]",
		`a["x#2"]`:           "a",
	} {
		if got := ParentPath(path); got != want {
			t.Errorf("ParentPath(%q) = %q, want %q", path, got, want)
		}
	}
}
