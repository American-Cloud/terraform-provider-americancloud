package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Helpers for optional set attributes (e.g. a rule's managed membership list).

// setToStrings extracts a string slice from an optional set attribute (nil when
// null/unknown).
func setToStrings(ctx context.Context, s types.Set, diags *diag.Diagnostics) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(s.ElementsAs(ctx, &out, false)...)
	return out
}

// diffStringSets returns the elements added (in next, not prev) and removed
// (in prev, not next).
func diffStringSets(prev, next []string) (added, removed []string) {
	prevSet := make(map[string]struct{}, len(prev))
	for _, v := range prev {
		prevSet[v] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(next))
	for _, v := range next {
		nextSet[v] = struct{}{}
	}
	for _, v := range next {
		if _, ok := prevSet[v]; !ok {
			added = append(added, v)
		}
	}
	for _, v := range prev {
		if _, ok := nextSet[v]; !ok {
			removed = append(removed, v)
		}
	}
	return added, removed
}
