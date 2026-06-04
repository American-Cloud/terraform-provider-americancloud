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
	_ resource.Resource                = (*vpcTierResource)(nil)
	_ resource.ResourceWithConfigure   = (*vpcTierResource)(nil)
	_ resource.ResourceWithImportState = (*vpcTierResource)(nil)
)

// NewVPCTierResource — sdkRef: vpcNetworks.CreateTierVpcNetworks / GetTierVpcNetworks /
// UpdateTierVpcNetworks / DeleteTierVpcNetworks. NOT_EXPOSED: vpcNetworks.RestartTierVpcNetworks
// (imperative op). UpdateTier handles name/description in place; gateway/netmask/acl_id/vpc_id
// force replacement.
func NewVPCTierResource() resource.Resource { return &vpcTierResource{} }

type vpcTierResource struct{ baseResource }

type vpcTierModel struct {
	ID          types.String `tfsdk:"id"`
	VpcID       types.String `tfsdk:"vpc_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Gateway     types.String `tfsdk:"gateway"`
	Netmask     types.String `tfsdk:"netmask"`
	AclID       types.String `tfsdk:"acl_id"`
	Cidr        types.String `tfsdk:"cidr"`
	Status      types.String `tfsdk:"status"`
}

func (r *vpcTierResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc_tier"
}

func (r *vpcTierResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A network tier (subnet) within a VPC. `name` and `description` are updatable in " +
			"place; gateway, netmask, acl_id, and vpc_id are immutable (changing them replaces the tier).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Tier identifier (UUID).", PlanModifiers: useState},
			"vpc_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Parent VPC identifier (from `americancloud_vpc_network`).",
				PlanModifiers: requiresReplace,
			},
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "Tier name. Updatable in place."},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Optional description. Updatable in place.", PlanModifiers: useState,
			},
			"gateway": schema.StringAttribute{
				Required: true, MarkdownDescription: "Gateway IP for the tier, e.g. `10.0.1.1`. Forces replacement.", PlanModifiers: requiresReplace,
			},
			"netmask": schema.StringAttribute{
				Required: true, MarkdownDescription: "Netmask for the tier, e.g. `255.255.255.0`. Forces replacement.", PlanModifiers: requiresReplace,
			},
			"acl_id": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Network ACL identifier to apply to the tier. Omit for the default. Forces replacement.", PlanModifiers: requiresReplace,
			},
			"cidr":   schema.StringAttribute{Computed: true, MarkdownDescription: "Tier CIDR (derived from gateway + netmask).", PlanModifiers: useState},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Tier status.", PlanModifiers: useState},
		},
	}
}

func (r *vpcTierResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vpcTierModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tier, err := r.client.VpcNetworks.CreateTierVpcNetworks(ctx, &acsdk.CreateVpcTierDto{
		Name:        plan.Name.ValueString(),
		Description: stringToPtr(plan.Description),
		VpcID:       plan.VpcID.ValueString(),
		Gateway:     plan.Gateway.ValueString(),
		Netmask:     plan.Netmask.ValueString(),
		AclId:       stringToPtr(plan.AclID),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating VPC tier", err.Error())
		return
	}

	// Set id + computed fields from the response. description is Optional+Computed
	// (the API defaults it to the tier name when none is supplied), so it must be
	// concrete after apply — leaving the planned unknown trips "invalid result
	// object after apply". The remaining config fields carry from the plan.
	plan.ID = types.StringValue(tier.ID)
	plan.Description = types.StringValue(tier.Description)
	plan.Cidr = types.StringValue(tier.Cidr)
	plan.Status = types.StringValue(string(tier.Status))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vpcTierResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vpcTierModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the tier directly (API 1.3.0 added get-tier-by-id) rather than paging the
	// parent VPC's tier list.
	tier, err := r.client.VpcNetworks.GetTierVpcNetworks(ctx, &acsdk.GetTierVpcNetworksRequest{
		ID:     state.VpcID.ValueString(),
		TierID: state.ID.ValueString(),
	})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx) // tier (or its VPC) gone — let Terraform recreate
			return
		}
		resp.Diagnostics.AddError("Error reading VPC tier", err.Error())
		return
	}

	// name + description are updatable in place (UpdateTier) — hydrate the live
	// value (drift detection + import). gateway/netmask/acl_id are immutable
	// (ForceNew): keepStr preserves the user's value on a normal refresh and
	// hydrates on import; acl_id round-trips via the 1.3.0 detail response's aclId
	// (optional, nil when unset, so drift-safe). cidr/status are computed.
	state.Name = types.StringValue(tier.Name)
	state.Description = types.StringValue(tier.Description)
	state.Gateway = keepStr(state.Gateway, tier.Gateway)
	state.Netmask = keepStr(state.Netmask, tier.Netmask)
	state.AclID = keepStr(state.AclID, derefString(tier.AclId))
	state.Cidr = types.StringValue(tier.Cidr)
	state.Status = types.StringValue(string(tier.Status))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place name/description changes via UpdateTierVpcNetworks;
// gateway/netmask/acl_id/vpc_id have no update path and force replacement instead.
func (r *vpcTierResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vpcTierModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.VpcNetworks.UpdateTierVpcNetworks(ctx, &acsdk.UpdateVpcTierDto{
		ID:          state.VpcID.ValueString(),
		TierID:      state.ID.ValueString(),
		Name:        stringToPtr(plan.Name),
		Description: stringToPtr(plan.Description),
	}); err != nil {
		resp.Diagnostics.AddError("Error updating VPC tier", err.Error())
		return
	}
	// name/description are the only mutable fields; persist the plan. Computed
	// fields carry via UseStateForUnknown and reconcile on the next Read.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vpcTierResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vpcTierModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Retried: the delete races the NIC release of a VM deleted in the same
	// destroy — see retryTransientDelete.
	if err := retryTransientDelete(ctx, func(ctx context.Context) error {
		return r.client.VpcNetworks.DeleteTierVpcNetworks(ctx, &acsdk.DeleteTierVpcNetworksRequest{
			ID:     state.VpcID.ValueString(),
			TierID: state.ID.ValueString(),
		})
	}); err != nil {
		resp.Diagnostics.AddError("Error deleting VPC tier", err.Error())
	}
}

func (r *vpcTierResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Composite id: vpcId/tierId (Read looks up the tier within its parent VPC).
	importCompositeID(ctx, resp, req.ID, "vpcId/tierId", path.Root("vpc_id"), path.Root("id"))
}
