package provider

// Coverage manifest. Every public method of a covered SDK namespace must be
// either in `mapped` (used by a resource or data source) or in `notExposed`
// (with a reason). TestSDKCoverage enforces this by reflection, turning SDK
// growth/renames into a failing test instead of a silent gap. Keys are
// "<ClientField>.<Method>" matching the SDK client's struct field names.

// coveredNamespaces are the SDK client namespaces v0.1 manages.
var coveredNamespaces = []string{
	"DNSZones", "DNSRecords", "BlockStorage", "Snapshots",
	"IsolatedNetworks", "VpcNetworks", "PublicIps", "FirewallRules",
	"SSHKeys", "Vms", "Kubernetes", "ObjectStorage",
	"Regions", "Images", "VMPackages",
}

// mapped: SDK methods a resource uses.
var mapped = map[string]bool{
	"DNSZones.CreateDNSZones":                 true,
	"DNSZones.ListDNSZones":                   true,
	"DNSZones.DeleteDNSZones":                 true,
	"DNSRecords.CreateDNSRecords":             true,
	"DNSRecords.ListDNSRecords":               true,
	"DNSRecords.DeleteDNSRecords":             true,
	"BlockStorage.CreateBlockStorage":         true,
	"BlockStorage.GetBlockStorage":            true,
	"BlockStorage.ResizeBlockStorage":         true,
	"BlockStorage.DeleteBlockStorage":         true,
	"Snapshots.CreateSnapshots":               true,
	"Snapshots.GetSnapshots":                  true,
	"Snapshots.DeleteSnapshots":               true,
	"IsolatedNetworks.CreateIsolatedNetworks": true,
	"IsolatedNetworks.GetIsolatedNetworks":    true,
	"IsolatedNetworks.UpdateIsolatedNetworks": true,
	"IsolatedNetworks.DeleteIsolatedNetworks": true,
	"VpcNetworks.CreateVpcNetworks":           true,
	"VpcNetworks.GetVpcNetworks":              true,
	"VpcNetworks.UpdateVpcNetworks":           true,
	"VpcNetworks.DeleteVpcNetworks":           true,
	"VpcNetworks.CreateTierVpcNetworks":       true,
	"VpcNetworks.GetTierVpcNetworks":          true,
	"VpcNetworks.UpdateTierVpcNetworks":       true,
	"VpcNetworks.DeleteTierVpcNetworks":       true,
	"PublicIps.ReservePublicIps":              true,
	"PublicIps.GetPublicIps":                  true,
	"PublicIps.ReleasePublicIps":              true,
	"FirewallRules.CreateFirewallRules":       true,
	"FirewallRules.ListFirewallRules":         true,
	"FirewallRules.DeleteFirewallRules":       true,
	"SSHKeys.CreateSSHKeys":                   true,
	"SSHKeys.ListSSHKeys":                     true,
	"SSHKeys.DeleteSSHKeys":                   true,
	"Vms.CreateVms":                           true,
	"Vms.GetVms":                              true,
	"Vms.ScaleVms":                            true,
	"Vms.ResizeDiskVms":                       true,
	"Vms.DeleteVms":                           true,
	"Kubernetes.CreateClusterKubernetes":      true,
	"Kubernetes.GetClusterKubernetes":         true,
	"Kubernetes.GetClusterConfigKubernetes":   true,
	"Kubernetes.ScaleClusterKubernetes":       true,
	"Kubernetes.UpgradeClusterKubernetes":     true,
	"Kubernetes.DeleteClusterKubernetes":      true,
	"ObjectStorage.CreateUnitObjectStorage":   true,
	"ObjectStorage.ListUnitsObjectStorage":    true,
	"ObjectStorage.GetKeysObjectStorage":      true,
	"ObjectStorage.SetUserQuotaObjectStorage": true,
	"ObjectStorage.DeleteUnitObjectStorage":   true,
	"Regions.GetByLabelRegions":               true,
	"Images.GetByLabelImages":                 true,
	"VMPackages.GetByLabelVMPackages":         true,
}

// notExposed: SDK methods deliberately not surfaced, with the reason.
var notExposed = map[string]string{
	"DNSRecords.UpdateDNSRecords": "update keys on (name,type) only — ambiguous for multi-value sets; resource is replace-on-change",

	"BlockStorage.AttachBlockStorage":          "volume attachment lands with the vm resource (validate end-to-end) — deferred",
	"BlockStorage.DetachBlockStorage":          "see AttachBlockStorage",
	"BlockStorage.ListBlockStorage":            "data-source surface",
	"BlockStorage.ListSnapshotsBlockStorage":   "data-source surface",
	"BlockStorage.GetCostEstimateBlockStorage": "cost preview — not declarable state",

	"Snapshots.RevertSnapshots":          "imperative restore — not declarable state",
	"Snapshots.ListSnapshots":            "data-source surface",
	"Snapshots.GetCostEstimateSnapshots": "cost preview — not declarable state",

	"IsolatedNetworks.RestartIsolatedNetworks": "imperative op",
	"IsolatedNetworks.ListIsolatedNetworks":    "data-source surface",

	"VpcNetworks.RestartVpcNetworks":         "imperative op",
	"VpcNetworks.RestartTierVpcNetworks":     "imperative op",
	"VpcNetworks.GetCostEstimateVpcNetworks": "cost preview",
	"VpcNetworks.ListVpcNetworks":            "data-source surface",

	"PublicIps.ChangeSourceNatIPPublicIps":     "NAT op — deferred to v0.2",
	"PublicIps.EnableStaticNatPublicIps":       "NAT op — deferred to v0.2",
	"PublicIps.DisableStaticNatPublicIps":      "NAT op — deferred to v0.2",
	"PublicIps.ListPublicIps":                  "data-source surface",
	"PublicIps.ListByIsolatedNetworkPublicIps": "data-source surface",
	"PublicIps.ListByVpcPublicIps":             "data-source surface",
	"PublicIps.GetCostEstimatePublicIps":       "cost preview",

	"Vms.UpdateHostnameVms":  "hostname not in get/create response — can't reconcile",
	"Vms.PowerVms":           "power is an imperative action, not declarable state",
	"Vms.ReinstallVms":       "imperative op",
	"Vms.ResetPasswordVms":   "imperative op",
	"Vms.CreateConsoleVms":   "imperative op (short-lived session)",
	"Vms.GetMetricsVms":      "read/metrics surface",
	"Vms.GetCostEstimateVms": "cost preview",
	"Vms.ListVms":            "data-source surface",

	"Kubernetes.ClusterPowerKubernetes":    "power is an imperative action, not declarable state",
	"Kubernetes.ListClustersKubernetes":    "data-source surface",
	"Kubernetes.ListVersionsKubernetes":    "data-source surface",
	"Kubernetes.ListPackagesKubernetes":    "data-source surface",
	"Kubernetes.GetCostEstimateKubernetes": "cost preview",

	"ObjectStorage.CreateBucketObjectStorage":    "buckets land as a follow-up sub-resource",
	"ObjectStorage.DeleteBucketObjectStorage":    "see CreateBucketObjectStorage",
	"ObjectStorage.ListBucketsObjectStorage":     "data-source / bucket sub-resource surface",
	"ObjectStorage.GetCostEstimateObjectStorage": "cost preview",

	"Regions.ListRegions":       "data-source list surface (by-label lookup used)",
	"Regions.GetRegions":        "get-by-id; by-label lookup used instead",
	"Images.ListImages":         "data-source list surface (by-label lookup used)",
	"Images.GetImages":          "get-by-id; by-label lookup used instead",
	"VMPackages.ListVMPackages": "data-source list surface (by-label lookup used)",
	"VMPackages.GetVMPackages":  "get-by-id; by-label lookup used instead",
}
