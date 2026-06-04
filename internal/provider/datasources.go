package provider

import (
	"context"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Read-only by-label lookups that resolve the labels used by resource arguments
// (region/image/vm_package) to their canonical ids + details.

// ── region ──────────────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = (*regionDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*regionDataSource)(nil)
)

// NewRegionDataSource — sdkRef: regions.GetByLabelRegions.
func NewRegionDataSource() datasource.DataSource { return &regionDataSource{} }

type regionDataSource struct{ baseDataSource }

type regionDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Label       types.String `tfsdk:"label"`
	DisplayName types.String `tfsdk:"display_name"`
}

func (d *regionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_region"
}

func (d *regionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a region by its label.",
		Attributes: map[string]schema.Attribute{
			"label":        schema.StringAttribute{Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`."},
			"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Region identifier (UUID)."},
			"display_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable region name."},
		},
	}
}

func (d *regionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg regionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	region, err := d.client.Regions.GetByLabelRegions(ctx, &acsdk.GetByLabelRegionsRequest{Label: cfg.Label.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error looking up region", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, regionDataSourceModel{
		ID:          types.StringValue(region.ID),
		Label:       types.StringValue(region.Label),
		DisplayName: types.StringValue(region.DisplayName),
	})...)
}

// ── image ───────────────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = (*imageDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*imageDataSource)(nil)
)

// NewImageDataSource — sdkRef: images.GetByLabelImages.
func NewImageDataSource() datasource.DataSource { return &imageDataSource{} }

type imageDataSource struct{ baseDataSource }

type imageDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Label       types.String `tfsdk:"label"`
	Description types.String `tfsdk:"description"`
	Os          types.String `tfsdk:"os"`
	Version     types.String `tfsdk:"version"`
}

func (d *imageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image"
}

func (d *imageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an OS image by its label.",
		Attributes: map[string]schema.Attribute{
			"label":       schema.StringAttribute{Required: true, MarkdownDescription: "Image label, e.g. `ubuntu-24.04-050826`."},
			"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Image identifier (UUID)."},
			"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Image description."},
			"os":          schema.StringAttribute{Computed: true, MarkdownDescription: "Operating system."},
			"version":     schema.StringAttribute{Computed: true, MarkdownDescription: "OS version."},
		},
	}
}

func (d *imageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg imageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	img, err := d.client.Images.GetByLabelImages(ctx, &acsdk.GetByLabelImagesRequest{Label: cfg.Label.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error looking up image", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, imageDataSourceModel{
		ID:          types.StringValue(img.ID),
		Label:       types.StringValue(img.Label),
		Description: types.StringValue(img.Description),
		Os:          types.StringValue(img.Os),
		Version:     types.StringValue(img.Version),
	})...)
}

// ── vm_package ──────────────────────────────────────────────────────────────

var (
	_ datasource.DataSource              = (*vmPackageDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*vmPackageDataSource)(nil)
)

// NewVMPackageDataSource — sdkRef: vmPackages.GetByLabelVMPackages.
func NewVMPackageDataSource() datasource.DataSource { return &vmPackageDataSource{} }

type vmPackageDataSource struct{ baseDataSource }

type vmPackageDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Label         types.String `tfsdk:"label"`
	Description   types.String `tfsdk:"description"`
	Tier          types.String `tfsdk:"tier"`
	MinCPU        types.Int64  `tfsdk:"min_cpu"`
	MaxCPU        types.Int64  `tfsdk:"max_cpu"`
	MinMemoryMb   types.Int64  `tfsdk:"min_memory_mb"`
	MaxMemoryMb   types.Int64  `tfsdk:"max_memory_mb"`
	MinRootDiskGb types.Int64  `tfsdk:"min_root_disk_gb"`
	MaxRootDiskGb types.Int64  `tfsdk:"max_root_disk_gb"`
}

func (d *vmPackageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_package"
}

func (d *vmPackageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computedInt := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a VM package (compute tier) by its label, including its CPU/memory/disk limits.",
		Attributes: map[string]schema.Attribute{
			"label":            schema.StringAttribute{Required: true, MarkdownDescription: "VM package label, e.g. `standard-custom`."},
			"id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Package identifier (UUID)."},
			"description":      schema.StringAttribute{Computed: true, MarkdownDescription: "Package description."},
			"tier":             schema.StringAttribute{Computed: true, MarkdownDescription: "Service tier."},
			"min_cpu":          computedInt("Minimum vCPUs."),
			"max_cpu":          computedInt("Maximum vCPUs."),
			"min_memory_mb":    computedInt("Minimum memory (MB)."),
			"max_memory_mb":    computedInt("Maximum memory (MB)."),
			"min_root_disk_gb": computedInt("Minimum root disk (GB)."),
			"max_root_disk_gb": computedInt("Maximum root disk (GB)."),
		},
	}
}

func (d *vmPackageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg vmPackageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pkg, err := d.client.VMPackages.GetByLabelVMPackages(ctx, &acsdk.GetByLabelVMPackagesRequest{Label: cfg.Label.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Error looking up VM package", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, vmPackageDataSourceModel{
		ID:            types.StringValue(pkg.ID),
		Label:         types.StringValue(pkg.Label),
		Description:   types.StringValue(pkg.Description),
		Tier:          types.StringValue(pkg.Tier),
		MinCPU:        types.Int64Value(int64(pkg.MinCPU)),
		MaxCPU:        types.Int64Value(int64(pkg.MaxCPU)),
		MinMemoryMb:   types.Int64Value(int64(pkg.MinMemoryMb)),
		MaxMemoryMb:   types.Int64Value(int64(pkg.MaxMemoryMb)),
		MinRootDiskGb: types.Int64Value(int64(pkg.MinRootDiskGb)),
		MaxRootDiskGb: types.Int64Value(int64(pkg.MaxRootDiskGb)),
	})...)
}
