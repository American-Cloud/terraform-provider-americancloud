# Changelog

All notable changes to the American Cloud Terraform provider are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-05

### Added

- **VM access configuration**: `keypairs` (SSH key names), `user_data`
  (plain-text cloud-init — the provider base64-encodes it for the API),
  `tags`, and `network_access` (create-time inbound port opening + egress
  allow-all for a platform-created network; conflicts with `network`, which
  is now optional — omitting it auto-creates an isolated network). All four
  are create-only (ForceNew) and are not recoverable by `terraform import`
  (the get response doesn't echo them; an import under a config that sets
  them plans a replacement).
- Initial provider built on `terraform-plugin-framework` over the exact-pinned
  `americancloud-sdk-go`, with env-based authentication
  (`AMERICANCLOUD_API_CLIENT_ID` / `_SECRET`) and provider-block overrides.
- Resources: `dns_zone`, `dns_record`, `block_storage`, `snapshot`,
  `isolated_network`, `vpc_network`, `public_ip`, `firewall_rule`, `ssh_key`,
  `vm`, `kubernetes_cluster`.
- SDK coverage test — every covered SDK method is mapped to a resource or
  explicitly recorded as not-exposed, so SDK growth surfaces as a failing test.
- New resources `object_storage_unit` and `vpc_tier`, unblocked by API 1.3.0:
  object-storage create now returns a canonical id, and VPC tiers gained a
  delete-tier endpoint (so a tier can be destroyed independently of its VPC).
  The storage unit exposes its S3 credentials as computed, sensitive
  `access_key` / `secret_key` outputs (fetched after create, backfilled on
  refresh/import) and a declarable `max_size_gb` quota — set on create,
  updated in place, lifted when removed from config. The API does not
  currently echo the quota back on reads, so out-of-band quota changes are
  not detected and import cannot recover `max_size_gb` (the first apply
  after import converges by re-setting it) — tracked API-side.
- Configurable `timeouts` (`create`, `delete` — plus `update` on
  `kubernetes_cluster`) on `vm` and `kubernetes_cluster`, replacing the
  previously hard-coded poll bounds.
- `public_ip` now validates at plan time that exactly one of `network_id` /
  `vpc_id` is set, and `vm.root_disk_gb` enforces its documented 25 GB minimum.
- `block_storage.size_gb` now validates the API's 5 GiB minimum (was 1), so an
  undersized volume is rejected at plan time instead of with a 400 on apply.
- **In-place resize/scale (no more destroy-and-recreate):** `vm.vcpu` /
  `vm.memory_mb` update via `ScaleVms`, `vm.root_disk_gb` grows via `ResizeDiskVms`
  (grow-only; shrink rejected), `kubernetes_cluster.worker_nodes` scales via
  `ScaleClusterKubernetes`, and `kubernetes_cluster.version` upgrades via
  `UpgradeClusterKubernetes` (upgrade-only; downgrades rejected by the API). These
  attributes previously forced a destructive replacement. The platform reboots the
  VM to apply a resize/scale; changes apply asynchronously (the next refresh
  reconciles). `control_nodes`, image, network, region, and name remain immutable.

### Changed

- Bumped to `americancloud-sdk-go` 1.3.1 (API platform 1.3.1): isolated-network
  and snapshot deletes now surface their transient `409`/`504` responses as
  typed errors. No provider-facing change — the delete retry already matches
  on status code.
- **`firewall_rule.start_port` / `end_port` are now numbers, not strings**, matching
  the API's standardized firewall port types — quote-free in config now.
- `public_ip` now recovers `vpc_id` on import (the 1.3.0 read response carries
  `vpcId`), and `snapshot` round-trips `type` on import (now always returned) — both
  drop from the "unrecoverable on import" list above.
- VM in-place scale/resize now sends its parameters in the request **body** (1.3.0
  moved them off the query string), so `vm.vcpu` / `memory_mb` / `root_disk_gb`
  changes apply against the live edge instead of being rejected.

### Fixed

- **`terraform destroy` of a full stack no longer flakes on CloudStack's
  release ordering.** Network-side deletes (`isolated_network`, `vpc_tier`,
  `vpc_network`) now retry while a just-deleted dependent releases its hold —
  the API returns 409/504 for a beat after a deleted VM's NIC or a deleted
  tier actually clears. `snapshot` delete now blocks until the snapshot is
  gone: returning early let the snapshotted VM's expunge in the same destroy
  race the surviving snapshot and wedge the volume removal. And
  `kubernetes_cluster` scale/upgrade now waits for the cluster to leave its
  transitional `Scaling` state (during which the API rejects further
  operations, including a destroy later in the same run).
- `snapshot` delete now retries the API's documented transient failures (409
  while its volume is mid-modification/removal, 504 while deletion settles)
  instead of failing the destroy on the first conflict.
- When a retried network/tier/VPC delete exhausts its timeout, the error now
  carries the API's last response (e.g. the 409 naming the attachment) instead
  of a bare "context deadline exceeded" — a permanent attachment such as an
  out-of-band associated public IP is otherwise undiagnosable.
- `vm` create no longer reports ready before `root_volume_id` is populated, so
  a dependent resource created in the same apply (snapshot, volume attach)
  resolves it to the real id instead of an empty string.
- `public_ip.network_id` now hydrates from the IP's *associated* network on
  refresh/import — the API's `networkId` is CloudStack's internal source-NAT
  network and never matches the configured value, so importing an
  isolated-network IP proposed a spurious replacement.
- `terraform import` is now round-trip clean for `vm`, `kubernetes_cluster`,
  `firewall_rule`, and `public_ip`. Their `Read` previously refreshed only
  computed fields and kept config attributes from prior state; on import (which
  seeds only the id) those attributes were left empty, so the next plan proposed
  a destructive replacement. `Read` now hydrates config attributes from the API
  when state is empty while still preferring existing state on a normal refresh
  (no phantom drift). A few attributes can't be recovered on import because the
  read endpoint doesn't echo them in the configured form — `vm.vm_package` (read
  returns the package display name, not the label), `public_ip.vpc_id`, and
  `kubernetes_cluster.network_id` / `keypair` — tracked API-side.
- VM and Kubernetes delete no longer treat the in-progress `expunging` /
  `deleting` states as terminal, so the delete poll blocks until the resource is
  actually gone and a dependent resource in the same apply isn't deleted early.
- `kubernetes_cluster` no longer silently drops the kubeconfig when the
  create-time fetch fails: it emits a warning and the next `Read` backfills it.
- `firewall_rule` import is normalized so it round-trips cleanly: the API returns
  `protocol` lowercased (uppercased back to match the schema enum) and omits
  `type` for the default Ingress direction (defaulted on import), either of which
  previously forced a spurious replacement after import.
