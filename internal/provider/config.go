package provider

import (
	"context"
	"fmt"
	"strings"

	acclient "github.com/American-Cloud/americancloud-sdk-go/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// baseResource supplies the SDK client and a Configure method to every resource
// via embedding, so each resource only declares its schema and CRUD.
type baseResource struct {
	client *acclient.Client
}

func (b *baseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	b.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// baseDataSource is the data-source analogue of baseResource.
type baseDataSource struct {
	client *acclient.Client
}

func (b *baseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// setPlanToState persists the plan unchanged — the Update body for resources whose
// every editable attribute forces replacement, so Update is never reached for a
// real change. Generic over the resource's model type.
func setPlanToState[T any](ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan T
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// clientFromProviderData extracts the configured SDK client that the provider's
// Configure stashed in ProviderData. Returns nil (without error) when ProviderData
// is nil — the framework calls Configure before the provider is configured.
func clientFromProviderData(providerData any, diags *diag.Diagnostics) *acclient.Client {
	if providerData == nil {
		return nil
	}
	client, ok := providerData.(*acclient.Client)
	if !ok {
		diags.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T. This is a bug in the provider; please report it.", providerData),
		)
		return nil
	}
	return client
}

// importCompositeID splits a "/"-delimited import id across the given attribute
// paths and sets each on state, erroring with the expected form if the shape is
// wrong. Used by resources whose identity is a composite key (e.g. firewall_rule's
// ipId/ruleId, vpc_tier's vpcId/tierId). The final segment absorbs any extra
// slashes (SplitN semantics), so the last attribute may contain "/".
func importCompositeID(ctx context.Context, resp *resource.ImportStateResponse, id, expected string, paths ...path.Path) {
	parts := strings.SplitN(id, "/", len(paths))
	if len(parts) != len(paths) {
		resp.Diagnostics.AddError("Invalid import ID", "Expected `"+expected+"`.")
		return
	}
	for i, p := range paths {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, p, parts[i])...)
	}
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
