package provider

import (
	"encoding/base64"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Conversions between framework optional attributes and the SDK's pointer/scalar
// fields. Null/unknown framework values map to nil pointers.

func int64ToFloatPtr(v types.Int64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	f := float64(v.ValueInt64())
	return &f
}

func int64ToIntPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := int(v.ValueInt64())
	return &i
}

func intPtrToInt64(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func floatPtrToInt64(p *float64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func stringPtrToString(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// stringToPtr converts an optional framework string to *string (nil when null/unknown).
func stringToPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// base64Ptr encodes an optional framework string to base64 for API fields that
// require it (e.g. VM userdata), nil when null/unknown.
func base64Ptr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := base64.StdEncoding.EncodeToString([]byte(v.ValueString()))
	return &s
}

// timePtrToString renders an optional timestamp as an RFC 3339 framework string.
func timePtrToString(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}

// keepStr returns the existing state value on a normal refresh (when it is
// already known), and the freshly-read API value otherwise. The "otherwise"
// case is `terraform import`, which seeds only the id — so config attributes
// must be hydrated from the API to make the import round-trip clean. Preferring
// state on a normal refresh keeps the user's own representation, so a value the
// API echoes back differently (e.g. a normalized label) never shows as drift.
// An empty apiVal maps to null so unset optional attributes stay null.
func keepStr(state types.String, apiVal string) types.String {
	if !state.IsNull() && !state.IsUnknown() && state.ValueString() != "" {
		return state
	}
	if apiVal == "" {
		return types.StringNull()
	}
	return types.StringValue(apiVal)
}

// keepInt is keepStr for required Int64 config attributes (see keepStr).
func keepInt(state types.Int64, apiVal int64) types.Int64 {
	if !state.IsNull() && !state.IsUnknown() {
		return state
	}
	return types.Int64Value(apiVal)
}

// keepIntPtr is keepStr for optional Int64 config attributes backed by an SDK
// *int (see keepStr): keep the user's value on a normal refresh, otherwise
// hydrate from the API on import. A nil API value maps to null so an unset
// optional attribute stays null.
func keepIntPtr(state types.Int64, api *int) types.Int64 {
	if !state.IsNull() && !state.IsUnknown() {
		return state
	}
	return intPtrToInt64(api)
}

// derefString returns the pointed-to string, or "" when nil — for keepStr-ing
// SDK response fields the API now models as optional (*string).
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
