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
	_ resource.Resource                = (*firewallRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*firewallRuleResource)(nil)
	_ resource.ResourceWithImportState = (*firewallRuleResource)(nil)
)

var (
	firewallProtocols = []string{"TCP", "UDP", "ICMP", "ALL"}
	firewallTypes     = []string{"Ingress", "Egress"}
)

// NewFirewallRuleResource — sdkRef: firewallRules.CreateFirewallRules /
// ListFirewallRules / DeleteFirewallRules. No get-by-id endpoint → Read is
// list-and-match by id within the rule's public IP. Immutable — any change replaces.
func NewFirewallRuleResource() resource.Resource { return &firewallRuleResource{} }

type firewallRuleResource struct{ baseResource }

type firewallRuleModel struct {
	ID             types.String `tfsdk:"id"`
	IpID           types.String `tfsdk:"ip_id"`
	Protocol       types.String `tfsdk:"protocol"`
	StartPort      types.Int64  `tfsdk:"start_port"`
	EndPort        types.Int64  `tfsdk:"end_port"`
	SourceCidrList types.String `tfsdk:"source_cidr_list"`
	Type           types.String `tfsdk:"type"`
	IPAddress      types.String `tfsdk:"ip_address"`
	State          types.String `tfsdk:"state"`
}

func (r *firewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *firewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An inbound/outbound firewall rule on a public IP. Immutable — any change replaces the rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Rule identifier (UUID).", PlanModifiers: useState},
			"ip_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Public IP identifier (from `americancloud_public_ip`) to attach the rule to.",
				PlanModifiers: requiresReplace,
			},
			"protocol": schema.StringAttribute{
				Required: true, MarkdownDescription: "Protocol: one of TCP, UDP, ICMP, ALL.",
				Validators:    []validator.String{stringvalidator.OneOf(firewallProtocols...)},
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
				Required: true, MarkdownDescription: "Allowed source CIDR(s), e.g. `0.0.0.0/0`.",
				PlanModifiers: requiresReplace,
			},
			"type": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Direction: Ingress or Egress. Defaults to Ingress.",
				Validators:    []validator.String{stringvalidator.OneOf(firewallTypes...)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"ip_address": schema.StringAttribute{Computed: true, MarkdownDescription: "The public IP address the rule applies to.", PlanModifiers: useState},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current rule state.", PlanModifiers: useState},
		},
	}
}

func (r *firewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var typePtr *acsdk.CreateFirewallRuleDtoType
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		t := acsdk.CreateFirewallRuleDtoType(plan.Type.ValueString())
		typePtr = &t
	}

	rule, err := r.client.FirewallRules.CreateFirewallRules(ctx, &acsdk.CreateFirewallRuleDto{
		IpId:           plan.IpID.ValueString(),
		Protocol:       acsdk.CreateFirewallRuleDtoProtocol(plan.Protocol.ValueString()),
		StartPort:      int64ToIntPtr(plan.StartPort),
		EndPort:        int64ToIntPtr(plan.EndPort),
		SourceCidrList: plan.SourceCidrList.ValueString(),
		Type:           typePtr,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall rule", err.Error())
		return
	}

	// Preserve the planned (known) config values; set only the computed fields
	// from the response. The API can report config fields (ports/CIDR) slightly
	// differently right after create, which would trip "inconsistent result".
	plan.ID = types.StringValue(rule.ID)
	plan.IPAddress = types.StringValue(rule.IPAddress)
	plan.State = types.StringValue(rule.State)
	if rule.Type != nil {
		plan.Type = types.StringValue(*rule.Type)
	} else if plan.Type.IsUnknown() {
		plan.Type = types.StringValue("Ingress")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipID, id := state.IpID.ValueString(), state.ID.ValueString()
	rule, err := findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.FirewallRuleResponseDto, error) {
			list, err := r.client.FirewallRules.ListFirewallRules(ctx, &acsdk.ListFirewallRulesRequest{IpId: ipID, Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(fr *acsdk.FirewallRuleResponseDto) bool { return fr.ID == id },
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading firewall rules", err.Error())
		return
	}
	if rule == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Config fields are immutable (ForceNew). keepStr preserves the user's values
	// on a normal refresh (so API normalization of ports/CIDR never shows as
	// drift) and hydrates them from the API when state is empty — the
	// `terraform import` case (ImportState seeds only ip_id + id). Without this an
	// imported rule would have null config and the next plan would propose
	// replacement.
	state.IpID = keepStr(state.IpID, rule.IPAddressID)
	// The API canonicalizes protocol to lowercase ("tcp"); the schema enum and
	// config are uppercase ("TCP"). Upper-case the hydrated value, or an imported
	// rule would force replacement on protocol case alone.
	state.Protocol = keepStr(state.Protocol, strings.ToUpper(rule.Protocol))
	state.StartPort = keepIntPtr(state.StartPort, rule.StartPort)
	state.EndPort = keepIntPtr(state.EndPort, rule.EndPort)
	state.SourceCidrList = keepStr(state.SourceCidrList, rule.SourceCidrList)
	// The API omits type for the default Ingress direction, so it isn't always
	// echoed. Hydrate it when present; otherwise default to Ingress (matching
	// Create) so an imported ingress rule round-trips instead of forcing replace.
	// (An Egress rule is expected to echo its type, so it hydrates via the first
	// branch.)
	if rule.Type != nil {
		state.Type = keepStr(state.Type, *rule.Type)
	} else if state.Type.IsNull() || state.Type.IsUnknown() {
		state.Type = types.StringValue("Ingress")
	}
	// Computed fields always refresh from the API.
	state.IPAddress = types.StringValue(rule.IPAddress)
	state.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement.
func (r *firewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[firewallRuleModel](ctx, req, resp)
}

func (r *firewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.FirewallRules.DeleteFirewallRules(ctx, &acsdk.DeleteFirewallRulesRequest{RuleID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting firewall rule", err.Error())
	}
}

func (r *firewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Composite id: ipId/ruleId (Read lists rules within the IP, then matches the rule id).
	importCompositeID(ctx, resp, req.ID, "ipId/ruleId", path.Root("ip_id"), path.Root("id"))
}
