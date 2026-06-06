# Changelog

All notable changes to the American Cloud Terraform provider are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
