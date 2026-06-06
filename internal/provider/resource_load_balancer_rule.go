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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*loadBalancerRuleResource)(nil)
	_ resource.ResourceWithConfigure   = (*loadBalancerRuleResource)(nil)
	_ resource.ResourceWithImportState = (*loadBalancerRuleResource)(nil)
)

var (
	loadBalancerAlgorithms = []string{"roundrobin", "leastconn", "source"}
	loadBalancerProtocols  = []string{"tcp", "udp", "tcp-proxy", "ssl"}
)

// NewLoadBalancerRuleResource — sdkRef: loadBalancerRules.CreateLoadBalancerRules /
// ListLoadBalancerRules / UpdateLoadBalancerRules / DeleteLoadBalancerRules /
// AssignVmsLoadBalancerRules / RemoveVmsLoadBalancerRules /
// ListInstancesLoadBalancerRules. No get-by-id endpoint → Read is
// list-and-match by id within the rule's public IP; the backend set refreshes
// from the applied-instances list. name/algorithm/description update in place;
// ports, protocol, source CIDR, and the IP replace (CloudStack's LB update
// only supports name/algorithm/description — sending the others would silently
// no-op).
func NewLoadBalancerRuleResource() resource.Resource { return &loadBalancerRuleResource{} }

type loadBalancerRuleResource struct{ baseResource }

type loadBalancerRuleModel struct {
	ID             types.String `tfsdk:"id"`
	IpID           types.String `tfsdk:"ip_id"`
	Name           types.String `tfsdk:"name"`
	Algorithm      types.String `tfsdk:"algorithm"`
	PublicPort     types.Int64  `tfsdk:"public_port"`
	PrivatePort    types.Int64  `tfsdk:"private_port"`
	Protocol       types.String `tfsdk:"protocol"`
	SourceCidrList types.String `tfsdk:"source_cidr_list"`
	Description    types.String `tfsdk:"description"`
	InstanceIDs    types.Set    `tfsdk:"instance_ids"`
	IPAddress      types.String `tfsdk:"ip_address"`
	NetworkID      types.String `tfsdk:"network_id"`
	State          types.String `tfsdk:"state"`
}

func (r *loadBalancerRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer_rule"
}

func (r *loadBalancerRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intRequiresReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A load balancer rule on a public IP, distributing traffic across backend " +
			"VMs in the IP's network. `name`, `algorithm`, `description`, and the backend set " +
			"(`instance_ids`) change in place; ports, protocol, the source CIDR, and the IP replace the rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Rule identifier (UUID).", PlanModifiers: useState},
			"ip_id": schema.StringAttribute{
				Required: true, MarkdownDescription: "Public IP identifier (from `americancloud_public_ip`) the load balancer listens on.",
				PlanModifiers: requiresReplace,
			},
			"name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Name of the load balancer rule.",
			},
			"algorithm": schema.StringAttribute{
				Required: true, MarkdownDescription: "Distribution algorithm: roundrobin, leastconn, or source.",
				Validators: []validator.String{stringvalidator.OneOf(loadBalancerAlgorithms...)},
			},
			"public_port": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Public port the load balancer listens on (1–65535).",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"private_port": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Port on the backend VMs that traffic is forwarded to (1–65535).",
				Validators:    []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers: intRequiresReplace,
			},
			"protocol": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Protocol the load balancer accepts: tcp, udp, tcp-proxy, or ssl. Defaults to tcp.",
				Validators:          []validator.String{stringvalidator.OneOf(loadBalancerProtocols...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"source_cidr_list": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Source CIDR allowed to reach the load balancer, e.g. `0.0.0.0/0`.",
				PlanModifiers: requiresReplace,
			},
			"description": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Free-form description of the rule. The platform does not echo this " +
					"back, so it is not recoverable by `terraform import` (the first apply after import converges by re-setting it).",
			},
			"instance_ids": schema.SetAttribute{
				Optional: true, ElementType: types.StringType,
				MarkdownDescription: "Identifiers of the backend VMs attached to the rule. VMs must be in the IP's network.",
			},
			"ip_address": schema.StringAttribute{Computed: true, MarkdownDescription: "The public IP address the rule listens on.", PlanModifiers: useState},
			"network_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the network the backends live in.", PlanModifiers: useState},
			"state":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current rule state.", PlanModifiers: useState},
		},
	}
}

func (r *loadBalancerRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan loadBalancerRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var protocol *acsdk.CreateLoadBalancerRuleDtoProtocol
	if !plan.Protocol.IsNull() && !plan.Protocol.IsUnknown() {
		p := acsdk.CreateLoadBalancerRuleDtoProtocol(plan.Protocol.ValueString())
		protocol = &p
	}
	// The API's create DTO takes ports as numeric strings (a v1 contract quirk —
	// firewall/egress/ACL ports are numbers); the response returns numbers.
	rule, err := r.client.LoadBalancerRules.CreateLoadBalancerRules(ctx, &acsdk.CreateLoadBalancerRuleDto{
		IpId:           plan.IpID.ValueString(),
		Name:           plan.Name.ValueString(),
		Algorithm:      acsdk.CreateLoadBalancerRuleDtoAlgorithm(plan.Algorithm.ValueString()),
		PublicPort:     strconv.FormatInt(plan.PublicPort.ValueInt64(), 10),
		PrivatePort:    strconv.FormatInt(plan.PrivatePort.ValueInt64(), 10),
		Protocol:       protocol,
		SourceCidrList: stringToPtr(plan.SourceCidrList),
		Description:    stringToPtr(plan.Description),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating load balancer rule", err.Error())
		return
	}

	// Preserve planned config values; fill computed fields from the response.
	plan.ID = types.StringValue(rule.ID)
	if plan.Protocol.IsUnknown() || plan.Protocol.IsNull() {
		plan.Protocol = types.StringValue(strings.ToLower(rule.Protocol))
	}
	plan.IPAddress = types.StringValue(rule.IPAddress)
	plan.NetworkID = types.StringValue(rule.NetworkID)
	plan.State = types.StringValue(rule.State)

	// Persist state (with the id) BEFORE attaching backends — if an assign
	// fails, the rule must not be orphaned from state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids := setToStrings(ctx, plan.InstanceIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(ids) > 0 {
		err := r.client.LoadBalancerRules.AssignVmsLoadBalancerRules(ctx, &acsdk.AssignVmsLoadBalancerRulesRequest{
			RuleID: rule.ID,
			Body:   &acsdk.AssignVmsToLoadBalancerRuleDto{VmIds: ids},
		})
		if err != nil {
			resp.Diagnostics.AddError(
				"Error assigning VMs to load balancer rule",
				"The rule was created and saved to state, but attaching backends failed: "+err.Error(),
			)
			return
		}
	}
}

func (r *loadBalancerRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state loadBalancerRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipID, id := state.IpID.ValueString(), state.ID.ValueString()
	rule, err := findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.LoadBalancerRuleResponseDto, error) {
			list, err := r.client.LoadBalancerRules.ListLoadBalancerRules(ctx, &acsdk.ListLoadBalancerRulesRequest{IpId: ipID, Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(lb *acsdk.LoadBalancerRuleResponseDto) bool { return lb.ID == id },
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading load balancer rules", err.Error())
		return
	}
	if rule == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// Mutable config refreshes from the API (drift detection); immutable config
	// uses keep* (refresh-stability + import hydration).
	state.IpID = keepStr(state.IpID, rule.IPAddressID)
	state.Name = types.StringValue(rule.Name)
	state.Algorithm = types.StringValue(rule.Algorithm)
	state.PublicPort = types.Int64Value(int64(rule.PublicPort))
	state.PrivatePort = types.Int64Value(int64(rule.PrivatePort))
	// The API canonicalizes protocol to lowercase, matching the schema enum.
	state.Protocol = keepStr(state.Protocol, strings.ToLower(rule.Protocol))
	if rule.SourceCidrList != nil {
		state.SourceCidrList = keepStr(state.SourceCidrList, *rule.SourceCidrList)
	}
	// description is never echoed — leave state untouched.

	// Refresh the backend set from the applied-instances list. Keep null when
	// both config and the platform agree there are no backends, so an
	// instance-less rule doesn't drift between null and [].
	applied := true
	instances, err := collectPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.LoadBalancerRuleInstanceDto, error) {
			list, err := r.client.LoadBalancerRules.ListInstancesLoadBalancerRules(ctx, &acsdk.ListInstancesLoadBalancerRulesRequest{
				RuleID: id, Applied: &applied, Page: &page, PageSize: &pageSize,
			})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		})
	if err != nil {
		resp.Diagnostics.AddError("Error reading load balancer rule instances", err.Error())
		return
	}
	if len(instances) > 0 || !state.InstanceIDs.IsNull() {
		ids := make([]string, 0, len(instances))
		for _, inst := range instances {
			ids = append(ids, inst.ID)
		}
		set, diags := types.SetValueFrom(ctx, types.StringType, ids)
		resp.Diagnostics.Append(diags...)
		state.InstanceIDs = set
	}

	// Computed fields always refresh.
	state.IPAddress = types.StringValue(rule.IPAddress)
	state.NetworkID = types.StringValue(rule.NetworkID)
	state.State = types.StringValue(rule.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *loadBalancerRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state loadBalancerRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// In-place fields via the platform's LB update (name/algorithm/description
	// only — everything else is ForceNew and unreachable here).
	if !plan.Name.Equal(state.Name) || !plan.Algorithm.Equal(state.Algorithm) || !plan.Description.Equal(state.Description) {
		name := plan.Name.ValueString()
		algorithm := acsdk.UpdateLoadBalancerRuleDtoAlgorithm(plan.Algorithm.ValueString())
		_, err := r.client.LoadBalancerRules.UpdateLoadBalancerRules(ctx, &acsdk.UpdateLoadBalancerRuleDto{
			RuleID:      state.ID.ValueString(),
			Name:        &name,
			Algorithm:   &algorithm,
			Description: stringToPtr(plan.Description),
		})
		if err != nil {
			resp.Diagnostics.AddError("Error updating load balancer rule", err.Error())
			return
		}
	}

	// Reconcile the backend set: assign the added VMs, remove the dropped ones.
	planIDs := setToStrings(ctx, plan.InstanceIDs, &resp.Diagnostics)
	stateIDs := setToStrings(ctx, state.InstanceIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	added, removed := diffStringSets(stateIDs, planIDs)
	if len(added) > 0 {
		err := r.client.LoadBalancerRules.AssignVmsLoadBalancerRules(ctx, &acsdk.AssignVmsLoadBalancerRulesRequest{
			RuleID: state.ID.ValueString(),
			Body:   &acsdk.AssignVmsToLoadBalancerRuleDto{VmIds: added},
		})
		if err != nil {
			resp.Diagnostics.AddError("Error assigning VMs to load balancer rule", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		err := r.client.LoadBalancerRules.RemoveVmsLoadBalancerRules(ctx, &acsdk.RemoveVmsLoadBalancerRulesRequest{
			RuleID: state.ID.ValueString(),
			Body:   &acsdk.AssignVmsToLoadBalancerRuleDto{VmIds: removed},
		})
		if err != nil {
			resp.Diagnostics.AddError("Error removing VMs from load balancer rule", err.Error())
			return
		}
	}

	// Per the in-place update rules: state is the plan, never the API response.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *loadBalancerRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state loadBalancerRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A rule whose backends are being expunged in the same destroy can briefly
	// 409/504 while CloudStack releases them.
	err := retryTransientDelete(ctx, func(ctx context.Context) error {
		return r.client.LoadBalancerRules.DeleteLoadBalancerRules(ctx, &acsdk.DeleteLoadBalancerRulesRequest{RuleID: state.ID.ValueString()})
	})
	if err != nil {
		resp.Diagnostics.AddError("Error deleting load balancer rule", err.Error())
	}
}

func (r *loadBalancerRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Composite id: ipId/ruleId (Read lists rules within the IP, then matches the rule id).
	importCompositeID(ctx, resp, req.ID, "ipId/ruleId", path.Root("ip_id"), path.Root("id"))
}
