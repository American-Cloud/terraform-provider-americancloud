# Versioning

The provider carries its own semantic version. **The compatibility contract is
the exact-pinned `americancloud-sdk-go` dependency**, which is lockstep with the
API platform version — so the pin transitively states the API surface the
provider was built and tested against:

> `terraform-provider-americancloud 0.1.x` ↔ `americancloud-sdk-go 1.3.1` ↔ API platform `1.3.1`

Rules:

- **Patch** — documentation/internal fixes; no schema change.
- **Minor** — new resources or data sources, or an SDK pin bump with additive
  surface changes.
- **Major** — removed/renamed resources or changed attribute shapes (whether from
  an SDK breaking change or our own redesign), and any move to a new API version.
  Breaking attribute changes are called out prominently in the CHANGELOG, since
  they can force resource replacement in customer state.

A new exact `americancloud-sdk-go` pin is the only event that drives resource work.
