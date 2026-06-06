package provider

import (
	"context"
	"strings"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*egressRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*egressRuleResource)(nil)
	_ resource.ResourceWithImportState = (*egressRuleResource)(nil)
)

var egressProtocols = []string{"TCP", "UDP", "ICMP", "ALL"}

// NewEgressRuleResource — sdkRef: egressRules.CreateEgressRules / GetEgressRules /
// DeleteEgressRules.
// NOT_EXPOSED: egressRules.UpdateEgressRules — the platform implements update as
// delete+recreate and the response carries a NEW rule id, which breaks
// Terraform's stable-id model; the resource is replace-on-change instead.
// NOT_EXPOSED: egressRules.ListEgressRules / ListByNetworkEgressRules — Read
// uses get-by-id; list surfaces are data-source candidates (roadmap).
func NewEgressRuleResource() resource.Resource { return &egressRuleResource{} }

type egressRuleResource struct{ baseResource }

type egressRuleModel struct {
	ID             types.String `tfsdk:"id"`
	NetworkID      types.String `tfsdk:"network_id"`
	Protocol       types.String `tfsdk:"protocol"`
	StartPort      types.Int64  `tfsdk:"start_port"`
	EndPort        types.Int64  `tfsdk:"end_port"`
	SourceCidrList types.String `tfsdk:"source_cidr_list"`
	DestCidrList   types.String `tfsdk:"dest_cidr_list"`
	State          types.String `tfsdk:"state"`
}

func (r *egressRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress_rule"
}

func (r *egressRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An egress firewall rule on an **isolated network** (VPC tiers use " +
			"`americancloud_network_acl` instead). `source_cidr_list` is the *source* of the " +
			"outbound traffic and must fall within the network's CIDR — it is not the destination; " +
			"use `dest_cidr_list` to scope where the traffic may go. Immutable — any change replaces the rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Rule identifier (UUID).", PlanModifiers: useState},
			"network_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Isolated network identifier (from `americancloud_isolated_network`) the rule applies to.",
				PlanModifiers: requiresReplace,
			},
			"protocol": schema.StringAttribute{
				Required: true, MarkdownDescription: "Protocol: one of TCP, UDP, ICMP, ALL.",
				Validators:    []validator.String{stringvalidator.OneOf(egressProtocols...)},
				PlanModifiers: requiresReplace,
			},
			"start_port": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Start of the port range (1–65535). Omit for ICMP/ALL.",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"end_port": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "End of the port range (1–65535). Omit for ICMP/ALL.",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"source_cidr_list": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Source CIDR of the outbound traffic, **within the network's CIDR** " +
					"(e.g. a subset of the guest network range). Defaults to the whole network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"dest_cidr_list": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Destination CIDR the traffic is allowed to reach. Defaults to `0.0.0.0/0` (anywhere).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"state": schema.StringAttribute{Computed: true, MarkdownDescription: "Current rule state.", PlanModifiers: useState},
		},
	}
}

func (r *egressRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan egressRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := plan.NetworkID.ValueString()
	rule, err := r.client.EgressRules.CreateEgressRules(ctx, &acsdk.CreateEgressRuleDto{
		NetworkID:      &networkID,
		Protocol:       plan.Protocol.ValueString(),
		StartPort:      int64ToIntPtr(plan.StartPort),
		EndPort:        int64ToIntPtr(plan.EndPort),
		SourceCidrList: stringToPtr(plan.SourceCidrList),
		DestCidrList:   stringToPtr(plan.DestCidrList),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating egress rule", err.Error())
		return
	}

	// Preserve planned config values; fill computed/defaulted fields from the
	// response (the platform defaults source to the network CIDR and dest to
	// 0.0.0.0/0 when omitted).
	plan.ID = types.StringValue(rule.ID)
	if plan.SourceCidrList.IsUnknown() || plan.SourceCidrList.IsNull() {
		plan.SourceCidrList = types.StringValue(rule.SourceCidrList)
	}
	if plan.DestCidrList.IsUnknown() || plan.DestCidrList.IsNull() {
		plan.DestCidrList = stringPtrToString(rule.DestCidrList)
	}
	plan.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *egressRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state egressRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.EgressRules.GetEgressRules(ctx, &acsdk.GetEgressRulesRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading egress rule", err.Error())
		return
	}

	// Config fields are immutable (ForceNew); keepStr preserves user values on a
	// normal refresh and hydrates them on import (ImportState seeds only id).
	state.NetworkID = keepStr(state.NetworkID, rule.NetworkID)
	// The API canonicalizes protocol to lowercase ("tcp"); config and the schema
	// enum are uppercase.
	state.Protocol = keepStr(state.Protocol, strings.ToUpper(rule.Protocol))
	state.StartPort = keepIntPtr(state.StartPort, rule.StartPort)
	state.EndPort = keepIntPtr(state.EndPort, rule.EndPort)
	state.SourceCidrList = keepStr(state.SourceCidrList, rule.SourceCidrList)
	if rule.DestCidrList != nil {
		state.DestCidrList = keepStr(state.DestCidrList, *rule.DestCidrList)
	}
	// Computed fields always refresh.
	state.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement. (The
// platform's egress update endpoint replaces the rule under a NEW id, so it is
// deliberately not used — see the constructor comment.)
func (r *egressRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[egressRuleModel](ctx, req, resp)
}

func (r *egressRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state egressRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.EgressRules.DeleteEgressRules(ctx, &acsdk.DeleteEgressRulesRequest{ID: state.ID.ValueString()}); err != nil {
		if isNotFound(err) {
			return // already gone
		}
		resp.Diagnostics.AddError("Error deleting egress rule", err.Error())
	}
}

func (r *egressRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
