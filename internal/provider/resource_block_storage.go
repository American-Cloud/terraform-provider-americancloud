package provider

import (
	"context"
	"fmt"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*blockStorageResource)(nil)
	_ resource.ResourceWithConfigure   = (*blockStorageResource)(nil)
	_ resource.ResourceWithImportState = (*blockStorageResource)(nil)
)

// NewBlockStorageResource — sdkRef: blockStorage.CreateBlockStorage / GetBlockStorage /
// ResizeBlockStorage / DeleteBlockStorage.
// NOT_EXPOSED (this increment): blockStorage.AttachBlockStorage / DetachBlockStorage —
// attachment lands with the vm resource so it can be validated end-to-end;
// blockStorage.ListBlockStorage / ListSnapshotsBlockStorage / GetCostEstimateBlockStorage
// — data-source / preview surface.
func NewBlockStorageResource() resource.Resource { return &blockStorageResource{} }

type blockStorageResource struct {
	baseResource
}

type blockStorageModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	SizeGb    types.Int64  `tfsdk:"size_gb"`
	Region    types.String `tfsdk:"region"`
	Status    types.String `tfsdk:"status"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *blockStorageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_block_storage"
}

func (r *blockStorageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A block storage volume. `size_gb` can be grown in place; volumes cannot be shrunk.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Volume identifier (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Volume name. Changing this forces a new volume.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"size_gb": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Size in GiB (minimum 5). Can be increased in place (grow-only); a decrease is rejected.",
				Validators:          []validator.Int64{int64validator.AtLeast(5)},
			},
			"region": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Region label, e.g. `us-west-0`. Changing this forces a new volume.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current volume status.",
				// UseStateForUnknown so an in-place resize doesn't mark status
				// unknown (which can't be persisted); Read reconciles it.
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the volume was created (RFC 3339).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *blockStorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan blockStorageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vol, err := r.client.BlockStorage.CreateBlockStorage(ctx, &acsdk.CreateVolumeDto{
		Name:   plan.Name.ValueString(),
		SizeGb: int(plan.SizeGb.ValueInt64()),
		Region: plan.Region.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating block storage volume", err.Error())
		return
	}

	state := volumeModelFromSDK(vol)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *blockStorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state blockStorageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vol, err := r.client.BlockStorage.GetBlockStorage(ctx, &acsdk.GetBlockStorageRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading block storage volume", err.Error())
		return
	}

	model := volumeModelFromSDK(vol)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *blockStorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state blockStorageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.SizeGb.ValueInt64() != state.SizeGb.ValueInt64() {
		if plan.SizeGb.ValueInt64() < state.SizeGb.ValueInt64() {
			resp.Diagnostics.AddError(
				"Cannot shrink volume",
				fmt.Sprintf("size_gb can only be increased (current %d GiB, requested %d GiB).",
					state.SizeGb.ValueInt64(), plan.SizeGb.ValueInt64()),
			)
			return
		}
		if _, err := r.client.BlockStorage.ResizeBlockStorage(ctx, &acsdk.ResizeVolumeDto{
			ID:     state.ID.ValueString(),
			SizeGb: int(plan.SizeGb.ValueInt64()),
		}); err != nil {
			resp.Diagnostics.AddError("Error resizing block storage volume", err.Error())
			return
		}
	}

	// The resize applies asynchronously — the volume may briefly still report the
	// old size, so do NOT refresh from the API here: Terraform requires the
	// applied value to equal the planned (known) size_gb, and a stale Get would
	// trip "inconsistent result after apply". Persist the plan; computed fields
	// (status) reconcile on the next Read.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *blockStorageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state blockStorageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.BlockStorage.DeleteBlockStorage(ctx, &acsdk.DeleteBlockStorageRequest{ID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting block storage volume", err.Error())
	}
}

func (r *blockStorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// volumeModelFromSDK maps an SDK volume response into resource state.
func volumeModelFromSDK(vol *acsdk.VolumeResponseDto) blockStorageModel {
	return blockStorageModel{
		ID:        types.StringValue(vol.ID),
		Name:      types.StringValue(vol.Name),
		SizeGb:    types.Int64Value(int64(vol.SizeGb)),
		Region:    types.StringValue(vol.Region),
		Status:    types.StringValue(vol.Status),
		CreatedAt: types.StringValue(vol.CreatedAt.Format(time.RFC3339)),
	}
}
