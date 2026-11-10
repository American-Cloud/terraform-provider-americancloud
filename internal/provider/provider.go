package provider

import (
	"context"
	"net/http"
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
	APIToken        types.String `tfsdk:"api_token"`
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
			"api_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "American Cloud API token (`amcld_…`). May also be set with the " +
					"`AMERICANCLOUD_API_TOKEN` environment variable. Create tokens at " +
					"[console.americancloud.com/api-tokens](https://console.americancloud.com/api-tokens). " +
					"Use a token with the Full access preset; the provider creates and deletes resources.",
			},
			"api_client_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Legacy API key client ID. May also be set with the " +
					"`AMERICANCLOUD_API_CLIENT_ID` environment variable. Use `api_token` for new configurations.",
			},
			"api_client_secret": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Legacy API key client secret. May also be set with the " +
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
	token := firstNonEmpty(cfg.APIToken.ValueString(), os.Getenv("AMERICANCLOUD_API_TOKEN"))
	clientID := firstNonEmpty(cfg.APIClientID.ValueString(), os.Getenv("AMERICANCLOUD_API_CLIENT_ID"))
	clientSecret := firstNonEmpty(cfg.APIClientSecret.ValueString(), os.Getenv("AMERICANCLOUD_API_CLIENT_SECRET"))
	baseURL := firstNonEmpty(cfg.APIURL.ValueString(), os.Getenv("AMERICANCLOUD_API_URL"))

	// One credential: an API token, or the legacy key pair. The API refuses a
	// request that carries both, so the provider refuses the configuration.
	var opts []option.RequestOption
	switch {
	case token != "" && (clientID != "" || clientSecret != ""):
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Two credentials set",
			"Set api_token (AMERICANCLOUD_API_TOKEN) or the api_client_id and api_client_secret pair, not both.",
		)
	case token != "":
		opts = append(opts, option.WithToken(token))
	case clientID != "" && clientSecret != "":
		// The Go SDK 1.8.0 has no option for the legacy pair. Send the two
		// headers directly; the API accepts them as before.
		header := http.Header{}
		header.Set("X-API-Client-ID", clientID)
		header.Set("X-API-Client-Secret", clientSecret)
		opts = append(opts, option.WithHTTPHeader(header))
	case clientID != "" || clientSecret != "":
		resp.Diagnostics.AddAttributeError(
			path.Root("api_client_secret"),
			"Incomplete API key",
			"A legacy API key needs both api_client_id and api_client_secret "+
				"(AMERICANCLOUD_API_CLIENT_ID and AMERICANCLOUD_API_CLIENT_SECRET).",
		)
	default:
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing credential",
			"Set the api_token provider attribute or the AMERICANCLOUD_API_TOKEN environment variable. "+
				"Create a token at https://console.americancloud.com/api-tokens.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
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
		NewPortForwardingRuleResource,
		NewEgressRuleResource,
		NewNetworkACLResource,
		NewNetworkACLRuleResource,
		NewLoadBalancerRuleResource,
		NewVMResource,
		NewKubernetesClusterResource,
		NewObjectStorageUnitResource,
		NewObjectStorageAccessKeyResource,
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
