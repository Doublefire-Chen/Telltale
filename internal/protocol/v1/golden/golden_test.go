package golden_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	v1 "github.com/Doublefire-Chen/Telltale/internal/protocol/v1"
	"github.com/Doublefire-Chen/Telltale/internal/protocol/v1/golden"
)

func corpus(t *testing.T) []golden.Payload {
	t.Helper()
	all, err := golden.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("golden corpus is empty")
	}
	return all
}

func target(p golden.Payload) any {
	if p.Path == v1.ReportPath {
		return &v1.Report{}
	}
	return &v1.HostInfo{}
}

// TestFieldsAreNeverRemovedOrRenamed decodes every recorded payload strictly:
// a field that no longer exists, or changed type, fails.
func TestFieldsAreNeverRemovedOrRenamed(t *testing.T) {
	for _, p := range corpus(t) {
		dec := json.NewDecoder(bytes.NewReader(p.Body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(target(p)); err != nil {
			t.Errorf("%s %s no longer decodes into protocol v1: %v", p.Version, p.Path, err)
		}
	}
}

// TestEveryFieldIsRecorded fails when the Agent can send a field that no
// golden payload contains: record it before releasing.
func TestEveryFieldIsRecorded(t *testing.T) {
	seen := map[string]map[string]bool{v1.ReportPath: {}, v1.HostInfoPath: {}}
	for _, p := range corpus(t) {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(p.Body, &fields); err != nil {
			t.Fatalf("%s %s: %v", p.Version, p.Path, err)
		}
		for k := range fields {
			seen[p.Path][k] = true
		}
	}
	for path, typ := range map[string]reflect.Type{
		v1.ReportPath: reflect.TypeFor[v1.Report](), v1.HostInfoPath: reflect.TypeFor[v1.HostInfo](),
	} {
		var missing []string
		for i := range typ.NumField() {
			name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			if name != "" && name != "-" && !seen[path][name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s fields missing from every golden payload: %v (add them to the newest version's file)", path, missing)
		}
	}
}
