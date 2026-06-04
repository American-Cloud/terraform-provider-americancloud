package provider

import (
	"context"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*snapshotResource)(nil)
	_ resource.ResourceWithConfigure   = (*snapshotResource)(nil)
	_ resource.ResourceWithImportState = (*snapshotResource)(nil)
)

var snapshotTypes = []string{"DataDisk", "RootDisk"}

// NewSnapshotResource — sdkRef: snapshots.CreateSnapshots / GetSnapshots / DeleteSnapshots.
// NOT_EXPOSED: snapshots.RevertSnapshots (imperative restore, not declarable state);
// snapshots.ListSnapshots (data-source surface); snapshots.GetCostEstimateSnapshots (preview).
func NewSnapshotResource() resource.Resource { return &snapshotResource{} }

type snapshotResource struct {
	baseResource
}

type snapshotModel struct {
	ID        types.String `tfsdk:"id"`
	VolumeID  types.String `tfsdk:"volume_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	SizeGb    types.Int64  `tfsdk:"size_gb"`
	Status    types.String `tfsdk:"status"`
	Region    types.String `tfsdk:"region"`
	CreatedAt types.String `tfsdk:"created_at"`
	VmID      types.String `tfsdk:"vm_id"`
}

func (r *snapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot"
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A point-in-time snapshot of a block storage volume. Immutable — any change replaces it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshot identifier (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"volume_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Identifier of the volume to snapshot (from `americancloud_block_storage`).",
				PlanModifiers:       requiresReplace,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot name.",
				PlanModifiers:       requiresReplace,
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Whether the snapshot is of a data-disk volume or a VM root disk: one of DataDisk, RootDisk.",
				Validators:          []validator.String{stringvalidator.OneOf(snapshotTypes...)},
				PlanModifiers:       requiresReplace,
			},
			"size_gb": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Snapshot size in GiB.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current snapshot status.",
			},
			"region": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Region the snapshot resides in.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the snapshot was created (RFC 3339).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vm_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the VM the source volume was attached to, if any.",
			},
		},
	}
}

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	snap, err := r.client.Snapshots.CreateSnapshots(ctx, &acsdk.CreateSnapshotDto{
		VolumeID: plan.VolumeID.ValueString(),
		Name:     plan.Name.ValueString(),
		Type:     acsdk.CreateSnapshotDtoType(plan.Type.ValueString()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating snapshot", err.Error())
		return
	}

	state := snapshotModelFromSDK(snap)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	snap, err := r.client.Snapshots.GetSnapshots(ctx, &acsdk.GetSnapshotsRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading snapshot", err.Error())
		return
	}

	model := snapshotModelFromSDK(snap)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update is unreachable: every editable attribute forces replacement.
func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[snapshotModel](ctx, req, resp)
}

// snapshotDeleteTimeout bounds the delete poll below. CloudStack snapshot
// deletion is asynchronous storage I/O; the API's record 404s once it has
// actually completed.
const snapshotDeleteTimeout = 10 * time.Minute

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Retried: the API documents transient failures on this delete — 409 while
	// the snapshot's volume is mid-modification/removal (e.g. its VM expunging
	// in the same destroy), 504 while deletion is settling. A 404 (already
	// gone) counts as done — see retryTransientDelete.
	if err := retryTransientDelete(ctx, func(ctx context.Context) error {
		_, e := r.client.Snapshots.DeleteSnapshots(ctx, &acsdk.DeleteSnapshotsRequest{ID: state.ID.ValueString()})
		return e
	}); err != nil {
		resp.Diagnostics.AddError("Error deleting snapshot", err.Error())
		return
	}
	// The API initiates the CloudStack snapshot deletion asynchronously — block
	// until the snapshot is actually gone. Returning early lets a dependent
	// teardown (the snapshotted VM in the same destroy) start expunging while
	// the snapshot still exists, which wedges the volume removal: the expunge
	// grinds against the surviving snapshot and the destroy times out.
	pollCtx, cancel := context.WithTimeout(ctx, snapshotDeleteTimeout)
	defer cancel()
	if err := pollGone(pollCtx, func(ctx context.Context) (string, bool, error) {
		s, e := r.client.Snapshots.GetSnapshots(ctx, &acsdk.GetSnapshotsRequest{ID: state.ID.ValueString()})
		if e != nil {
			if isNotFound(e) {
				return "", true, nil
			}
			return "", false, e
		}
		return s.Status, false, nil
	}, []string{"deleted", "destroyed"}); err != nil {
		resp.Diagnostics.AddError("Snapshot did not finish deleting", err.Error())
	}
}

func (r *snapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// snapshotModelFromSDK maps an SDK snapshot response into state. As of API 1.3.0
// the response always carries `type`, so it round-trips cleanly on import.
func snapshotModelFromSDK(snap *acsdk.SnapshotResponseDto) snapshotModel {
	return snapshotModel{
		ID:        types.StringValue(snap.ID),
		VolumeID:  types.StringValue(snap.VolumeID),
		Name:      types.StringValue(snap.Name),
		Type:      types.StringValue(string(snap.Type)),
		SizeGb:    types.Int64Value(int64(snap.SizeGb)),
		Status:    types.StringValue(snap.Status),
		Region:    types.StringValue(snap.Region),
		CreatedAt: types.StringValue(snap.CreatedAt.Format(time.RFC3339)),
		VmID:      stringPtrToString(snap.VmId),
	}
}
