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
	_ resource.Resource                = (*isolatedNetworkResource)(nil)
	_ resource.ResourceWithConfigure   = (*isolatedNetworkResource)(nil)
	_ resource.ResourceWithImportState = (*isolatedNetworkResource)(nil)
)

// NewIsolatedNetworkResource — sdkRef: isolatedNetworks.CreateIsolatedNetworks /
// GetIsolatedNetworks / UpdateIsolatedNetworks / DeleteIsolatedNetworks.
// NOT_EXPOSED: isolatedNetworks.RestartIsolatedNetworks (imperative op);
// isolatedNetworks.ListIsolatedNetworks (data-source surface).
func NewIsolatedNetworkResource() resource.Resource { return &isolatedNetworkResource{} }

type isolatedNetworkResource struct{ baseResource }

type isolatedNetworkModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Region      types.String `tfsdk:"region"`
	Netmask     types.String `tfsdk:"netmask"`
	Gateway     types.String `tfsdk:"gateway"`
	Cidr        types.String `tfsdk:"cidr"`
	Status      types.String `tfsdk:"status"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *isolatedNetworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_isolated_network"
}

func (r *isolatedNetworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An isolated (single-tier) private network. `name` and `description` are updatable in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "Network identifier (UUID).", PlanModifiers: useState,
			},
			"name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Network name.",
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Optional description.", PlanModifiers: useState,
			},
			"region": schema.StringAttribute{
				Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`. Changing this forces a new network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"netmask": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Netmask, e.g. `255.255.255.0`. Omit for the default. Forces a new network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"gateway": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Gateway IP. Omit for the default. Forces a new network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"cidr":       schema.StringAttribute{Computed: true, MarkdownDescription: "Network CIDR.", PlanModifiers: useState},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Network status.", PlanModifiers: useState},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time (RFC 3339).", PlanModifiers: useState},
		},
	}
}

func (r *isolatedNetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan isolatedNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	net, err := r.client.IsolatedNetworks.CreateIsolatedNetworks(ctx, &acsdk.CreateIsolatedNetworkDto{
		Name:        plan.Name.ValueString(),
		Description: stringToPtr(plan.Description),
		Region:      plan.Region.ValueString(),
		Netmask:     stringToPtr(plan.Netmask),
		Gateway:     stringToPtr(plan.Gateway),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating isolated network", err.Error())
		return
	}

	state := isolatedNetworkState(net.ID, net.Name, net.Description, net.Region, net.Cidr, net.Gateway, string(net.Status), net.CreatedAt, plan.Netmask)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *isolatedNetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state isolatedNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	net, err := r.client.IsolatedNetworks.GetIsolatedNetworks(ctx, &acsdk.GetIsolatedNetworksRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading isolated network", err.Error())
		return
	}

	// netmask is an input-only field (not returned) — preserve it from state.
	model := isolatedNetworkState(net.ID, net.Name, net.Description, net.Region, net.Cidr, net.Gateway, string(net.Status), net.CreatedAt, state.Netmask)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *isolatedNetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state isolatedNetworkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.IsolatedNetworks.UpdateIsolatedNetworks(ctx, &acsdk.UpdateIsolatedNetworkDto{
		ID:          state.ID.ValueString(),
		Name:        stringToPtr(plan.Name),
		Description: stringToPtr(plan.Description),
	}); err != nil {
		resp.Diagnostics.AddError("Error updating isolated network", err.Error())
		return
	}
	// Persist the plan (name/description are the only mutable fields); computed
	// fields carry through via UseStateForUnknown and reconcile on the next Read.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *isolatedNetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state isolatedNetworkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Retried: the delete races the NIC release of a VM deleted in the same
	// destroy — see retryTransientDelete.
	if err := retryTransientDelete(ctx, func(ctx context.Context) error {
		return r.client.IsolatedNetworks.DeleteIsolatedNetworks(ctx, &acsdk.DeleteIsolatedNetworksRequest{ID: state.ID.ValueString()})
	}); err != nil {
		resp.Diagnostics.AddError("Error deleting isolated network", err.Error())
	}
}

func (r *isolatedNetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func isolatedNetworkState(id, name, description, region, cidr, gateway, status string, createdAt *time.Time, netmask types.String) isolatedNetworkModel {
	return isolatedNetworkModel{
		ID:          types.StringValue(id),
		Name:        types.StringValue(name),
		Description: types.StringValue(description),
		Region:      types.StringValue(region),
		Netmask:     netmask,
		Gateway:     types.StringValue(gateway),
		Cidr:        types.StringValue(cidr),
		Status:      types.StringValue(status),
		CreatedAt:   timePtrToString(createdAt),
	}
}
