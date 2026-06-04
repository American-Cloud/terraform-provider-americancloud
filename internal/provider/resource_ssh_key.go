package provider

import (
	"context"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*sshKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*sshKeyResource)(nil)
	_ resource.ResourceWithImportState = (*sshKeyResource)(nil)
)

// NewSSHKeyResource — sdkRef: sshKeys.CreateSSHKeys / ListSSHKeys / DeleteSSHKeys.
// No get-by-id endpoint → Read is list-and-match by name (the key's identity).
func NewSSHKeyResource() resource.Resource { return &sshKeyResource{} }

type sshKeyResource struct{ baseResource }

type sshKeyModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	PublicKey   types.String `tfsdk:"public_key"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An SSH key pair. Provide `public_key` to register an existing key, or omit it to " +
			"have one generated — the `private_key` is then returned once and stored in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Key identifier (the key name).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the SSH key pair. Changing this forces a new key.",
				PlanModifiers:       requiresReplace,
			},
			"public_key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Public key in OpenSSH format. Omit to generate a new pair. Changing it forces a new key.",
				PlanModifiers:       requiresReplace,
			},
			"fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fingerprint of the key.",
			},
			"private_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Generated private key — set only when `public_key` is omitted, returned once at creation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.SSHKeys.CreateSSHKeys(ctx, &acsdk.CreateSSHKeyDto{
		Name:      plan.Name.ValueString(),
		PublicKey: stringToPtr(plan.PublicKey),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating SSH key", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, sshKeyModel{
		ID:          types.StringValue(key.Name),
		Name:        types.StringValue(key.Name),
		PublicKey:   plan.PublicKey, // the response does not echo the public key
		Fingerprint: types.StringValue(key.Fingerprint),
		PrivateKey:  stringPtrToString(key.PrivateKey),
	})...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	item, err := findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.SSHKeyListItemDto, error) {
			list, err := r.client.SSHKeys.ListSSHKeys(ctx, &acsdk.ListSSHKeysRequest{Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(k *acsdk.SSHKeyListItemDto) bool { return k.Name == name },
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading SSH keys", err.Error())
		return
	}
	if item == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	// public_key and private_key are not returned by the list endpoint — preserve them.
	state.ID = types.StringValue(item.Name)
	state.Name = types.StringValue(item.Name)
	state.Fingerprint = types.StringValue(item.Fingerprint)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: name and public_key both force replacement.
func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[sshKeyModel](ctx, req, resp)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SSHKeys.DeleteSSHKeys(ctx, &acsdk.DeleteSSHKeysRequest{Name: state.Name.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting SSH key", err.Error())
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Identity is the key name; populate both id and name so Read can match.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
