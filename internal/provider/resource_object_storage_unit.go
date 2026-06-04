package provider

import (
	"context"
	"strings"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                = (*objectStorageUnitResource)(nil)
	_ resource.ResourceWithConfigure   = (*objectStorageUnitResource)(nil)
	_ resource.ResourceWithImportState = (*objectStorageUnitResource)(nil)
)

// NewObjectStorageUnitResource — sdkRef: objectStorage.CreateUnitObjectStorage /
// ListUnitsObjectStorage / GetKeysObjectStorage / SetUserQuotaObjectStorage /
// DeleteUnitObjectStorage. No get-by-id endpoint → Read is list-and-match by
// storage-unit id. name forces replacement; max_size_gb is declarable quota,
// applied via the SetUserQuota op (set / update in place / RemoveLimit on unset);
// access_key/secret_key are computed sensitive outputs fetched after create and
// backfilled on Read (the kubeconfig pattern).
// NOT_EXPOSED: objectStorage.CreateBucketObjectStorage / DeleteBucketObjectStorage /
// ListBucketsObjectStorage (buckets land as a follow-up sub-resource);
// objectStorage.GetCostEstimateObjectStorage (cost preview).
func NewObjectStorageUnitResource() resource.Resource { return &objectStorageUnitResource{} }

type objectStorageUnitResource struct{ baseResource }

type objectStorageUnitModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	MaxSizeGb  types.Int64  `tfsdk:"max_size_gb"`
	MaxBuckets types.Int64  `tfsdk:"max_buckets"`
	AccessKey  types.String `tfsdk:"access_key"`
	SecretKey  types.String `tfsdk:"secret_key"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (r *objectStorageUnitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_unit"
}

func (r *objectStorageUnitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An object storage unit — an S3-compatible storage account that holds buckets. " +
			"Use its `access_key`/`secret_key` with any S3 client against the endpoint " +
			"`https://a2-west.americancloud.com`. " +
			"`max_size_gb` is updatable in place; changing the name replaces the unit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Storage unit identifier.", PlanModifiers: useState},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage unit name (alphanumeric). Changing it forces a new unit.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"max_size_gb": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Storage quota for the unit, in gigabytes. Omit for unlimited; removing it lifts an existing limit. Updatable in place.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
			"max_buckets": schema.Int64Attribute{
				Computed: true, MarkdownDescription: "Maximum number of buckets the unit may hold (null means unlimited).",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"access_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				MarkdownDescription: "S3 access key for the unit. Sensitive — grants storage access.",
				PlanModifiers:       useState,
			},
			"secret_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				MarkdownDescription: "S3 secret key for the unit. Sensitive — grants storage access.",
				PlanModifiers:       useState,
			},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "When the unit was created (RFC 3339).", PlanModifiers: useState},
		},
	}
}

func (r *objectStorageUnitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageUnitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unit, err := r.client.ObjectStorage.CreateUnitObjectStorage(ctx, &acsdk.CreateStorageUnitRequestDto{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error creating object storage unit", err.Error())
		return
	}

	// name isn't echoed by the API — carry the planned value; fill the rest.
	plan.ID = types.StringValue(unit.StorageUnitID)
	plan.MaxBuckets = floatPtrToInt64(unit.MaxBuckets)
	plan.CreatedAt = types.StringValue(unit.CreatedAt.Format(time.RFC3339))
	plan.AccessKey = types.StringNull()
	plan.SecretKey = types.StringNull()

	// Quota is applied as a separate op after create. On failure, persist the id
	// first (the unit exists — never orphan it) and fail the apply.
	if !plan.MaxSizeGb.IsNull() {
		if _, qerr := r.client.ObjectStorage.SetUserQuotaObjectStorage(ctx, &acsdk.SetUserQuotaRequestDto{
			StorageUnitID: unit.StorageUnitID,
			MaxSizeGb:     int64ToFloatPtr(plan.MaxSizeGb),
		}); qerr != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Error setting object storage unit quota", qerr.Error())
			return
		}
	}

	// Fetch the unit's S3 keys. Best-effort: don't fail the apply — surface a
	// warning and let the next Read backfill them (the kubeconfig pattern).
	if keys, kerr := r.client.ObjectStorage.GetKeysObjectStorage(ctx, &acsdk.GetKeysObjectStorageRequest{StorageUnitID: unit.StorageUnitID}); kerr == nil {
		plan.AccessKey = types.StringValue(keys.AccessKey)
		plan.SecretKey = types.StringValue(keys.SecretKey)
	} else {
		resp.Diagnostics.AddWarning(
			"Object storage keys not yet available",
			"The storage unit was created but its access keys could not be fetched: "+kerr.Error()+
				". Run `terraform refresh` (or the next plan/apply) to populate them.",
		)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageUnitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageUnitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No get-by-id endpoint — page the list and match on the storage-unit id.
	id := state.ID.ValueString()
	unit, err := findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.ObjectStorageUnitDto, error) {
			list, err := r.client.ObjectStorage.ListUnitsObjectStorage(ctx, &acsdk.ListUnitsObjectStorageRequest{Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(u *acsdk.ObjectStorageUnitDto) bool { return u.StorageUnitID == id },
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading object storage units", err.Error())
		return
	}
	if unit == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// name has no field of its own in the response, but the storage-unit id is
	// "<account>$<name>" (the RGW user id) — recover the name from it so import
	// round-trips. keepStr keeps the user's value on a normal refresh.
	state.Name = keepStr(state.Name, nameFromStorageUnitID(unit.StorageUnitID))
	state.ID = types.StringValue(unit.StorageUnitID)
	// max_size_gb is mutable (quota op). The list response does NOT currently
	// echo the quota fields (limitKb stays null even right after a successful
	// SetUserQuota — verified 2026-06-05, API-side gap), so a
	// null limit is indistinguishable from "not echoed": hydrate from limitKb
	// only when it is present (forward-compatible drift detection), otherwise
	// preserve the configured value. Until the API populates it, out-of-band
	// quota changes don't surface as drift and import can't recover the value
	// (re-applying converges by re-setting the quota).
	if unit.LimitKb != nil {
		state.MaxSizeGb = types.Int64Value(int64(*unit.LimitKb / (1024 * 1024)))
	}
	state.MaxBuckets = floatPtrToInt64(unit.MaxBuckets)
	state.CreatedAt = types.StringValue(unit.CreatedAt.Format(time.RFC3339))
	// Backfill the S3 keys if they aren't in state yet (create-time fetch failed,
	// or the unit was imported). Best-effort — never fail Read over it.
	if state.AccessKey.IsNull() || state.AccessKey.ValueString() == "" {
		if keys, kerr := r.client.ObjectStorage.GetKeysObjectStorage(ctx, &acsdk.GetKeysObjectStorageRequest{StorageUnitID: id}); kerr == nil {
			state.AccessKey = types.StringValue(keys.AccessKey)
			state.SecretKey = types.StringValue(keys.SecretKey)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place quota changes via SetUserQuotaObjectStorage: a new
// max_size_gb sets the limit, unsetting it lifts the limit (RemoveLimit).
// Everything else is ForceNew or computed.
func (r *objectStorageUnitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageUnitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.MaxSizeGb.Equal(state.MaxSizeGb) {
		quota := &acsdk.SetUserQuotaRequestDto{StorageUnitID: state.ID.ValueString()}
		if plan.MaxSizeGb.IsNull() {
			quota.RemoveLimit = acsdk.Bool(true)
		} else {
			quota.MaxSizeGb = int64ToFloatPtr(plan.MaxSizeGb)
		}
		if _, err := r.client.ObjectStorage.SetUserQuotaObjectStorage(ctx, quota); err != nil {
			resp.Diagnostics.AddError("Error setting object storage unit quota", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageUnitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageUnitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.ObjectStorage.DeleteUnitObjectStorage(ctx, &acsdk.DeleteUnitObjectStorageRequest{StorageUnitID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting object storage unit", err.Error())
	}
}

func (r *objectStorageUnitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// nameFromStorageUnitID recovers the unit name from a storage-unit id of the
// form "<account>$<name>" (the RGW user id). Returns the whole id if there's no
// separator. The API doesn't return the name as its own field, so this is how
// Read/import reconstruct it.
func nameFromStorageUnitID(id string) string {
	if i := strings.LastIndex(id, "$"); i >= 0 {
		return id[i+1:]
	}
	return id
}
