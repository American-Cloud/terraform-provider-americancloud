package provider

import (
	"context"
	"time"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*kubernetesClusterResource)(nil)
	_ resource.ResourceWithConfigure   = (*kubernetesClusterResource)(nil)
	_ resource.ResourceWithImportState = (*kubernetesClusterResource)(nil)
)

const (
	k8sCreateTimeout = 30 * time.Minute
	k8sUpdateTimeout = 30 * time.Minute
	k8sDeleteTimeout = 20 * time.Minute
)

// Terminal status sets shared by the create and update polls.
var (
	k8sReadyStatuses  = []string{"running", "active", "ready"}
	k8sFailedStatuses = []string{"error", "failed"}
)

// NewKubernetesClusterResource — sdkRef: kubernetes.CreateClusterKubernetes /
// GetClusterKubernetes / GetClusterConfigKubernetes / ScaleClusterKubernetes /
// UpgradeClusterKubernetes / DeleteClusterKubernetes.
// NOT_EXPOSED: kubernetes.ClusterPowerKubernetes (power is an imperative action, not
// declarable state — belongs in the CLI/MCP, not TF); GetCostEstimateKubernetes
// (preview); ListClustersKubernetes / ListVersionsKubernetes / ListPackagesKubernetes
// (data-source surface).
func NewKubernetesClusterResource() resource.Resource { return &kubernetesClusterResource{} }

type kubernetesClusterResource struct{ baseResource }

type kubernetesClusterModel struct {
	ID           types.String   `tfsdk:"id"`
	Name         types.String   `tfsdk:"name"`
	Region       types.String   `tfsdk:"region"`
	Package      types.String   `tfsdk:"package"`
	Version      types.String   `tfsdk:"version"`
	ControlNodes types.Int64    `tfsdk:"control_nodes"`
	WorkerNodes  types.Int64    `tfsdk:"worker_nodes"`
	Description  types.String   `tfsdk:"description"`
	NetworkID    types.String   `tfsdk:"network_id"`
	Keypair      types.String   `tfsdk:"keypair"`
	Status       types.String   `tfsdk:"status"`
	IPAddress    types.String   `tfsdk:"ip_address"`
	CreatedAt    types.String   `tfsdk:"created_at"`
	Kubeconfig   types.String   `tfsdk:"kubeconfig"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func (r *kubernetesClusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kubernetes_cluster"
}

func (r *kubernetesClusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	strReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	intReplace := []planmodifier.Int64{int64planmodifier.RequiresReplace()}
	useState := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A managed Kubernetes cluster. Provisions asynchronously — `apply` blocks until " +
			"the cluster is running. `worker_nodes` scales and `version` upgrades in place; control_nodes, " +
			"package, region, and name are immutable (changing them replaces the cluster).",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster identifier (UUID).", PlanModifiers: useState},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Cluster name. Forces replacement.", PlanModifiers: strReplace},
			"region":  schema.StringAttribute{Required: true, MarkdownDescription: "Region label, e.g. `us-west-0`. Forces replacement.", PlanModifiers: strReplace},
			"package": schema.StringAttribute{Required: true, MarkdownDescription: "Kubernetes node package label (from `list_kubernetes_packages`). Forces replacement.", PlanModifiers: strReplace},
			"version": schema.StringAttribute{Required: true, MarkdownDescription: "Kubernetes version (from `list_kubernetes_versions`). Upgraded in place; the API rejects downgrades."},
			"control_nodes": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Number of control-plane nodes. Forces replacement.",
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: intReplace,
			},
			"worker_nodes": schema.Int64Attribute{
				Required: true, MarkdownDescription: "Number of worker nodes. Scaled in place.",
				Validators: []validator.Int64{int64validator.AtLeast(0)},
			},
			"description": schema.StringAttribute{Optional: true, MarkdownDescription: "Optional description. Forces replacement.", PlanModifiers: strReplace},
			"network_id":  schema.StringAttribute{Optional: true, MarkdownDescription: "Existing network UUID to place the cluster in. Omit to auto-create. Forces replacement.", PlanModifiers: strReplace},
			"keypair":     schema.StringAttribute{Optional: true, MarkdownDescription: "SSH key pair name to install on nodes. Forces replacement.", PlanModifiers: strReplace},
			"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Current lifecycle status.", PlanModifiers: useState},
			"ip_address":  schema.StringAttribute{Computed: true, MarkdownDescription: "Cluster API endpoint IP.", PlanModifiers: useState},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time (RFC 3339).", PlanModifiers: useState},
			"kubeconfig": schema.StringAttribute{
				Computed: true, Sensitive: true,
				MarkdownDescription: "kubeconfig (YAML) for connecting kubectl. Sensitive — grants cluster access.",
				PlanModifiers:       useState,
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func (r *kubernetesClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kubernetesClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := r.client.Kubernetes.CreateClusterKubernetes(ctx, &acsdk.CreateKubernetesClusterRequest{
		Name:         plan.Name.ValueString(),
		Package:      plan.Package.ValueString(),
		Region:       plan.Region.ValueString(),
		Version:      plan.Version.ValueString(),
		ControlNodes: float64(plan.ControlNodes.ValueInt64()),
		WorkerNodes:  float64(plan.WorkerNodes.ValueInt64()),
		Description:  stringToPtr(plan.Description),
		NetworkID:    stringToPtr(plan.NetworkID),
		Keypair:      stringToPtr(plan.Keypair),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating Kubernetes cluster", err.Error())
		return
	}

	// Persist the id immediately so a poll timeout below can't orphan the cluster.
	plan.ID = types.StringValue(cluster.ID)
	plan.Status = types.StringValue(cluster.Status)
	plan.IPAddress = stringPtrToString(cluster.IPAddress)
	plan.CreatedAt = types.StringValue(cluster.CreatedAt.Format(time.RFC3339))
	plan.Kubeconfig = types.StringNull()

	createTimeout, diags := plan.Timeouts.Create(ctx, k8sCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...) // persist the id even if the timeout config is invalid
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	ready, perr := r.pollClusterReady(pollCtx, cluster.ID)
	if perr != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...) // persist the id first
		resp.Diagnostics.AddError("Kubernetes cluster did not reach a running state", perr.Error())
		return
	}

	plan.Status = types.StringValue(ready.Status)
	plan.IPAddress = stringPtrToString(ready.IPAddress)
	if cfg, cerr := r.client.Kubernetes.GetClusterConfigKubernetes(ctx, &acsdk.GetClusterConfigKubernetesRequest{ID: cluster.ID}); cerr == nil {
		plan.Kubeconfig = types.StringValue(cfg.Configdata)
	} else {
		// The cluster is running but its config isn't retrievable yet. Don't fail
		// the apply — surface a warning and let the next Read backfill it.
		resp.Diagnostics.AddWarning(
			"Kubeconfig not yet available",
			"The cluster reached a running state but its kubeconfig could not be fetched: "+cerr.Error()+
				". Run `terraform refresh` (or the next plan/apply) once the cluster finishes initializing to populate it.",
		)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kubernetesClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kubernetesClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := r.client.Kubernetes.GetClusterKubernetes(ctx, &acsdk.GetClusterKubernetesRequest{ID: state.ID.ValueString()})
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Kubernetes cluster", err.Error())
		return
	}

	// Config fields are immutable (ForceNew). keepStr/keepInt preserve the user's
	// values on a normal refresh and hydrate the required ones from the API when
	// state is empty (the `terraform import` case), so an id-only import populates
	// them instead of proposing a replacement. The optional fields are preserved
	// from state, never hydrated: description is a plain (always-present) response
	// string and is RequiresReplace, so hydrating it on a null would risk a
	// spurious cluster replacement if the API ever returns one the user didn't
	// set; network_id and keypair aren't echoed by the response at all. The
	// trade-off is that a cluster created with description/network_id/keypair
	// can't fully round-trip on import (tracked API-side).
	state.Name = keepStr(state.Name, cluster.Name)
	state.Region = keepStr(state.Region, cluster.Region)
	state.Package = keepStr(state.Package, cluster.Package)
	// version + worker_nodes are mutable (in-place upgrade/scale) — hydrate the live
	// value so out-of-band changes show as drift and import populates them.
	// control_nodes stays immutable (keepStr/keepInt semantics).
	state.Version = types.StringValue(cluster.KubernetesVersion)
	if cluster.NodePool != nil {
		state.ControlNodes = keepInt(state.ControlNodes, int64(cluster.NodePool.ControlNodes))
		state.WorkerNodes = types.Int64Value(int64(cluster.NodePool.WorkerNodes))
	}
	// Computed fields always refresh from the API.
	state.Status = types.StringValue(cluster.Status)
	state.IPAddress = stringPtrToString(cluster.IPAddress)
	state.CreatedAt = types.StringValue(cluster.CreatedAt.Format(time.RFC3339))
	// Backfill the kubeconfig if it isn't in state yet (create-time fetch failed,
	// or the cluster was imported). Best-effort — never fail Read over it.
	if state.Kubeconfig.IsNull() || state.Kubeconfig.ValueString() == "" {
		if cfg, cerr := r.client.Kubernetes.GetClusterConfigKubernetes(ctx, &acsdk.GetClusterConfigKubernetesRequest{ID: state.ID.ValueString()}); cerr == nil {
			state.Kubeconfig = types.StringValue(cfg.Configdata)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place changes: worker_nodes via ScaleClusterKubernetes and
// version via UpgradeClusterKubernetes (upgrade-only — the API rejects downgrades).
// Both are async and put the cluster into a transitional state (Scaling) during
// which CloudStack REJECTS further operations — so, unlike the cheap in-place
// ops on other resources, this polls the cluster back to a ready state at the
// target worker-count/version before returning. Without that, Terraform reports
// the change complete while the cluster is still working, and a subsequent op in
// the same run (notably destroy) races the in-flight scale and is rejected.
func (r *kubernetesClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state kubernetesClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scaled := !plan.WorkerNodes.Equal(state.WorkerNodes)
	upgraded := !plan.Version.Equal(state.Version)

	if scaled {
		workers := float64(plan.WorkerNodes.ValueInt64())
		if err := r.client.Kubernetes.ScaleClusterKubernetes(ctx, &acsdk.ScaleKubernetesClusterRequest{
			ID: state.ID.ValueString(), WorkerNodes: &workers,
		}); err != nil {
			resp.Diagnostics.AddError("Error scaling Kubernetes cluster", err.Error())
			return
		}
	}

	if upgraded {
		if err := r.client.Kubernetes.UpgradeClusterKubernetes(ctx, &acsdk.UpgradeKubernetesClusterRequest{
			ID: state.ID.ValueString(), Version: plan.Version.ValueString(),
		}); err != nil {
			resp.Diagnostics.AddError("Error upgrading Kubernetes cluster", err.Error())
			return
		}
	}

	// Wait until the cluster is back to a ready status at the target
	// worker-count/version (the method comment above has the why).
	if scaled || upgraded {
		updateTimeout, diags := plan.Timeouts.Update(ctx, k8sUpdateTimeout)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		pollCtx, cancel := context.WithTimeout(ctx, updateTimeout)
		defer cancel()
		wantWorkers := plan.WorkerNodes.ValueInt64()
		wantVersion := plan.Version.ValueString()
		if err := pollReady(pollCtx, func(ctx context.Context) (string, error) {
			c, e := r.client.Kubernetes.GetClusterKubernetes(ctx, &acsdk.GetClusterKubernetesRequest{ID: state.ID.ValueString()})
			if e != nil {
				return "", e
			}
			// Right after issuing, the cluster briefly still reports ready
			// before transitioning to Scaling — hold off "ready" until the
			// live worker-count/version match the target (same idiom as
			// pollVMReady's root_volume_id hold-off).
			atTarget := c.NodePool != nil && int64(c.NodePool.WorkerNodes) == wantWorkers &&
				c.KubernetesVersion == wantVersion
			if containsFold(k8sReadyStatuses, c.Status) && !atTarget {
				return "scaling", nil
			}
			return c.Status, nil
		}, k8sReadyStatuses, k8sFailedStatuses); err != nil {
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Kubernetes cluster did not finish updating", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *kubernetesClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kubernetesClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteTimeout, diags := state.Timeouts.Delete(ctx, k8sDeleteTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Kubernetes.DeleteClusterKubernetes(ctx, &acsdk.DeleteClusterKubernetesRequest{ID: state.ID.ValueString()}); err != nil {
		resp.Diagnostics.AddError("Error deleting Kubernetes cluster", err.Error())
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if err := r.pollClusterGone(pollCtx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Kubernetes cluster did not finish terminating", err.Error())
	}
}

func (r *kubernetesClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *kubernetesClusterResource) pollClusterReady(ctx context.Context, id string) (*acsdk.KubernetesClusterResponse, error) {
	var last *acsdk.KubernetesClusterResponse
	err := pollReady(ctx, func(ctx context.Context) (string, error) {
		c, e := r.client.Kubernetes.GetClusterKubernetes(ctx, &acsdk.GetClusterKubernetesRequest{ID: id})
		if e != nil {
			return "", e
		}
		last = c
		return c.Status, nil
	}, k8sReadyStatuses, k8sFailedStatuses)
	return last, err
}

func (r *kubernetesClusterResource) pollClusterGone(ctx context.Context, id string) error {
	return pollGone(ctx, func(ctx context.Context) (string, bool, error) {
		c, e := r.client.Kubernetes.GetClusterKubernetes(ctx, &acsdk.GetClusterKubernetesRequest{ID: id})
		if e != nil {
			if isNotFound(e) {
				return "", true, nil
			}
			return "", false, e
		}
		return c.Status, false, nil
		// "deleting" is still in progress — only terminal states count as gone, so
		// the poll doesn't return before the cluster releases its resources. The
		// 404 branch above is the usual completion signal once the record is purged.
	}, []string{"deleted", "destroyed"})
}
