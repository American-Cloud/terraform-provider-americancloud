package provider

import (
	"context"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*dnsZoneResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsZoneResource)(nil)
	_ resource.ResourceWithImportState = (*dnsZoneResource)(nil)
)

// NewDNSZoneResource — sdkRef: dnsZones.CreateDNSZones / ListDNSZones / DeleteDNSZones.
func NewDNSZoneResource() resource.Resource { return &dnsZoneResource{} }

type dnsZoneResource struct {
	baseResource
}

type dnsZoneModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (r *dnsZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *dnsZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A hosted DNS zone (domain). Add records with `americancloud_dns_record`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Zone identifier (UUID).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain name for the zone, e.g. `example.com`. Changing this forces a new zone.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the zone was created (RFC 3339).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *dnsZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.client.DNSZones.CreateDNSZones(ctx, &acsdk.CreateZoneDto{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error creating DNS zone", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, zoneModelFromSDK(zone))...)
}

func (r *dnsZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No get-by-id endpoint — page through the list and match on id.
	zone, err := r.findZoneByID(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS zones", err.Error())
		return
	}
	if zone == nil {
		resp.State.RemoveResource(ctx) // gone — let Terraform recreate
		return
	}

	model := zoneModelFromSDK(zone)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update never changes anything: both editable attributes force replacement.
func (r *dnsZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[dnsZoneModel](ctx, req, resp)
}

func (r *dnsZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DNSZones.DeleteDNSZones(ctx, &acsdk.DeleteDNSZonesRequest{ID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting DNS zone", err.Error())
	}
}

func (r *dnsZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findZoneByID locates a zone by id via list-and-match (there is no GET-by-id
// endpoint). Returns (nil, nil) when the zone is absent.
func (r *dnsZoneResource) findZoneByID(ctx context.Context, id string) (*acsdk.ZoneResponseDto, error) {
	return findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.ZoneResponseDto, error) {
			list, err := r.client.DNSZones.ListDNSZones(ctx, &acsdk.ListDNSZonesRequest{Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(z *acsdk.ZoneResponseDto) bool { return z.ID == id },
	)
}

// zoneModelFromSDK maps an SDK zone response into resource state.
func zoneModelFromSDK(zone *acsdk.ZoneResponseDto) dnsZoneModel {
	return dnsZoneModel{
		ID:        types.StringValue(zone.ID),
		Name:      types.StringValue(zone.Name),
		CreatedAt: types.StringValue(zone.CreatedAt.Format(time.RFC3339)),
	}
}
