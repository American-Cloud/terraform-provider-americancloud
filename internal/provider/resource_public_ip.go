package provider

import (
	"context"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                     = (*publicIpResource)(nil)
	_ resource.ResourceWithConfigure        = (*publicIpResource)(nil)
	_ resource.ResourceWithImportState      = (*publicIpResource)(nil)
	_ resource.ResourceWithConfigValidators = (*publicIpResource)(nil)
)

// NewPublicIPResource — sdkRef: publicIps.ReservePublicIps / GetPublicIps / ReleasePublicIps.
// NOT_EXPOSED: publicIps.ChangeSourceNatIPPublicIps / EnableStaticNatPublicIps /
// DisableStaticNatPublicIps (NAT ops, deferred to v0.2); ListPublicIps /
// ListByIsolatedNetworkPublicIps / ListByVpcPublicIps (data-source surface);
// GetCostEstimatePublicIps (preview).
func NewPublicIPResource() resource.Resource { return &publicIpResource{} }

type publicIpResource struct{ baseResource }

type publicIpModel struct {
	ID        types.String `tfsdk:"id"`
	NetworkID types.String `tfsdk:"network_id"`
	VpcID     types.String `tfsdk:"vpc_id"`
	Region    types.String `tfsdk:"region"`
	IPAddress types.String `tfsdk:"ip_address"`
	Status    types.String `tfsdk:"status"`
}

func (r *publicIpResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_public_ip"
}

// ConfigValidators enforces that exactly one of network_id / vpc_id is set — an
// IP is reserved in either an isolated network or a VPC, never both or neither.
func (r *publicIpResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("network_id"),
			path.MatchRoot("vpc_id"),
		),
	}
}

func (r *publicIpResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A reserved public IP address. Reserve it in an isolated network (`network_id`) or a VPC (`vpc_id`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Public IP identifier (UUID).", PlanModifiers: useState},
			"network_id": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Isolated network UUID to reserve the IP in (set this or `vpc_id`). Forces a new IP.",
				PlanModifiers: requiresReplace,
			},
			"vpc_id": schema.StringAttribute{
				Optional: true, MarkdownDescription: "VPC UUID to reserve the IP in (set this or `network_id`). Forces a new IP.",
				PlanModifiers: requiresReplace,
			},
			"region": schema.StringAttribute{
				Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`. Changing this forces a new IP.",
				PlanModifiers: requiresReplace,
			},
			"ip_address": schema.StringAttribute{Computed: true, MarkdownDescription: "The allocated IP address.", PlanModifiers: useState},
			"status":     schema.StringAttribute{Computed: true, MarkdownDescription: "Current status.", PlanModifiers: useState},
		},
	}
}

func (r *publicIpResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan publicIpModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip, err := r.client.PublicIps.ReservePublicIps(ctx, &acsdk.ReservePublicIPDto{
		NetworkID: stringToPtr(plan.NetworkID),
		VpcID:     stringToPtr(plan.VpcID),
		Region:    plan.Region.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reserving public IP", err.Error())
		return
	}

	plan.ID = types.StringValue(ip.ID)
	plan.IPAddress = types.StringValue(ip.IPAddress)
	plan.Status = types.StringValue(ip.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *publicIpResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state publicIpModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip, err := r.client.PublicIps.GetPublicIps(ctx, &acsdk.GetPublicIpsRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading public IP", err.Error())
		return
	}

	// region is Required, so it's null only on `terraform import` — hydrate then,
	// preserve the user's value otherwise.
	state.Region = keepStr(state.Region, ip.Region)
	// As of API 1.3.0 the response carries vpcId, so the VPC-vs-isolated
	// association round-trips on import: hydrate whichever side the API reports
	// (the other stays null, satisfying the ExactlyOneOf rule). keepStr preserves
	// the user's value on a normal refresh, so neither shows phantom drift.
	if ip.VpcID != nil && *ip.VpcID != "" {
		state.VpcID = keepStr(state.VpcID, *ip.VpcID)
	} else {
		// The user supplies the IP's *associated* network as network_id, which
		// the API echoes back as `associatedNetworkId`. The response's
		// `networkId` is CloudStack's internal/source-NAT network id and never
		// equals the configured value — hydrating from it made `terraform
		// import` of an isolated-network IP propose a spurious ForceNew
		// replacement. keepStr still preserves the create-time value on refresh.
		state.NetworkID = keepStr(state.NetworkID, derefString(ip.AssociatedNetworkID))
	}
	// Computed fields always refresh from the API.
	state.IPAddress = types.StringValue(ip.IPAddress)
	state.Status = types.StringValue(ip.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement (NAT ops are deferred).
func (r *publicIpResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[publicIpModel](ctx, req, resp)
}

func (r *publicIpResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state publicIpModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PublicIps.ReleasePublicIps(ctx, &acsdk.ReleasePublicIpsRequest{ID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error releasing public IP", err.Error())
	}
}

func (r *publicIpResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
