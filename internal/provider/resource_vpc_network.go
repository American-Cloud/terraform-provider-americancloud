package provider

import (
	"context"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*vpcNetworkResource)(nil)
	_ resource.ResourceWithConfigure   = (*vpcNetworkResource)(nil)
	_ resource.ResourceWithImportState = (*vpcNetworkResource)(nil)
)

// NewVPCNetworkResource — sdkRef: vpcNetworks.CreateVpcNetworks / GetVpcNetworks /
// UpdateVpcNetworks / DeleteVpcNetworks.
// NOT_EXPOSED: vpcNetworks.CreateTierVpcNetworks (tiers deferred — no delete-tier
// endpoint); RestartVpcNetworks (imperative); GetCostEstimateVpcNetworks
// (preview); ListVpcNetworks (data-source surface).
func NewVPCNetworkResource() resource.Resource { return &vpcNetworkResource{} }

type vpcNetworkResource struct{ baseResource }

type vpcNetworkModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Region      types.String `tfsdk:"region"`
	Cidr        types.String `tfsdk:"cidr"`
	Status      types.String `tfsdk:"status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *vpcNetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc_network"
}

func (r *vpcNetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Virtual Private Cloud network. `name` and `description` are updatable in place. " +
			"Tier subnets are managed outside this resource for now (see provider docs).",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "VPC identifier (UUID).", PlanModifiers: useState},
			"name":        schema.StringAttribute{Required: true, MarkdownDescription: "VPC name."},
			"description": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Optional description.", PlanModifiers: useState},
			"region": schema.StringAttribute{
				Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`. Changing this forces a new VPC.",
				PlanModifiers: requiresReplace,
			},
			"cidr": schema.StringAttribute{
				Required: true, MarkdownDescription: "VPC CIDR block, e.g. `10.0.0.0/16`. Changing this forces a new VPC.",
				PlanModifiers: requiresReplace,
			},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "VPC status.", PlanModifiers: useState},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time (RFC 3339).", PlanModifiers: useState},
		},
	}
}

func (r *vpcNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vpcNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpc, err := r.client.VpcNetworks.CreateVpcNetworks(ctx, &acsdk.CreateVpcNetworkDto{
		Name:        plan.Name.ValueString(),
		Description: stringToPtr(plan.Description),
		Region:      plan.Region.ValueString(),
		Cidr:        plan.Cidr.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating VPC network", err.Error())
		return
	}

	state := vpcNetworkState(vpc.ID, vpc.Name, vpc.Description, vpc.Cidr, vpc.Region, string(vpc.Status), vpc.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vpcNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vpcNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpc, err := r.client.VpcNetworks.GetVpcNetworks(ctx, &acsdk.GetVpcNetworksRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VPC network", err.Error())
		return
	}

	model := vpcNetworkState(vpc.ID, vpc.Name, vpc.Description, vpc.Cidr, vpc.Region, string(vpc.Status), vpc.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *vpcNetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vpcNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.VpcNetworks.UpdateVpcNetworks(ctx, &acsdk.UpdateVpcNetworkDto{
		ID:          state.ID.ValueString(),
		Name:        stringToPtr(plan.Name),
		Description: stringToPtr(plan.Description),
	}); err != nil {
		resp.Diagnostics.AddError("Error updating VPC network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vpcNetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vpcNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Retried: the delete races a just-deleted tier (and its VMs' NICs)
	// releasing their hold on the VPC — see retryTransientDelete.
	if err := retryTransientDelete(ctx, func(ctx context.Context) error {
		return r.client.VpcNetworks.DeleteVpcNetworks(ctx, &acsdk.DeleteVpcNetworksRequest{ID: state.ID.ValueString()})
	}); err != nil {
		resp.Diagnostics.AddError("Error deleting VPC network", err.Error())
	}
}

func (r *vpcNetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func vpcNetworkState(id, name, description, cidr, region, status string, createdAt *time.Time) vpcNetworkModel {
	return vpcNetworkModel{
		ID:          types.StringValue(id),
		Name:        types.StringValue(name),
		Description: types.StringValue(description),
		Region:      types.StringValue(region),
		Cidr:        types.StringValue(cidr),
		Status:      types.StringValue(status),
		CreatedAt:   timePtrToString(createdAt),
	}
}
