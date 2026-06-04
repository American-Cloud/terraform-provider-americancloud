package provider

import (
	"context"
	"fmt"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = (*vmResource)(nil)
	_ resource.ResourceWithConfigure   = (*vmResource)(nil)
	_ resource.ResourceWithImportState = (*vmResource)(nil)
)

const (
	vmCreateTimeout = 20 * time.Minute
	vmDeleteTimeout = 15 * time.Minute
)

// Terminal status sets for the create poll — shared between its ready set and
// the root_volume_id hold-off check so the two can't drift.
var (
	vmReadyStatuses  = []string{"running", "started"}
	vmFailedStatuses = []string{"error", "failed"}
)

// NewVMResource — sdkRef: vms.CreateVms / GetVms / ScaleVms / ResizeDiskVms / DeleteVms.
// NOT_EXPOSED: vms.PowerVms / ReinstallVms / ResetPasswordVms / CreateConsoleVms
// (imperative actions, not declarable state — they belong in the CLI/MCP, not TF;
// the generated VM password is only obtainable via the reset op, so SSH access is
// keypairs/user_data); vms.UpdateHostnameVms (hostname isn't in the get/create
// response, so it can't be reconciled); GetMetricsVms / GetCostEstimateVms
// (read/preview surface).
//
// keypairs / user_data / tags / network_access are create-only (ForceNew) and are
// NOT hydrated on Read: the get response doesn't echo keypairs/userdata/
// network_access at all, and tags may be platform-injected — hydrating an
// optional ForceNew attribute the user didn't set would propose a destructive
// replacement. Consequence: these four can't be recovered by `terraform import`
// (an import under a config that sets them plans a replacement; documented).
func NewVMResource() resource.Resource { return &vmResource{} }

type vmResource struct{ baseResource }

type vmModel struct {
	ID                 types.String   `tfsdk:"id"`
	Name               types.String   `tfsdk:"name"`
	Region             types.String   `tfsdk:"region"`
	VMPackage          types.String   `tfsdk:"vm_package"`
	Vcpu               types.Int64    `tfsdk:"vcpu"`
	MemoryMb           types.Int64    `tfsdk:"memory_mb"`
	RootDiskGb         types.Int64    `tfsdk:"root_disk_gb"`
	Image              types.String   `tfsdk:"image"`
	Network            types.String   `tfsdk:"network"`
	SubscriptionPeriod types.String   `tfsdk:"subscription_period"`
	Keypairs           types.Set      `tfsdk:"keypairs"`
	UserData           types.String   `tfsdk:"user_data"`
	Tags               types.Set      `tfsdk:"tags"`
	NetworkAccess      types.Object   `tfsdk:"network_access"`
	Status             types.String   `tfsdk:"status"`
	IPAddress          types.String   `tfsdk:"ip_address"`
	RootVolumeID       types.String   `tfsdk:"root_volume_id"`
	CreatedAt          types.String   `tfsdk:"created_at"`
	Timeouts           timeouts.Value `tfsdk:"timeouts"`
}

// vmNetworkAccessModel mirrors the API's create-time NetworkAccessDto.
type vmNetworkAccessModel struct {
	AllowEgressAll types.Bool   `tfsdk:"allow_egress_all"`
	SourceCidr     types.String `tfsdk:"source_cidr"`
	InboundPorts   types.List   `tfsdk:"inbound_ports"`
}

type vmInboundPortModel struct {
	Port     types.Int64  `tfsdk:"port"`
	Protocol types.String `tfsdk:"protocol"`
}

func (r *vmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *vmResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	strReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A virtual machine. Provisions asynchronously — `apply` blocks until the VM is " +
			"running. `vcpu`, `memory_mb`, and `root_disk_gb` change in place (the platform reboots the " +
			"VM to apply); name, image, network, and region are immutable (changing them replaces the VM).",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "VM identifier (UUID).", PlanModifiers: useState},
			"name":       schema.StringAttribute{Required: true, MarkdownDescription: "VM name. Forces replacement.", PlanModifiers: strReplace},
			"region":     schema.StringAttribute{Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`. Forces replacement.", PlanModifiers: strReplace},
			"vm_package": schema.StringAttribute{Required: true, MarkdownDescription: "VM package label (from `list_vm_packages`). Forces replacement.", PlanModifiers: strReplace},
			"vcpu":       schema.Int64Attribute{Required: true, MarkdownDescription: "Number of vCPUs. Changing it resizes the VM in place (reboots it)."},
			"memory_mb":  schema.Int64Attribute{Required: true, MarkdownDescription: "Memory in MB. Changing it resizes the VM in place (reboots it)."},
			"root_disk_gb": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Root disk size in GB (min 25). Can be grown in place (reboots the VM); shrinking is rejected.",
				Validators: []validator.Int64{int64validator.AtLeast(25)},
			},
			"image": schema.StringAttribute{Required: true, MarkdownDescription: "Image label (from `list_images`). Forces replacement.", PlanModifiers: strReplace},
			"network": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Network UUID to attach the VM to. Omit to have the platform auto-create an isolated network " +
					"(required for `network_access`; the auto-created network is not managed by Terraform and survives the VM). Forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"subscription_period": schema.StringAttribute{
				Required: true, MarkdownDescription: "Billing period: `hourly` or `monthly`. Forces replacement.",
				Validators:    []validator.String{stringvalidator.OneOf("hourly", "monthly")},
				PlanModifiers: strReplace,
			},
			"keypairs": schema.SetAttribute{
				ElementType: types.StringType, Optional: true,
				MarkdownDescription: "SSH key names to install (see `americancloud_ssh_key`). Keys are installed for the image's access user — `root` on the stock Ubuntu images. Forces replacement. Not recoverable by `terraform import`.",
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"user_data": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cloud-init user data executed on first boot, as plain text (the provider base64-encodes it for the API). Forces replacement. Not recoverable by `terraform import`.",
				PlanModifiers:       strReplace,
			},
			"tags": schema.SetAttribute{
				ElementType: types.StringType, Optional: true,
				MarkdownDescription: "Tags to assign to the VM. Forces replacement. Not recoverable by `terraform import`.",
				PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"network_access": schema.SingleNestedAttribute{
				Optional: true,
				MarkdownDescription: "Create-time network access for a platform-created network: opens the requested inbound ports " +
					"(port forwarding + firewall rules on the network's public IP) and optionally allows all egress. " +
					"Only honored when `network` is omitted — conflicts with it. Forces replacement. Not recoverable by `terraform import`.",
				Validators:    []validator.Object{objectvalidator.ConflictsWith(path.MatchRoot("network"))},
				PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"allow_egress_all": schema.BoolAttribute{
						Required:            true,
						MarkdownDescription: "Allow all outbound traffic from the VM.",
					},
					"source_cidr": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Source CIDR the inbound ports are opened to. Omit for any source.",
					},
					"inbound_ports": schema.ListNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Inbound ports to open to the VM.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"port": schema.Int64Attribute{
									Required: true, MarkdownDescription: "Port number.",
									Validators: []validator.Int64{int64validator.Between(1, 65535)},
								},
								"protocol": schema.StringAttribute{
									Required: true, MarkdownDescription: "`TCP` or `UDP`.",
									Validators: []validator.String{stringvalidator.OneOf("TCP", "UDP")},
								},
							},
						},
					},
				},
			},
			"status":         schema.StringAttribute{Computed: true, MarkdownDescription: "Current lifecycle status.", PlanModifiers: useState},
			"ip_address":     schema.StringAttribute{Computed: true, MarkdownDescription: "Primary IP address.", PlanModifiers: useState},
			"root_volume_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Root volume identifier.", PlanModifiers: useState},
			"created_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time (RFC 3339).", PlanModifiers: useState},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dto := &acsdk.CreateVMDto{
		Name:      plan.Name.ValueString(),
		Region:    plan.Region.ValueString(),
		VMPackage: plan.VMPackage.ValueString(),
		VMSpecs: &acsdk.VMSpecsDto{
			Vcpu:       float64(plan.Vcpu.ValueInt64()),
			MemoryMb:   int(plan.MemoryMb.ValueInt64()),
			RootDiskGb: int(plan.RootDiskGb.ValueInt64()),
		},
		Image:              plan.Image.ValueString(),
		Network:            stringToPtr(plan.Network),
		SubscriptionPeriod: acsdk.CreateVMDtoSubscriptionPeriod(plan.SubscriptionPeriod.ValueString()),
		Userdata:           base64Ptr(plan.UserData), // API expects base64-encoded cloud-init
	}
	if !plan.Keypairs.IsNull() {
		resp.Diagnostics.Append(plan.Keypairs.ElementsAs(ctx, &dto.Keypairs, false)...)
	}
	if !plan.Tags.IsNull() {
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &dto.Tags, false)...)
	}
	if !plan.NetworkAccess.IsNull() {
		var na vmNetworkAccessModel
		resp.Diagnostics.Append(plan.NetworkAccess.As(ctx, &na, basetypes.ObjectAsOptions{})...)
		access := &acsdk.NetworkAccessDto{
			AllowEgressAll: na.AllowEgressAll.ValueBool(),
			SourceCidr:     stringToPtr(na.SourceCidr),
		}
		if !na.InboundPorts.IsNull() {
			var ports []vmInboundPortModel
			resp.Diagnostics.Append(na.InboundPorts.ElementsAs(ctx, &ports, false)...)
			for _, p := range ports {
				access.InboundPorts = append(access.InboundPorts, &acsdk.InboundPortDto{
					Port:     int(p.Port.ValueInt64()),
					Protocol: acsdk.InboundPortDtoProtocol(p.Protocol.ValueString()),
				})
			}
		}
		dto.NetworkAccess = access
	}
	if resp.Diagnostics.HasError() {
		return
	}

	vm, err := r.client.Vms.CreateVms(ctx, dto)
	if err != nil {
		resp.Diagnostics.AddError("Error creating VM", err.Error())
		return
	}

	// Seed state with the id and create-time values immediately, so a poll
	// timeout/error below can persist the id (the VM exists) instead of orphaning it.
	plan.ID = types.StringValue(vm.ID)
	plan.Status = types.StringValue(vm.Status)
	plan.IPAddress = types.StringValue(vm.IPAddress)
	plan.RootVolumeID = stringPtrToString(vm.RootVolumeID)
	plan.CreatedAt = types.StringValue(vm.CreatedAt.Format(time.RFC3339))
	// network is computed when omitted (the platform auto-creates one) — resolve
	// the planned unknown to the actual network id (or null until known).
	if plan.Network.IsUnknown() {
		plan.Network = stringPtrToString(vm.NetworkID)
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, vmCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...) // persist the id even if the timeout config is invalid
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	ready, perr := r.pollVMReady(pollCtx, vm.ID)
	if perr != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...) // persist the id first
		resp.Diagnostics.AddError("VM did not reach a running state", perr.Error())
		return
	}

	plan.Status = types.StringValue(ready.Status)
	plan.IPAddress = types.StringValue(ready.IPAddress)
	plan.RootVolumeID = stringPtrToString(ready.RootVolumeID)
	if plan.Network.IsNull() {
		plan.Network = stringPtrToString(ready.NetworkID) // auto-created network, late-populated
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vm, err := r.client.Vms.GetVms(ctx, &acsdk.GetVmsRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VM", err.Error())
		return
	}

	// Immutable (ForceNew) fields: keepStr preserves the user's value on a normal
	// refresh (so an API value normalized differently never shows as drift) and
	// hydrates from the API when state is empty (the `terraform import` case).
	// vm_package is intentionally NOT hydrated: the GET returns the package display
	// name ("Standard Custom"), not the label ("standard-custom") config uses, so it
	// can't round-trip on import until the API echoes the label (tracked API-side).
	state.Name = keepStr(state.Name, vm.Name)
	state.Region = keepStr(state.Region, vm.Region)
	state.Image = keepStr(state.Image, vm.Image)
	state.Network = keepStr(state.Network, derefString(vm.NetworkID))
	state.SubscriptionPeriod = keepStr(state.SubscriptionPeriod, vm.SubscriptionPeriod)
	// keypairs / user_data / tags / network_access: deliberately untouched — see
	// the constructor comment (not echoed / platform may inject tags; hydrating
	// an optional ForceNew attribute would risk a destructive replacement).
	// Mutable (in-place) fields: hydrate the live value so out-of-band changes show
	// as drift and import populates them. The API returns these faithfully.
	state.Vcpu = types.Int64Value(int64(vm.CPU))
	state.MemoryMb = types.Int64Value(int64(vm.MemoryMb))
	state.RootDiskGb = types.Int64Value(int64(vm.RootDiskGb))
	// Computed fields always refresh from the API.
	state.Status = types.StringValue(vm.Status)
	state.IPAddress = types.StringValue(vm.IPAddress)
	state.RootVolumeID = stringPtrToString(vm.RootVolumeID)
	state.CreatedAt = types.StringValue(vm.CreatedAt.Format(time.RFC3339))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place size changes: vcpu/memory_mb via ScaleVms, root_disk_gb
// (grow-only) via ResizeDiskVms. Both reboot the VM on the platform side and apply
// asynchronously; per the in-place rule we set state to the plan and let the next
// Read reconcile computed fields, rather than refreshing here (the API can briefly
// report the old size, which would trip "inconsistent result after apply").
func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Vcpu.Equal(state.Vcpu) || !plan.MemoryMb.Equal(state.MemoryMb) {
		cpu := int(plan.Vcpu.ValueInt64())
		mem := int(plan.MemoryMb.ValueInt64())
		if err := r.client.Vms.ScaleVms(ctx, &acsdk.ScaleVMDto{ID: state.ID.ValueString(), CPU: &cpu, MemoryMb: &mem}); err != nil {
			resp.Diagnostics.AddError("Error scaling VM", err.Error())
			return
		}
	}

	if !plan.RootDiskGb.Equal(state.RootDiskGb) {
		if plan.RootDiskGb.ValueInt64() < state.RootDiskGb.ValueInt64() {
			resp.Diagnostics.AddError(
				"Cannot shrink root disk",
				fmt.Sprintf("root_disk_gb can only be increased (current %d GB, requested %d GB).",
					state.RootDiskGb.ValueInt64(), plan.RootDiskGb.ValueInt64()),
			)
			return
		}
		reboot := true
		if _, err := r.client.Vms.ResizeDiskVms(ctx, &acsdk.ResizeVMDiskDto{
			ID: state.ID.ValueString(), SizeGb: int(plan.RootDiskGb.ValueInt64()), Reboot: &reboot,
		}); err != nil {
			resp.Diagnostics.AddError("Error resizing VM root disk", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vmModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteTimeout, diags := state.Timeouts.Delete(ctx, vmDeleteTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Vms.DeleteVms(ctx, &acsdk.DeleteVmsRequest{ID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting VM", err.Error())
		return
	}
	// Block until the VM is fully expunged so dependent resources (e.g. its
	// network) can be deleted in the same apply.
	pollCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if err := r.pollVMGone(pollCtx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("VM did not finish terminating", err.Error())
	}
}

func (r *vmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// pollVMReady polls until the VM reaches a running state (or a failure state, or
// the deadline). Returns the terminal VM.
func (r *vmResource) pollVMReady(ctx context.Context, id string) (*acsdk.VMResponseDto, error) {
	var last *acsdk.VMResponseDto
	err := pollReady(ctx, func(ctx context.Context) (string, error) {
		vm, e := r.client.Vms.GetVms(ctx, &acsdk.GetVmsRequest{ID: id})
		if e != nil {
			return "", e
		}
		last = vm
		// root_volume_id is populated a beat after the VM reports running. Hold
		// off "ready" until it's present, so dependents that reference
		// root_volume_id in the same apply (snapshots, volume attaches) resolve
		// to the real id instead of an empty string.
		if containsFold(vmReadyStatuses, vm.Status) &&
			(vm.RootVolumeID == nil || *vm.RootVolumeID == "") {
			return "provisioning", nil
		}
		return vm.Status, nil
	}, vmReadyStatuses, vmFailedStatuses)
	return last, err
}

// pollVMGone polls until the VM is gone (404) or expunged.
func (r *vmResource) pollVMGone(ctx context.Context, id string) error {
	return pollGone(ctx, func(ctx context.Context) (string, bool, error) {
		vm, e := r.client.Vms.GetVms(ctx, &acsdk.GetVmsRequest{ID: id})
		if e != nil {
			if isNotFound(e) {
				return "", true, nil
			}
			return "", false, e
		}
		return vm.Status, false, nil
		// Only terminal states count as gone — "expunging" is still in progress,
		// and treating it as done would let a dependent resource (e.g. the VM's
		// network) be deleted before the VM has actually released it. The 404
		// branch above is the usual completion signal once the record is purged.
	}, []string{"expunged", "destroyed"})
}
