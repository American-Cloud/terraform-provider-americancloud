package provider

import (
	"context"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*networkACLResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkACLResource)(nil)
	_ resource.ResourceWithImportState = (*networkACLResource)(nil)
)

// NewNetworkACLResource — sdkRef: networkACLs.CreateListNetworkACLs /
// GetListNetworkACLs / DeleteListNetworkACLs.
// NOT_EXPOSED: networkACLs.ListListsNetworkACLs / ListListsByVpcNetworkACLs —
// Read uses get-by-id; list surfaces are data-source candidates (roadmap).
// NOT_EXPOSED: networkACLs.ReplaceListNetworkACLs — attaching an ACL to a tier
// is modeled on `americancloud_vpc_tier.acl_id`; in-place re-assignment of a
// tier's ACL is a vpc_tier roadmap item, not an ACL-side operation.
func NewNetworkACLResource() resource.Resource { return &networkACLResource{} }

type networkACLResource struct{ baseResource }

type networkACLModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	VpcID       types.String `tfsdk:"vpc_id"`
}

func (r *networkACLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_acl"
}

func (r *networkACLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A network ACL list on a VPC — the traffic-policy container for VPC tiers " +
			"(isolated networks use `americancloud_egress_rule` / `americancloud_firewall_rule` instead). " +
			"Attach it to a tier via `americancloud_vpc_tier.acl_id`; add rules with " +
			"`americancloud_network_acl_rule`. Deleting the list deletes its rules. " +
			"Immutable — any change replaces the list.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "ACL list identifier (UUID).", PlanModifiers: useState},
			"name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Human-readable name for the ACL list.",
				PlanModifiers: requiresReplace,
			},
			"description": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Free-form description of the ACL list.",
				PlanModifiers: requiresReplace,
			},
			"vpc_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "VPC identifier (from `americancloud_vpc_network`) the ACL list belongs to.",
				PlanModifiers: requiresReplace,
			},
		},
	}
}

func (r *networkACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkACLModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := r.client.NetworkACLs.CreateListNetworkACLs(ctx, &acsdk.CreateNetworkACLListDto{
		Name:        plan.Name.ValueString(),
		Description: stringToPtr(plan.Description),
		VpcID:       plan.VpcID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating network ACL list", err.Error())
		return
	}

	plan.ID = types.StringValue(acl.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkACLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	acl, err := r.client.NetworkACLs.GetListNetworkACLs(ctx, &acsdk.GetListNetworkACLsRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading network ACL list", err.Error())
		return
	}

	// Config fields are immutable (ForceNew); keepStr preserves user values on a
	// normal refresh and hydrates them on import (ImportState seeds only id).
	state.Name = keepStr(state.Name, acl.Name)
	if acl.Description != nil {
		state.Description = keepStr(state.Description, *acl.Description)
	}
	if acl.VpcID != nil {
		state.VpcID = keepStr(state.VpcID, *acl.VpcID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement.
func (r *networkACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[networkACLModel](ctx, req, resp)
}

func (r *networkACLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkACLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A list still attached to a tier in the same destroy can briefly 409/504
	// while the tier releases it — same eventual-consistency window as
	// network/tier deletes.
	err := retryTransientDelete(ctx, func(ctx context.Context) error {
		return r.client.NetworkACLs.DeleteListNetworkACLs(ctx, &acsdk.DeleteListNetworkACLsRequest{ID: state.ID.ValueString()})
	})
	if err != nil {
		resp.Diagnostics.AddError("Error deleting network ACL list", err.Error())
	}
}

func (r *networkACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
