package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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
	_ resource.Resource                = (*objectStorageAccessKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*objectStorageAccessKeyResource)(nil)
	_ resource.ResourceWithImportState = (*objectStorageAccessKeyResource)(nil)
)

// NewObjectStorageAccessKeyResource — sdkRef: objectStorage.CreateAccessKeyObjectStorage /
// ListAccessKeysObjectStorage / DeleteAccessKeyObjectStorage. No get-by-id endpoint → Read is
// list-and-match on the access key. The API has no update and a label cannot change after
// create, so every argument forces replacement. The id is "<storage_unit_id>/<access_key>".
//
// The API refuses a change with 409 operation_busy while another key request for the same
// unit runs, and with 429 when the caller sends too many key requests. Nothing changes in
// either case, so Create and Delete retry them until their timeout. A 409
// operation_in_progress means the request ran past its deadline and can still finish, so
// Create does not send it again: it waits for the new key to show in the list.
func NewObjectStorageAccessKeyResource() resource.Resource {
	return &objectStorageAccessKeyResource{}
}

type objectStorageAccessKeyResource struct{ baseResource }

type objectStorageAccessKeyModel struct {
	ID            types.String   `tfsdk:"id"`
	StorageUnitID types.String   `tfsdk:"storage_unit_id"`
	Label         types.String   `tfsdk:"label"`
	AccessKey     types.String   `tfsdk:"access_key"`
	SecretKey     types.String   `tfsdk:"secret_key"`
	CreatedAt     types.String   `tfsdk:"created_at"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

const (
	accessKeyCreateTimeout = 5 * time.Minute
	accessKeyDeleteTimeout = 5 * time.Minute
	accessKeyLabelMaxRunes = 64
)

// accessKeyLabelPattern is the API's label rule. RE2 has no lookahead, so "a space not
// followed by a space" is written as "a space followed by an allowed character". That also
// refuses a trailing space: the API trims it, and the label in state would then differ from
// the config.
var accessKeyLabelPattern = regexp.MustCompile(
	`^[\p{L}\p{N}](?:[\p{L}\p{N}._\-:,/@#()+&']| [\p{L}\p{N}._\-:,/@#()+&'])*$`,
)

func (r *objectStorageAccessKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_object_storage_access_key"
}

func (r *objectStorageAccessKeyResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An extra S3 access key for an object storage unit. Every key of a unit works at " +
			"the same time, so you can give each application its own key and rotate one key without " +
			"affecting the others. A unit holds up to 10 keys. The unit's original key is the " +
			"`access_key`/`secret_key` of `americancloud_object_storage_unit`. Changing any argument " +
			"replaces the key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the form `<storage_unit_id>/<access_key>`.",
				PlanModifiers:       useState,
			},
			"storage_unit_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The `id` of the `americancloud_object_storage_unit` that holds the key. Changing it replaces the key.",
				PlanModifiers:       replace,
			},
			"label": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "A label that identifies the key, for example the application that uses it. " +
					"1 to 64 characters; it starts with a letter or a number and holds only letters, numbers, " +
					"single spaces, and `. _ - : , / @ # ( ) + & '`. Changing it replaces the key.",
				PlanModifiers: replace,
				Validators: []validator.String{
					stringvalidator.UTF8LengthBetween(1, accessKeyLabelMaxRunes),
					stringvalidator.RegexMatches(accessKeyLabelPattern,
						"must start with a letter or a number, hold only letters, numbers, single spaces and . _ - : , / @ # ( ) + & ', and not end with a space"),
				},
			},
			"access_key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The S3 access key ID.",
				PlanModifiers:       useState,
			},
			"secret_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The S3 secret key. Sensitive — grants storage access.",
				PlanModifiers:       useState,
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the key was created (RFC 3339).",
				PlanModifiers:       useState,
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func (r *objectStorageAccessKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageAccessKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createTimeout, diags := plan.Timeouts.Create(ctx, accessKeyCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	unitID := plan.StorageUnitID.ValueString()
	label := plan.Label.ValueString()

	// Record the keys that exist now. If the create runs past its deadline, the new key is the
	// one that is not in this set.
	before, err := r.listKeys(ctx, unitID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing object storage access keys", err.Error())
		return
	}
	existing := make(map[string]bool, len(before))
	for _, k := range before {
		existing[k.AccessKey] = true
	}

	var created *acsdk.ObjectStorageAccessKeyDto
	waitingForKey := false
	err = pollUntil(ctx, func(ctx context.Context) (bool, error) {
		if waitingForKey {
			key, ferr := r.findNewKey(ctx, unitID, label, existing)
			if ferr != nil || key == nil {
				return false, ferr
			}
			created = key
			return true, nil
		}
		key, cerr := r.client.ObjectStorage.CreateAccessKeyObjectStorage(ctx, &acsdk.CreateAccessKeyRequestDto{
			StorageUnitID: unitID,
			Label:         label,
		})
		switch {
		case cerr == nil:
			created = key
			return true, nil
		case accessKeyRequestRefused(cerr):
			return false, nil // nothing changed — send it again
		case conflictCode(cerr) == "operation_in_progress":
			waitingForKey = true // the create can still finish — never send a second one
			return false, nil
		default:
			return false, accessKeyError(cerr)
		}
	})
	if err != nil {
		if waitingForKey {
			err = fmt.Errorf("%w. The key can still appear: check the keys of storage unit %s, then "+
				"import it with `terraform import <address> %s/<access_key>`", err, unitID, unitID)
		}
		resp.Diagnostics.AddError("Error creating object storage access key", err.Error())
		return
	}

	plan.ID = types.StringValue(unitID + "/" + created.AccessKey)
	plan.AccessKey = types.StringValue(created.AccessKey)
	plan.SecretKey = types.StringValue(created.SecretKey)
	plan.CreatedAt = timePtrToString(created.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageAccessKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageAccessKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	unitID, accessKey := state.StorageUnitID.ValueString(), state.AccessKey.ValueString()
	key, err := r.findKey(ctx, unitID, accessKey)
	if err != nil {
		if isNotFound(err) || apiStatusCode(err) == 404 {
			resp.State.RemoveResource(ctx) // the storage unit is gone, and its keys with it
			return
		}
		resp.Diagnostics.AddError("Error reading object storage access keys", err.Error())
		return
	}
	if key == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(unitID + "/" + key.AccessKey)
	state.AccessKey = types.StringValue(key.AccessKey)
	state.SecretKey = types.StringValue(key.SecretKey)
	// The label cannot change after create, so keep the configured value (the API stores the
	// NFC form, which can differ in bytes). Import has no value yet and takes the API's.
	apiLabel := ""
	if key.Label != nil {
		apiLabel = *key.Label
	}
	state.Label = keepStr(state.Label, apiLabel)
	state.CreatedAt = timePtrToString(key.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached for a real change: every argument forces replacement.
func (r *objectStorageAccessKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[objectStorageAccessKeyModel](ctx, req, resp)
}

func (r *objectStorageAccessKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageAccessKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteTimeout, diags := state.Timeouts.Delete(ctx, accessKeyDeleteTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	unitID, accessKey := state.StorageUnitID.ValueString(), state.AccessKey.ValueString()
	err := pollUntil(ctx, func(ctx context.Context) (bool, error) {
		derr := r.client.ObjectStorage.DeleteAccessKeyObjectStorage(ctx, &acsdk.DeleteAccessKeyObjectStorageRequest{
			StorageUnitID: unitID,
			AccessKey:     accessKey,
		})
		switch {
		case derr == nil, isNotFound(derr), apiStatusCode(derr) == 404:
			return true, nil // deleted, or already gone
		case accessKeyRequestRefused(derr), conflictCode(derr) == "operation_in_progress":
			// Refused, or still running: send it again. A repeat of a finished delete answers 404.
			return false, nil
		default:
			return false, accessKeyError(derr)
		}
	})
	if err != nil {
		resp.Diagnostics.AddError("Error deleting object storage access key", err.Error())
	}
}

// ImportState takes "<storage_unit_id>/<access_key>". It refuses the unit's original key:
// that key belongs to americancloud_object_storage_unit, and it has no label to manage.
func (r *objectStorageAccessKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	i := strings.LastIndex(req.ID, "/")
	if i <= 0 || i == len(req.ID)-1 {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("Expected \"<storage_unit_id>/<access_key>\", got %q.", req.ID))
		return
	}
	unitID, accessKey := req.ID[:i], req.ID[i+1:]

	key, err := r.findKey(ctx, unitID, accessKey)
	if err != nil {
		resp.Diagnostics.AddError("Error reading object storage access keys", err.Error())
		return
	}
	if key == nil {
		resp.Diagnostics.AddError("Access key not found",
			fmt.Sprintf("Storage unit %s has no access key %s.", unitID, accessKey))
		return
	}
	if key.Label == nil {
		resp.Diagnostics.AddError("Cannot import the original key",
			"This is the storage unit's original key. Manage it through the access_key and "+
				"secret_key of americancloud_object_storage_unit.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("storage_unit_id"), unitID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_key"), accessKey)...)
}

func (r *objectStorageAccessKeyResource) listPage(unitID string) func(context.Context, int, int) ([]*acsdk.ObjectStorageAccessKeyDto, error) {
	return func(ctx context.Context, page, pageSize int) ([]*acsdk.ObjectStorageAccessKeyDto, error) {
		list, err := r.client.ObjectStorage.ListAccessKeysObjectStorage(ctx, &acsdk.ListAccessKeysObjectStorageRequest{
			StorageUnitID: unitID,
			Page:          &page,
			PageSize:      &pageSize,
		})
		if err != nil {
			return nil, err
		}
		return list.Data, nil
	}
}

func (r *objectStorageAccessKeyResource) listKeys(ctx context.Context, unitID string) ([]*acsdk.ObjectStorageAccessKeyDto, error) {
	return collectPages(ctx, r.listPage(unitID))
}

func (r *objectStorageAccessKeyResource) findKey(ctx context.Context, unitID, accessKey string) (*acsdk.ObjectStorageAccessKeyDto, error) {
	return findInPages(ctx, r.listPage(unitID), func(k *acsdk.ObjectStorageAccessKeyDto) bool {
		return k.AccessKey == accessKey
	})
}

// findNewKey returns the one key that is not in existing and has the label, or nil while no
// such key exists. Two such keys mean that something else created a key with the same
// label at the same time, so neither one can be claimed.
func (r *objectStorageAccessKeyResource) findNewKey(ctx context.Context, unitID, label string, existing map[string]bool) (*acsdk.ObjectStorageAccessKeyDto, error) {
	keys, err := r.listKeys(ctx, unitID)
	if err != nil {
		return nil, err
	}
	var match *acsdk.ObjectStorageAccessKeyDto
	for _, k := range keys {
		if existing[k.AccessKey] || k.Label == nil || *k.Label != label {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("two new access keys on storage unit %s have the label %q, so the "+
				"provider cannot tell which one it created; delete the extra key, then import the other", unitID, label)
		}
		match = k
	}
	return match, nil
}

// accessKeyRequestRefused reports whether the API refused a key request before it started,
// so that sending it again is safe: 409 operation_busy (another key request runs) or 429
// (too many key requests from this caller).
func accessKeyRequestRefused(err error) bool {
	return conflictCode(err) == "operation_busy" || apiStatusCode(err) == 429
}

// accessKeyError gives the known refusals a message that says what to do.
func accessKeyError(err error) error {
	switch conflictCode(err) {
	case "access_key_limit_reached":
		return fmt.Errorf("the storage unit already holds the maximum number of access keys; remove a key first: %w", err)
	case "last_access_key":
		return fmt.Errorf("this is the last access key of the storage unit, and a unit always keeps one key; "+
			"add another key first, or delete the storage unit: %w", err)
	}
	return err
}
