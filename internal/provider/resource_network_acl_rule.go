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
	_ resource.Resource                = (*networkACLRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkACLRuleResource)(nil)
	_ resource.ResourceWithImportState = (*networkACLRuleResource)(nil)
)

var (
	aclRuleProtocols    = []string{"TCP", "UDP", "ICMP", "ALL"}
	aclRuleActions      = []string{"Allow", "Deny"}
	aclRuleTrafficTypes = []string{"Ingress", "Egress"}
)

// NewNetworkACLRuleResource — sdkRef: networkACLs.CreateRuleNetworkACLs /
// GetRuleNetworkACLs / DeleteRuleNetworkACLs.
// NOT_EXPOSED: networkACLs.ListRulesNetworkACLs — Read uses get-by-id; the
// rules-of-a-list view is a data-source candidate (roadmap).
func NewNetworkACLRuleResource() resource.Resource { return &networkACLRuleResource{} }

type networkACLRuleResource struct{ baseResource }

type networkACLRuleModel struct {
	ID          types.String `tfsdk:"id"`
	AclID       types.String `tfsdk:"acl_id"`
	Number      types.Int64  `tfsdk:"number"`
	Protocol    types.String `tfsdk:"protocol"`
	CidrList    types.String `tfsdk:"cidr_list"`
	Action      types.String `tfsdk:"action"`
	TrafficType types.String `tfsdk:"traffic_type"`
	StartPort   types.Int64  `tfsdk:"start_port"`
	EndPort     types.Int64  `tfsdk:"end_port"`
	IcmpType    types.Int64  `tfsdk:"icmp_type"`
	IcmpCode    types.Int64  `tfsdk:"icmp_code"`
}

func (r *networkACLRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_acl_rule"
}

func (r *networkACLRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A rule in a `americancloud_network_acl` list. Rules are evaluated in " +
			"ascending `number` order; the first match wins. Immutable — any change replaces the rule. " +
			"Deleting the parent ACL list deletes its rules.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Rule identifier (UUID).", PlanModifiers: useState},
			"acl_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "ACL list identifier (from `americancloud_network_acl`) the rule belongs to.",
				PlanModifiers: requiresReplace,
			},
			"number": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Rule number (1–1000). Unique within the list; determines evaluation order. Assigned by the platform when omitted.",
				Validators:          []validator.Int64{int64validator.Between(1, 1000)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
			},
			"protocol": schema.StringAttribute{
				Required: true, MarkdownDescription: "Protocol: one of TCP, UDP, ICMP, ALL.",
				Validators:    []validator.String{stringvalidator.OneOf(aclRuleProtocols...)},
				PlanModifiers: requiresReplace,
			},
			"cidr_list": schema.StringAttribute{
				Required: true, MarkdownDescription: "CIDR the rule applies to, e.g. `0.0.0.0/0`.",
				PlanModifiers: requiresReplace,
			},
			"action": schema.StringAttribute{
				Required: true, MarkdownDescription: "Whether matching traffic is allowed: Allow or Deny.",
				Validators:    []validator.String{stringvalidator.OneOf(aclRuleActions...)},
				PlanModifiers: requiresReplace,
			},
			"traffic_type": schema.StringAttribute{
				Required: true, MarkdownDescription: "Direction the rule applies to: Ingress or Egress.",
				Validators:    []validator.String{stringvalidator.OneOf(aclRuleTrafficTypes...)},
				PlanModifiers: requiresReplace,
			},
			"start_port": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Start of the port range (1–65535). Used for TCP/UDP.",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"end_port": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "End of the port range (1–65535). Used for TCP/UDP.",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"icmp_type": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "ICMP message type. Used for the ICMP protocol.",
				PlanModifiers: intRequiresReplace,
			},
			"icmp_code": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "ICMP message code. Used for the ICMP protocol.",
				PlanModifiers: intRequiresReplace,
			},
		},
	}
}

func (r *networkACLRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkACLRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API's create DTO takes number/icmpType/icmpCode as numeric strings (a
	// v1 contract quirk); responses return numbers.
	rule, err := r.client.NetworkACLs.CreateRuleNetworkACLs(ctx, &acsdk.CreateNetworkACLRuleDto{
		ListID:      plan.AclID.ValueString(),
		CidrList:    plan.CidrList.ValueString(),
		Protocol:    plan.Protocol.ValueString(),
		Action:      plan.Action.ValueString(),
		TrafficType: plan.TrafficType.ValueString(),
		Number:      int64ToStringPtr(plan.Number),
		StartPort:   int64ToIntPtr(plan.StartPort),
		EndPort:     int64ToIntPtr(plan.EndPort),
		IcmpType:    int64ToStringPtr(plan.IcmpType),
		IcmpCode:    int64ToStringPtr(plan.IcmpCode),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating network ACL rule", err.Error())
		return
	}

	// Preserve planned config values; fill computed fields from the response
	// (the platform assigns `number` when omitted).
	plan.ID = types.StringValue(rule.ID)
	if plan.Number.IsUnknown() || plan.Number.IsNull() {
		plan.Number = types.Int64Value(int64(rule.Number))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkACLRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkACLRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.NetworkACLs.GetRuleNetworkACLs(ctx, &acsdk.GetRuleNetworkACLsRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading network ACL rule", err.Error())
		return
	}

	// Config fields are immutable (ForceNew); keep* preserves user values on a
	// normal refresh and hydrates them on import (ImportState seeds only id).
	state.AclID = keepStr(state.AclID, rule.ACLListID)
	// ACL rules echo protocol uppercase today, but normalize anyway — the
	// sibling rule endpoints (firewall/egress/PF) canonicalize to lowercase.
	state.Protocol = keepStr(state.Protocol, strings.ToUpper(rule.Protocol))
	state.CidrList = keepStr(state.CidrList, rule.CidrList)
	state.Action = keepStr(state.Action, rule.Action)
	state.TrafficType = keepStr(state.TrafficType, rule.TrafficType)
	state.Number = types.Int64Value(int64(rule.Number))
	state.StartPort = keepIntPtr(state.StartPort, rule.StartPort)
	state.EndPort = keepIntPtr(state.EndPort, rule.EndPort)
	state.IcmpType = keepIntPtr(state.IcmpType, rule.IcmpType)
	state.IcmpCode = keepIntPtr(state.IcmpCode, rule.IcmpCode)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement.
func (r *networkACLRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[networkACLRuleModel](ctx, req, resp)
}

func (r *networkACLRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkACLRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.NetworkACLs.DeleteRuleNetworkACLs(ctx, &acsdk.DeleteRuleNetworkACLsRequest{ID: state.ID.ValueString()}); err != nil {
		if isNotFound(err) {
			return // already gone (e.g. the parent list's delete cascaded first)
		}
		resp.Diagnostics.AddError("Error deleting network ACL rule", err.Error())
	}
}

func (r *networkACLRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
