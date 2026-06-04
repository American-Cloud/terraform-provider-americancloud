package provider

import (
	"context"
	"os"

	acclient "github.com/American-Cloud/americancloud-sdk-go/client"
	"github.com/American-Cloud/americancloud-sdk-go/option"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure americancloudProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*americancloudProvider)(nil)

type americancloudProvider struct {
	// version is set to the running provider's version on build, "dev" otherwise.
	version string
}

// New returns a provider factory bound to the given version string.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &americancloudProvider{version: version}
	}
}

// providerModel maps the provider configuration block.
type providerModel struct {
	APIClientID     types.String `tfsdk:"api_client_id"`
	APIClientSecret types.String `tfsdk:"api_client_secret"`
	APIURL          types.String `tfsdk:"api_url"`
}

func (p *americancloudProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "americancloud"
	resp.Version = p.version
}

func (p *americancloudProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage American Cloud infrastructure — VMs, storage, networking, " +
			"Kubernetes, and DNS — as code. Configure credentials below or via environment variables.",
		Attributes: map[string]schema.Attribute{
			"api_client_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "American Cloud API client ID. May also be set with the " +
					"`AMERICANCLOUD_API_CLIENT_ID` environment variable. Create API keys in the console.",
			},
			"api_client_secret": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "American Cloud API client secret. May also be set with the " +
					"`AMERICANCLOUD_API_CLIENT_SECRET` environment variable.",
			},
			"api_url": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Override the API base URL. May also be set with the " +
					"`AMERICANCLOUD_API_URL` environment variable. Defaults to the production API.",
			},
		},
	}
}

func (p *americancloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Explicit config wins; fall back to environment variables.
	clientID := firstNonEmpty(cfg.APIClientID.ValueString(), os.Getenv("AMERICANCLOUD_API_CLIENT_ID"))
	clientSecret := firstNonEmpty(cfg.APIClientSecret.ValueString(), os.Getenv("AMERICANCLOUD_API_CLIENT_SECRET"))
	baseURL := firstNonEmpty(cfg.APIURL.ValueString(), os.Getenv("AMERICANCLOUD_API_URL"))

	if clientID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_client_id"),
			"Missing API client ID",
			"Set the api_client_id provider attribute or the AMERICANCLOUD_API_CLIENT_ID environment variable.",
		)
	}
	if clientSecret == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_client_secret"),
			"Missing API client secret",
			"Set the api_client_secret provider attribute or the AMERICANCLOUD_API_CLIENT_SECRET environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	opts := []option.RequestOption{
		option.WithAPIKey(clientID),
		option.WithAPIClientSecret(clientSecret),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := acclient.NewClient(opts...)
	// Resources and data sources retrieve this in their Configure methods.
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *americancloudProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDNSZoneResource,
		NewDNSRecordResource,
		NewBlockStorageResource,
		NewSnapshotResource,
		NewSSHKeyResource,
		NewIsolatedNetworkResource,
		NewVPCNetworkResource,
		NewPublicIPResource,
		NewFirewallRuleResource,
		NewVMResource,
		NewKubernetesClusterResource,
		NewObjectStorageUnitResource,
		NewVPCTierResource,
	}
}

func (p *americancloudProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewRegionDataSource,
		NewImageDataSource,
		NewVMPackageDataSource,
	}
}
