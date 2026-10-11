# Changelog

All notable changes to the American Cloud Terraform provider are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-10-10

### Changed

- Built against `americancloud-sdk-go` 1.6.0 (API platform 1.6.0).
- The API now protects the rules that a managed Kubernetes cluster depends on.
  A plan that removes or changes the cluster's port 6443 load balancer rule, its
  port 2222+ SSH forwarding rules or the firewall rules that open them, releases
  the cluster's public IP, or changes the source NAT address of its network is
  refused by the API with a message that names the cluster. Scaling or upgrading
  a cluster whose port 6443 rule is already gone is refused the same way, with
  nothing changed.
- `americancloud_kubernetes_cluster` reads `kubeconfig` with a read-write API
  key only. The API now requires manage access for it. With a read-only key the
  apply still succeeds; the provider warns that the kubeconfig needs a read-write
  key and leaves the attribute empty.
- `americancloud_object_storage_unit` `name` accepts hyphens and underscores:
  letters, digits, hyphens and underscores, up to 100 characters, starting and
  ending with a letter or digit. The names `anonymous` and `RGW` followed by 17
  digits are reserved. The documentation said alphanumeric only.
- A create of `americancloud_vm` or `americancloud_kubernetes_cluster` reads
  its record at once. The API returns the new resource right after create, so
  the first poll no longer waits through a not-found window.

## [0.5.0] - 2026-10-05

### Added

- New resource `americancloud_object_storage_access_key`: an extra S3 access key
  for an object storage unit. Every key of a unit works at the same time, so each
  application can have its own key, and you can rotate one key without touching
  the others. A unit holds up to 10 keys. Changing `storage_unit_id` or `label`
  replaces the key. `secret_key` is sensitive. Import with
  `<storage_unit_id>/<access_key>`. The unit's original key stays the
  `access_key`/`secret_key` of `americancloud_object_storage_unit`.
- When the API answers that another key request for the same unit is running, or
  that you sent too many key requests, the provider waits and retries until the
  create or delete timeout. Several keys on one unit in one apply therefore work
  with the default parallelism.

### Changed

- Built against `americancloud-sdk-go` 1.5.0 (API platform 1.5.0).
- The `americancloud_object_storage_unit` documentation says that its
  `access_key`/`secret_key` are the unit's original key.
- The `egress_rule` and `isolated_network` documentation now lists the
  `action` and `default_egress_policy` attributes.

### Security

- The provider now builds with Go 1.26.8. This release includes the Go fixes for
  `crypto/tls`, `net/http`, `net/url` and `encoding/asn1`.
- Updated `google.golang.org/grpc` to v1.83.2, `golang.org/x/net` to v0.59.0 and
  `golang.org/x/text` to v0.42.0. These versions fix the known advisories in the
  plugin server and in host-name handling. The provider behavior does not change.

## [0.4.1] - 2026-09-30

### Changed

- **`vm.keypairs`** — the description now names `cloud` as the default login
  user on the Linux images (`ssh cloud@<public-ip>`). It said `root` before.
  Nothing in the provider behavior changes.

## [0.4.0] - 2026-08-26

### Added

- **`isolated_network.default_egress_policy`** — how outbound traffic is treated
  when the network has no egress rules. `allow` permits all outbound traffic and
  each egress rule blocks what it matches; `deny` blocks all outbound traffic and
  each egress rule permits what it matches. The platform fixes it when the
  network is created and it cannot be changed afterwards, so the attribute is
  read-only.
- **`egress_rule.action`** — whether the rule permits or blocks the traffic it
  matches, `allow` or `deny`. The rule's network decides this through its
  `default_egress_policy`, not the rule itself. Read-only.
- Deleting a VM or a block storage volume whose disk still has snapshots now
  reports which snapshots block it, by name and id, instead of the raw API
  error. Delete those snapshots, then retry.

### Changed

- SDK pin bumped to `americancloud-sdk-go` v1.4.0 (API 1.4.0).
- `vm.subscription_period` reads a now-optional API field. The platform omits it
  for a few moments after a VM is created, while the billing term is recorded.
  Your configuration is unaffected: the attribute keeps the value you set, and a
  refresh in that window no longer fails.

## [0.3.0] - 2026-07-27

### Added

- **`port_forwarding_rule.tier_id`** — the VPC tier a rule applies to, for a
  public IP reserved in a VPC. Set it only when the target VM has interfaces in
  more than one tier of that VPC; otherwise the platform determines the tier
  from the VM. Ignored for IPs in an isolated network. Create-only: the platform
  does not echo it back, so it forces replacement and is not recoverable by
  `terraform import`.

### Changed

- SDK pin bumped to `americancloud-sdk-go` 1.3.3 (API platform 1.3.3).
- **`vm.network` behavior note.** When `network` is omitted, the platform
  auto-creates an isolated network for the VM. That auto-created network is now
  tied to the VM's lifecycle: destroying the VM (when no other VMs remain on the
  network) deletes the network and releases its public IPs, instead of leaving
  it behind. No schema change — declare an `isolated_network` resource and
  reference it from `vm.network` to keep a network across a VM's lifetime (a
  network you supply is never auto-deleted).

## [0.2.0] - 2026-06-05

### Added

- **Network rule resources** completing the connectivity story for VMs on
  managed networks:
  - `port_forwarding_rule` — forward a public-IP port to a VM port. Pairs with
    `firewall_rule` on the same IP (forwarding routes the traffic, the
    firewall admits it). Immutable; composite import `ipId/ruleId`;
    `open_firewall` is create-only and not recoverable on import.
  - `egress_rule` — outbound rules on **isolated networks** (VPC tiers use
    ACLs). `source_cidr_list` is the *source* of the traffic and must fall
    within the network's CIDR; scope destinations with `dest_cidr_list`.
    Immutable — the platform's egress update replaces the rule under a new id,
    so the provider models every change as a replacement.
  - `network_acl` + `network_acl_rule` — VPC traffic policy. Attach a list to
    a tier via `vpc_tier.acl_id`; rules evaluate in ascending `number` order
    (platform-assigned when omitted). Deleting a list deletes its rules, and
    the rule resource tolerates that cascade on destroy.
  - `load_balancer_rule` — balance a public-IP port across backend VMs.
    `name`, `algorithm`, `description`, and the `instance_ids` backend set
    update in place; ports, protocol, source CIDR, and the IP replace the
    rule. `description` is not echoed by the platform and is not recoverable
    on import.

## [0.1.0] - 2026-06-05

Initial public release. Built on `terraform-plugin-framework` over the
exact-pinned `americancloud-sdk-go` 1.3.1 (API platform 1.3.1). Authentication
via the `AMERICANCLOUD_API_CLIENT_ID` / `AMERICANCLOUD_API_CLIENT_SECRET`
environment variables or provider-block overrides.

### Added

- **Resources (13):** `vm`, `kubernetes_cluster`, `block_storage`, `snapshot`,
  `object_storage_unit`, `isolated_network`, `vpc_network`, `vpc_tier`,
  `public_ip`, `firewall_rule`, `dns_zone`, `dns_record`, `ssh_key`.
- **Data sources (3):** `region`, `image`, `vm_package` (by-label lookups).
- **VM access configuration:** `keypairs` (SSH key names), `user_data`
  (plain-text cloud-init — the provider base64-encodes it for the API),
  `tags`, and `network_access` (create-time inbound port opening + egress
  allow-all on a platform-created network; conflicts with `network`, which is
  optional — omitting it auto-creates an isolated network). All four are
  create-only and not recoverable by `terraform import` (an import under a
  config that sets them plans a replacement).
- **In-place updates wherever the platform supports them** — no
  destroy-and-recreate for: `vm.vcpu` / `vm.memory_mb` (scale),
  `vm.root_disk_gb` (grow-only resize), `kubernetes_cluster.worker_nodes`
  (scale) and `kubernetes_cluster.version` (upgrade-only),
  `block_storage.size_gb` (grow-only), `object_storage_unit.max_size_gb`
  (set, raise, or lift), and network/tier/VPC names and descriptions. The
  platform reboots a VM to apply a scale/resize.
- **Reliable full-stack `terraform destroy`:** deletes block until the
  resource is actually gone and retry the platform's transient 409/504
  responses while a just-deleted dependent (a VM's NIC on its network, a tier
  on its VPC, a snapshot's volume) releases its hold — a VM + network +
  volume + snapshot stack tears down in a single run.
- **Clean `terraform import` round-trips** for every resource, with documented
  exceptions where the API doesn't echo the configured form
  (`vm.vm_package`, `kubernetes_cluster.network_id` / `keypair`,
  `object_storage_unit.max_size_gb`, and the VM access fields above) — the
  first apply after import converges without replacement.
- `object_storage_unit` exposes its S3 credentials as computed, sensitive
  `access_key` / `secret_key` outputs (fetched after create, backfilled on
  refresh/import). Out-of-band quota changes are not drift-detected — the API
  does not echo quotas back; tracked API-side.
- Plan-time validation: `public_ip` requires exactly one of `network_id` /
  `vpc_id`; `vm.root_disk_gb` enforces the 25 GB minimum;
  `block_storage.size_gb` enforces the 5 GiB minimum; ports, protocols, and
  enums are validated against the API's accepted values.
- Configurable `timeouts` (`create` / `delete`, plus `update` on
  `kubernetes_cluster`) on `vm` and `kubernetes_cluster`.
- SDK coverage gate: every method of a covered SDK namespace is mapped to a
  resource/data source or explicitly recorded as not-exposed with a reason,
  so SDK surface growth fails the build instead of drifting silently.
