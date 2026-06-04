# Contributing

Thanks for your interest in the American Cloud Terraform provider.

## Generated from our API/SDK

The resource surface tracks the
[American Cloud Go SDK](https://github.com/American-Cloud/americancloud-sdk-go) and
the American Cloud API. Most missing or incorrect resources, attributes, or
schemas trace back to the API or its OpenAPI spec — [open an issue](../../issues)
describing what you expected, and fixing it upstream flows into the next release.

Docs, README, and packaging fixes are welcome as pull requests.

## Local development

Build and install the provider, then point Terraform at the local binary with a
dev override (no `terraform init` needed):

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "American-Cloud/americancloud" = "<your-GOPATH>/bin"
  }
  direct {}
}
```

```sh
go install .
go test ./...          # unit + SDK coverage
# In an examples dir, export AMERICANCLOUD_API_CLIENT_ID/_SECRET, then:
terraform plan
```

## Secret scanning (pre-commit hook)

This repo ships a [gitleaks](https://github.com/gitleaks/gitleaks) pre-commit
hook in `.githooks/` that blocks commits containing secrets. Git hook paths
aren't enabled automatically, so turn it on once per clone:

```sh
brew install gitleaks            # or see gitleaks' install docs
git config core.hooksPath .githooks
```

The hook scans staged changes and fails closed (a missing gitleaks blocks the
commit). To scan the whole tree manually: `gitleaks dir . --redact`.

## Reporting security issues

Email **security@americancloud.com** rather than opening a public issue.
