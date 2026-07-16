package contract_test

import (
	"context"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// loadContract loads the shipped CUE contract module (the repo's top-level
// contract/ directory) through the same load path the engine uses, returning the
// built value plus a cleanup func.
func loadContract(t *testing.T) (cue.Value, func()) {
	t.Helper()
	// The test runs from internal/contract; the contract module is two levels up.
	dir := filepath.Join("..", "..", "contract")
	v, cleanup, err := render.LoadModule(context.Background(), render.LocalLoader{Dir: dir}, "ignored")
	require.NoError(t, err)
	return v, cleanup
}

// TestContract_TransformClosedness proves the contract's whole point: unifying a
// module's `out` against #Transform accepts a conforming transform and rejects an
// unknown/misspelled field or an invalid readiness hint at author time.
func TestContract_TransformClosedness(t *testing.T) {
	module, cleanup := loadContract(t)
	defer cleanup()
	ctx := module.Context()

	transform := module.LookupPath(cue.ParsePath("#Transform"))
	require.NoError(t, transform.Err())

	t.Run("conforming transform satisfies the contract", func(t *testing.T) {
		valid := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resources: {deployment: {object: {apiVersion: "apps/v1", kind: "Deployment"}, ready: "Ready"}}
			status: {ready: true}
		}`)
		require.NoError(t, valid.Err())
		require.NoError(t, transform.Unify(valid).Validate(), "a conforming transform must satisfy #Transform")
	})

	t.Run("a misspelled top-level field is rejected", func(t *testing.T) {
		typo := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resorces: {}
		}`)
		require.NoError(t, typo.Err())
		err := transform.Unify(typo).Validate()
		require.Error(t, err, "#Transform must reject an unknown top-level field")
		assert.Contains(t, err.Error(), "resorces")
	})

	t.Run("an invalid readiness hint is rejected", func(t *testing.T) {
		bad := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resources: {x: {object: {apiVersion: "v1", kind: "ConfigMap"}, ready: "Maybe"}}
		}`)
		require.NoError(t, bad.Err())
		require.Error(t, transform.Unify(bad).Validate(), "#Transform must reject an invalid readiness hint")
	})

	t.Run("optional requirements and requiredResources are accepted", func(t *testing.T) {
		valid := ctx.CompileString(`{
			input: {
				spec: {}, metadata: {name: "demo"}, environment: {}
				requiredResources: {cfg: [{apiVersion: "v1", kind: "ConfigMap"}]}
			}
			requirements: {cfg: {apiVersion: "v1", kind: "ConfigMap", matchName: "app-cfg"}}
			resources: {}
		}`)
		require.NoError(t, valid.Err())
		require.NoError(t, transform.Unify(valid).Validate(),
			"#Transform must accept the optional requirements + requiredResources fields")
	})

	t.Run("observedResources accepts full kind-specific objects", func(t *testing.T) {
		valid := ctx.CompileString(`{
			input: {
				spec: {}, metadata: {name: "demo"}, environment: {}
				observedResources: {workload: {
					apiVersion: "apps/v1"
					kind: "Deployment"
					spec: {replicas: 2}
					status: {observedGeneration: 4, vendorDetail: {revision: "abc"}}
				}}
			}
			resources: {}
		}`)
		require.NoError(t, valid.Err())
		require.NoError(t, transform.Unify(valid).Validate(),
			"#ObservedResource must preserve open kind-specific spec and status")
	})

	t.Run("a misspelled observedResources field is rejected", func(t *testing.T) {
		typo := ctx.CompileString(`{
			input: {
				spec: {}, metadata: {name: "demo"}, environment: {}
				observedResorces: {}
			}
			resources: {}
		}`)
		require.NoError(t, typo.Err())
		err := transform.Unify(typo).Validate()
		require.Error(t, err, "#Input must reject a misspelled observedResources field")
		assert.Contains(t, err.Error(), "observedResorces")
	})

	t.Run("a misspelled requirements field is rejected", func(t *testing.T) {
		typo := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resources: {}
			requirments: {cfg: {apiVersion: "v1", kind: "ConfigMap", matchName: "x"}}
		}`)
		require.NoError(t, typo.Err())
		err := transform.Unify(typo).Validate()
		require.Error(t, err, "#Transform must reject a misspelled requirements field")
		assert.Contains(t, err.Error(), "requirments")
	})

	t.Run("an unknown field inside a requirement is rejected", func(t *testing.T) {
		bad := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resources: {}
			requirements: {cfg: {apiVersion: "v1", kind: "ConfigMap", matchName: "x", nope: true}}
		}`)
		require.NoError(t, bad.Err())
		require.Error(t, transform.Unify(bad).Validate(), "#Requirement must reject an unknown field")
	})

	t.Run("a requirement with neither matchName nor matchLabels still satisfies the contract", func(t *testing.T) {
		// "exactly one of matchName/matchLabels" is enforced by the engine at
		// render time, not by the contract (both are optional). This test pins
		// that contract-level intent so a future tightening is a deliberate change.
		loose := ctx.CompileString(`{
			input: {spec: {}, metadata: {name: "demo"}, environment: {}}
			resources: {}
			requirements: {cfg: {apiVersion: "v1", kind: "ConfigMap"}}
		}`)
		require.NoError(t, loose.Err())
		require.NoError(t, transform.Unify(loose).Validate(),
			"#Requirement leaves match-arm enforcement to the engine, not the contract")
	})
}

// TestContract_APIClosedness proves #API accepts the envelope keys (defaulting
// scope) and rejects an unknown field.
func TestContract_APIClosedness(t *testing.T) {
	module, cleanup := loadContract(t)
	defer cleanup()
	ctx := module.Context()

	api := module.LookupPath(cue.ParsePath("#API"))
	require.NoError(t, api.Err())

	valid := ctx.CompileString(`{group: "platform.example.com", version: "v1alpha1", kind: "XApp", plural: "xapps"}`)
	require.NoError(t, valid.Err())
	require.NoError(
		t,
		api.Unify(valid).Validate(cue.Concrete(true)),
		"#API must accept the envelope keys and default scope",
	)

	typo := ctx.CompileString(`{group: "g", version: "v", kind: "k", plural: "p", scop: "Namespaced"}`)
	require.NoError(t, typo.Err())
	require.Error(t, api.Unify(typo).Validate(), "#API must reject an unknown field")
}
