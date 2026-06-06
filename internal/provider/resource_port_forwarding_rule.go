package provider

import (
	"context"
	"strconv"
	"strings"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*portForwardingRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*portForwardingRuleResource)(nil)
	_ resource.ResourceWithImportState = (*portForwardingRuleResource)(nil)
)

var portForwardingProtocols = []string{"TCP", "UDP"}

// NewPortForwardingRuleResource — sdkRef: portForwarding.CreatePortForwarding /
// ListPortForwarding / DeletePortForwarding. No get-by-id endpoint → Read is
// list-and-match by id within the rule's public IP. Immutable — any change
// replaces (CloudStack has no port-forwarding update operation).
func NewPortForwardingRuleResource() resource.Resource { return &portForwardingRuleResource{} }

type portForwardingRuleResource struct{ baseResource }

type portForwardingRuleModel struct {
	ID           types.String `tfsdk:"id"`
	IpID         types.String `tfsdk:"ip_id"`
	VmID         types.String `tfsdk:"vm_id"`
	PrivatePort  types.Int64  `tfsdk:"private_port"`
	PublicPort   types.Int64  `tfsdk:"public_port"`
	Protocol     types.String `tfsdk:"protocol"`
	OpenFirewall types.Bool   `tfsdk:"open_firewall"`
	VMName       types.String `tfsdk:"vm_name"`
	IPAddress    types.String `tfsdk:"ip_address"`
	State        types.String `tfsdk:"state"`
}

func (r *portForwardingRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forwarding_rule"
}

func (r *portForwardingRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Forwards a port on a public IP to a port on a VM in the IP's network. " +
			"Pair with `americancloud_firewall_rule` on the same IP to allow the inbound traffic — " +
			"a firewall rule alone does not route traffic to the VM, and a forwarding rule alone is " +
			"blocked by the firewall. Immutable — any change replaces the rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Rule identifier (UUID).", PlanModifiers: useState},
			"ip_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Public IP identifier (from `americancloud_public_ip`) that receives the inbound traffic.",
				PlanModifiers: requiresReplace,
			},
			"vm_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Identifier of the VM the traffic is forwarded to. The VM must be in the IP's network.",
				PlanModifiers: requiresReplace,
			},
			"private_port": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Port on the VM that traffic is forwarded to (1–65535).",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"public_port": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Port on the public IP that receives the inbound traffic (1–65535).",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"protocol": schema.StringAttribute{
				Required: true, MarkdownDescription: "Protocol: TCP or UDP.",
				Validators:    []validator.String{stringvalidator.OneOf(portForwardingProtocols...)},
				PlanModifiers: requiresReplace,
			},
			"open_firewall": schema.BoolAttribute{
				Optional: true, MarkdownDescription: "When `true`, also creates a matching firewall rule on the public IP. " +
					"Create-only: the platform does not echo this back, so it is not recoverable by `terraform import` " +
					"(an import under a config that sets it plans a replacement), and the auto-created firewall rule " +
					"is not managed by this resource.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"vm_name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the target VM.", PlanModifiers: useState},
			"ip_address": schema.StringAttribute{Computed: true, MarkdownDescription: "The public IP address the rule applies to.", PlanModifiers: useState},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current rule state.", PlanModifiers: useState},
		},
	}
}

func (r *portForwardingRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portForwardingRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API's create DTO takes ports as numeric strings (a v1 contract quirk —
	// firewall/egress/ACL ports are numbers); the response returns numbers.
	var openFirewall *string
	if !plan.OpenFirewall.IsNull() && !plan.OpenFirewall.IsUnknown() {
		s := strconv.FormatBool(plan.OpenFirewall.ValueBool())
		openFirewall = &s
	}
	rule, err := r.client.PortForwarding.CreatePortForwarding(ctx, &acsdk.CreatePortForwardingRuleDto{
		IpId:         plan.IpID.ValueString(),
		VmId:         plan.VmID.ValueString(),
		PrivatePort:  strconv.FormatInt(plan.PrivatePort.ValueInt64(), 10),
		PublicPort:   strconv.FormatInt(plan.PublicPort.ValueInt64(), 10),
		Protocol:     acsdk.CreatePortForwardingRuleDtoProtocol(plan.Protocol.ValueString()),
		OpenFirewall: openFirewall,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating port forwarding rule", err.Error())
		return
	}

	// Preserve planned config values; set only computed fields from the response.
	plan.ID = types.StringValue(rule.ID)
	plan.VMName = types.StringValue(rule.VMName)
	plan.IPAddress = types.StringValue(rule.IPAddress)
	plan.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portForwardingRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portForwardingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipID, id := state.IpID.ValueString(), state.ID.ValueString()
	rule, err := findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.PortForwardingRuleResponseDto, error) {
			list, err := r.client.PortForwarding.ListPortForwarding(ctx, &acsdk.ListPortForwardingRequest{IpId: ipID, Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(pf *acsdk.PortForwardingRuleResponseDto) bool { return pf.ID == id },
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading port forwarding rules", err.Error())
		return
	}
	if rule == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Config fields are immutable (ForceNew); keepStr preserves user values on
	// refresh and hydrates them on import (ImportState seeds only ip_id + id).
	state.IpID = keepStr(state.IpID, rule.IPAddressID)
	state.VmID = keepStr(state.VmID, rule.VmId)
	// The API canonicalizes protocol to lowercase ("tcp"); config and the schema
	// enum are uppercase. Upper-case the hydrated value or an imported rule would
	// force replacement on case alone.
	state.Protocol = keepStr(state.Protocol, strings.ToUpper(rule.Protocol))
	// Ports echo back as numbers exactly as sent — refresh directly (also
	// hydrates the import case).
	state.PrivatePort = types.Int64Value(int64(rule.PrivatePort))
	state.PublicPort = types.Int64Value(int64(rule.PublicPort))
	// open_firewall is create-only and never echoed — leave state untouched.
	// Computed fields always refresh.
	state.VMName = types.StringValue(rule.VMName)
	state.IPAddress = types.StringValue(rule.IPAddress)
	state.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every editable attribute forces replacement.
func (r *portForwardingRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[portForwardingRuleModel](ctx, req, resp)
}

func (r *portForwardingRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portForwardingRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PortForwarding.DeletePortForwarding(ctx, &acsdk.DeletePortForwardingRequest{RuleID: state.ID.ValueString()}); err != nil {
		if isNotFound(err) {
			return // already gone
		}
		resp.Diagnostics.AddError("Error deleting port forwarding rule", err.Error())
	}
}

func (r *portForwardingRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Composite id: ipId/ruleId (Read lists rules within the IP, then matches the rule id).
	importCompositeID(ctx, resp, req.ID, "ipId/ruleId", path.Root("ip_id"), path.Root("id"))
}
