# Terraform Provider for American Cloud

Manage [American Cloud](https://americancloud.com) infrastructure — VMs, block
storage, networking, Kubernetes, and DNS — as code.

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
  # Credentials may be set here or via environment variables:
  #   AMERICANCLOUD_API_CLIENT_ID, AMERICANCLOUD_API_CLIENT_SECRET
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

Create API keys at **[console.americancloud.com](https://console.americancloud.com)**.

## Resources

`americancloud_dns_zone` · `americancloud_dns_record` · `americancloud_block_storage` ·
`americancloud_snapshot` · `americancloud_isolated_network` · `americancloud_vpc_network` ·
`americancloud_public_ip` · `americancloud_firewall_rule` · `americancloud_ssh_key` ·
`americancloud_vm` · `americancloud_kubernetes_cluster`.

See [`examples/`](./examples) for per-resource configurations.

## Configuration

| Provider setting | Environment variable | Purpose |
|---|---|---|
| `api_client_id` | `AMERICANCLOUD_API_CLIENT_ID` | API client ID |
| `api_client_secret` | `AMERICANCLOUD_API_CLIENT_SECRET` | API client secret (sensitive) |
| `api_url` | `AMERICANCLOUD_API_URL` | API base URL override (optional) |

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
