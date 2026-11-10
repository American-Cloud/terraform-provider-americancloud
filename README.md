# Terraform Provider for American Cloud

Manage [American Cloud](https://americancloud.com) infrastructure — VMs, block
storage, networking, Kubernetes, and DNS — as code.

![terraform apply with the American Cloud provider, then a clean re-plan showing no drift](https://raw.githubusercontent.com/American-Cloud/terraform-provider-americancloud/master/.demo/tf-apply.gif)

## Usage

```hcl
terraform {
  required_providers {
    americancloud = {
      source = "American-Cloud/americancloud"
    }
  }
}

provider "americancloud" {
  # Credentials may be set here or via the environment variable
  #   AMERICANCLOUD_API_TOKEN
}

resource "americancloud_isolated_network" "net" {
  name   = "app-net"
  region = "us-west-0"
}

resource "americancloud_vm" "web" {
  name                = "web-1"
  region              = "us-west-0"
  vm_package          = "standard-custom"
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04-050826"
  network             = americancloud_isolated_network.net.id
  subscription_period = "hourly"
}
```

Create an API token at
**[console.americancloud.com/api-tokens](https://console.americancloud.com/api-tokens)**
with the **Full access** preset; the provider creates and deletes resources.
A legacy API key pair (`api_client_id` and `api_client_secret`) still works.

## Resources

`americancloud_vm` · `americancloud_ssh_key` · `americancloud_snapshot` ·
`americancloud_block_storage` · `americancloud_kubernetes_cluster` ·
`americancloud_isolated_network` · `americancloud_vpc_network` · `americancloud_vpc_tier` ·
`americancloud_public_ip` · `americancloud_firewall_rule` · `americancloud_egress_rule` ·
`americancloud_network_acl` · `americancloud_network_acl_rule` ·
`americancloud_port_forwarding_rule` · `americancloud_load_balancer_rule` ·
`americancloud_object_storage_unit` · `americancloud_object_storage_access_key` ·
`americancloud_dns_zone` · `americancloud_dns_record`.

**Data sources:** `americancloud_region` · `americancloud_image` · `americancloud_vm_package`.

See [`examples/`](./examples) for per-resource configurations, the
[registry documentation](https://registry.terraform.io/providers/American-Cloud/americancloud/latest/docs)
for the full schema, and the [Terraform guide](https://americancloud.com/docs/sdks/terraform)
for a walkthrough.

## Configuration

| Provider setting | Environment variable | Purpose |
|---|---|---|
| `api_token` | `AMERICANCLOUD_API_TOKEN` | API token, Full access preset (sensitive). Create one at [console.americancloud.com/api-tokens](https://console.americancloud.com/api-tokens) |
| `api_client_id` | `AMERICANCLOUD_API_CLIENT_ID` | Legacy API key client ID |
| `api_client_secret` | `AMERICANCLOUD_API_CLIENT_SECRET` | Legacy API key client secret (sensitive) |
| `api_url` | `AMERICANCLOUD_API_URL` | API base URL override (optional) |

## Guides

Narrative walkthroughs on the American Cloud docs site:

- [Terraform guide](https://americancloud.com/docs/sdks/terraform) — provider setup, authentication, and managing resources as code
- [SDKs & Terraform overview](https://americancloud.com/docs/sdks/overview) — when to reach for Terraform, an SDK, or the MCP server
- [MCP server](https://americancloud.com/docs/mcp/overview) — manage the same resources from Claude, ChatGPT, Cursor, and other AI assistants
- [Deploy with AI](https://americancloud.com/docs/deploy-with-ai/overview) — let an AI assistant build and ship to American Cloud

## Development

```sh
go build ./...
go test ./...   # unit + SDK coverage (no credentials needed)
go install .    # then configure dev_overrides — see CONTRIBUTING.md
```

## Contributing

The resource surface tracks the American Cloud Go SDK and API — see
[`CONTRIBUTING.md`](./CONTRIBUTING.md).

## License

Apache-2.0 — see [`LICENSE`](./LICENSE) and [`NOTICE`](./NOTICE).
