package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// An existing cluster must plan no change when the config leaves a package
// attribute unset, also when the state value is null (the API does not know
// the package). A plan of unknown there makes RequiresReplace replace it.
func TestKeepStateWhenUnset(t *testing.T) {
	present := tftypes.NewValue(tftypes.String, "present")
	absent := tftypes.NewValue(tftypes.String, nil)
	cases := []struct {
		name              string
		stateRaw, planRaw tftypes.Value
		config, state     types.String
		planIn, wantPlan  types.String
	}{
		{"update, unset, state null", present, present, types.StringNull(), types.StringNull(), types.StringUnknown(), types.StringNull()},
		{"update, unset, state set", present, present, types.StringNull(), types.StringValue("basic"), types.StringUnknown(), types.StringValue("basic")},
		{"update, set in config", present, present, types.StringValue("scale"), types.StringValue("basic"), types.StringValue("scale"), types.StringValue("scale")},
		{"create", absent, present, types.StringNull(), types.StringNull(), types.StringUnknown(), types.StringUnknown()},
		{"destroy", present, absent, types.StringNull(), types.StringValue("basic"), types.StringNull(), types.StringNull()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planmodifier.StringRequest{
				State:       tfsdk.State{Raw: c.stateRaw},
				Plan:        tfsdk.Plan{Raw: c.planRaw},
				ConfigValue: c.config,
				StateValue:  c.state,
				PlanValue:   c.planIn,
			}
			resp := &planmodifier.StringResponse{PlanValue: c.planIn}
			keepStateWhenUnset().PlanModifyString(context.Background(), req, resp)
			if !resp.PlanValue.Equal(c.wantPlan) {
				t.Errorf("plan = %s, want %s", resp.PlanValue, c.wantPlan)
			}
		})
	}
}

func TestFillUnsetPackages(t *testing.T) {
	u, v := types.StringUnknown(), types.StringValue
	cases := []struct {
		name                          string
		pkg, control, worker          types.String
		wantPkg, wantCtrl, wantWorker string
	}{
		{"package only", v("basic"), u, u, "basic", "basic", "basic"},
		{"package and control", v("basic"), v("scale"), u, "basic", "scale", "basic"},
		{"package and worker", v("basic"), u, v("scale"), "basic", "basic", "scale"},
		{"both overrides", u, v("scale"), v("basic"), "basic", "scale", "basic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := kubernetesClusterModel{Package: c.pkg, ControlPackage: c.control, WorkerPackage: c.worker}
			fillUnsetPackages(&m)
			got := [3]string{m.Package.ValueString(), m.ControlPackage.ValueString(), m.WorkerPackage.ValueString()}
			want := [3]string{c.wantPkg, c.wantCtrl, c.wantWorker}
			if got != want {
				t.Errorf("package/control/worker = %v, want %v", got, want)
			}
		})
	}
}

func TestKubernetesClusterValidateConfig(t *testing.T) {
	ctx := context.Background()
	r := &kubernetesClusterResource{}
	var sresp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sresp)
	objType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	// config builds a config with every attribute null except the given packages.
	config := func(pkgs map[string]string) tfsdk.Config {
		vals := map[string]tftypes.Value{}
		for name, typ := range objType.AttributeTypes {
			vals[name] = tftypes.NewValue(typ, nil)
		}
		for name, val := range pkgs {
			if val == "<unknown>" {
				vals[name] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
			} else {
				vals[name] = tftypes.NewValue(tftypes.String, val)
			}
		}
		return tfsdk.Config{Schema: sresp.Schema, Raw: tftypes.NewValue(objType, vals)}
	}

	cases := []struct {
		name    string
		pkgs    map[string]string
		wantErr bool
	}{
		{"package only", map[string]string{"package": "basic"}, false},
		{"package and control", map[string]string{"package": "basic", "control_package": "scale"}, false},
		{"package and worker", map[string]string{"package": "basic", "worker_package": "scale"}, false},
		{"both overrides", map[string]string{"control_package": "scale", "worker_package": "basic"}, false},
		{"all three", map[string]string{"package": "basic", "control_package": "scale", "worker_package": "basic"}, true},
		{"none", map[string]string{}, true},
		{"control only", map[string]string{"control_package": "scale"}, true},
		{"worker only", map[string]string{"worker_package": "basic"}, true},
		{"unknown value is not checked", map[string]string{"control_package": "<unknown>"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config(c.pkgs)}, resp)
			if got := resp.Diagnostics.HasError(); got != c.wantErr {
				t.Errorf("error = %v, want %v: %v", got, c.wantErr, resp.Diagnostics)
			}
		})
	}
}
