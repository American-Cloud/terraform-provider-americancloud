package provider

import (
	"context"
	"fmt"
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
	_ resource.Resource                = (*dnsRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsRecordResource)(nil)
	_ resource.ResourceWithImportState = (*dnsRecordResource)(nil)
)

var dnsRecordTypes = []string{"A", "AAAA", "CNAME", "MX", "NS", "SRV", "TXT"}

// NewDNSRecordResource — sdkRef: dnsRecords.CreateDNSRecords / ListDNSRecords / DeleteDNSRecords.
// NOT_EXPOSED: dnsRecords.UpdateDNSRecords — keys on (name,type) only, which is
// ambiguous for multi-value sets; the resource is replace-on-change instead.
func NewDNSRecordResource() resource.Resource { return &dnsRecordResource{} }

type dnsRecordResource struct {
	baseResource
}

type dnsRecordModel struct {
	ID       types.String `tfsdk:"id"`
	ZoneID   types.String `tfsdk:"zone_id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	Content  types.String `tfsdk:"content"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Priority types.Int64  `tfsdk:"priority"`
	Weight   types.Int64  `tfsdk:"weight"`
	Port     types.Int64  `tfsdk:"port"`
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Every attribute forces replacement: the API has no stable record id and its
	// update keys on (name,type) only, so any change is delete-then-create. Delete
	// targets the exact value via content, so multi-value sets are safe.
	requiresReplaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A DNS record within a zone. Any change replaces the record " +
			"(the API has no stable record id). Manage one record value per resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Synthetic identifier: `zoneId/name/type/content`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS zone identifier (from `americancloud_dns_zone`).",
				PlanModifiers:       requiresReplaceStr,
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Hostname relative to the zone. Use `@` for the zone apex.",
				PlanModifiers:       requiresReplaceStr,
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type: one of A, AAAA, CNAME, MX, NS, SRV, TXT.",
				Validators:          []validator.String{stringvalidator.OneOf(dnsRecordTypes...)},
				PlanModifiers:       requiresReplaceStr,
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record value: an IP for A/AAAA, a hostname for CNAME/NS/MX, or text for TXT.",
				PlanModifiers:       requiresReplaceStr,
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Time to live in seconds (60–86400). Omit to use the zone default.",
				Validators:          []validator.Int64{int64validator.Between(60, 86400)},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"priority": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Priority. Required for MX and SRV records.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"weight": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Weight. Required for SRV records.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Port. Required for SRV records.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rec, err := r.client.DNSRecords.CreateDNSRecords(ctx, &acsdk.CreateRecordDto{
		ZoneID:   plan.ZoneID.ValueString(),
		Name:     plan.Name.ValueString(),
		Type:     acsdk.CreateRecordDtoType(plan.Type.ValueString()),
		Content:  plan.Content.ValueString(),
		TTL:      int64ToFloatPtr(plan.TTL),
		Priority: int64ToIntPtr(plan.Priority),
		Weight:   int64ToIntPtr(plan.Weight),
		Port:     int64ToIntPtr(plan.Port),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating DNS record", err.Error())
		return
	}

	state := recordModelFromSDK(plan.ZoneID.ValueString(), rec)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rec, err := r.findRecord(ctx, state.ZoneID.ValueString(), state.Name.ValueString(), state.Type.ValueString(), state.Content.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS records", err.Error())
		return
	}
	if rec == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	model := recordModelFromSDK(state.ZoneID.ValueString(), rec)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update is unreachable: every attribute forces replacement.
func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	setPlanToState[dnsRecordModel](ctx, req, resp)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content := state.Content.ValueString()
	err := r.client.DNSRecords.DeleteDNSRecords(ctx, &acsdk.DeleteRecordBody{
		ZoneID:  state.ZoneID.ValueString(),
		Name:    state.Name.ValueString(),
		Type:    state.Type.ValueString(),
		Content: &content, // disambiguate the exact value in a multi-value set
	})
	if err != nil {
		resp.Diagnostics.AddError("Error deleting DNS record", err.Error())
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Composite id: zoneId/name/type/content (content may contain slashes).
	parts := strings.SplitN(req.ID, "/", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected `zoneId/name/type/content`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("content"), parts[3])...)
}

func (r *dnsRecordResource) findRecord(ctx context.Context, zoneID, name, recordType, content string) (*acsdk.RecordResponseDto, error) {
	return findInPages(ctx,
		func(ctx context.Context, page, pageSize int) ([]*acsdk.RecordResponseDto, error) {
			list, err := r.client.DNSRecords.ListDNSRecords(ctx, &acsdk.ListDNSRecordsRequest{ZoneID: zoneID, Page: &page, PageSize: &pageSize})
			if err != nil {
				return nil, err
			}
			return list.Data, nil
		},
		func(rec *acsdk.RecordResponseDto) bool {
			return rec.Name == name && rec.Type == recordType && rec.Content == content
		},
	)
}

// recordModelFromSDK maps an SDK record response into resource state. zoneID is
// carried separately because the record response doesn't echo it back.
func recordModelFromSDK(zoneID string, rec *acsdk.RecordResponseDto) dnsRecordModel {
	return dnsRecordModel{
		ID:       types.StringValue(dnsRecordID(zoneID, rec.Name, rec.Type, rec.Content)),
		ZoneID:   types.StringValue(zoneID),
		Name:     types.StringValue(rec.Name),
		Type:     types.StringValue(rec.Type),
		Content:  types.StringValue(rec.Content),
		TTL:      types.Int64Value(int64(rec.TTL)),
		Priority: intPtrToInt64(rec.Priority),
		Weight:   intPtrToInt64(rec.Weight),
		Port:     intPtrToInt64(rec.Port),
	}
}

func dnsRecordID(zoneID, name, recordType, content string) string {
	return fmt.Sprintf("%s/%s/%s/%s", zoneID, name, recordType, content)
}
