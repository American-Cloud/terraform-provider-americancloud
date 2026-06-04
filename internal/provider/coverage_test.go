package provider

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	acclient "github.com/American-Cloud/americancloud-sdk-go/client"
	"github.com/American-Cloud/americancloud-sdk-go/option"
)

// TestSDKCoverage asserts every public method of each covered SDK namespace is
// accounted for — mapped to a resource/data source or listed in notExposed with
// a reason. SDK additions/renames on a version bump surface here as a failure.
func TestSDKCoverage(t *testing.T) {
	client := acclient.NewClient(option.WithAPIKey("coverage-dummy"), option.WithAPIClientSecret("coverage-dummy"))
	cv := reflect.ValueOf(client).Elem()

	var problems []string

	for _, ns := range coveredNamespaces {
		field := cv.FieldByName(ns)
		if !field.IsValid() {
			problems = append(problems, "no such SDK client namespace field: "+ns)
			continue
		}
		nsType := field.Type() // *<ns>.Client
		for i := 0; i < nsType.NumMethod(); i++ {
			method := nsType.Method(i).Name
			if method == "WithRawResponse" {
				continue // infra accessor, not an API call
			}
			key := ns + "." + method
			if !mapped[key] && notExposed[key] == "" {
				problems = append(problems, "unaccounted SDK method: "+key+" (map it to a resource or add a notExposed reason)")
			}
		}
	}

	// Reverse: every mapped / notExposed key must resolve to a real method —
	// catches renames/removals on SDK bumps and typos in the manifest.
	known := map[string]reflect.Type{}
	for _, ns := range coveredNamespaces {
		if f := cv.FieldByName(ns); f.IsValid() {
			known[ns] = f.Type()
		}
	}
	checkResolves := func(key string) {
		ns, method, ok := strings.Cut(key, ".")
		t, found := known[ns]
		if !ok || !found {
			problems = append(problems, "manifest references unknown namespace: "+key)
			return
		}
		if _, has := t.MethodByName(method); !has {
			problems = append(problems, "manifest references nonexistent method: "+key)
		}
	}
	for k := range mapped {
		checkResolves(k)
	}
	for k := range notExposed {
		checkResolves(k)
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("SDK coverage problems (%d):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}
