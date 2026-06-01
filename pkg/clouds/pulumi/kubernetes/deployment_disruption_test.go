package kubernetes

import (
	"testing"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/clouds/k8s"
)

// TestDefaultDisruptionBudget guards the PodDisruptionBudget default. The default
// must be maxUnavailable:1 (not minAvailable:1): minAvailable:1 on a single-replica
// deployment yields disruptionsAllowed:0, which blocks the cluster autoscaler from
// draining the pod's node and pins underutilized nodes on GKE Autopilot.
func TestDefaultDisruptionBudget(t *testing.T) {
	t.Run("nil defaults to maxUnavailable:1 so single-replica nodes stay drainable", func(t *testing.T) {
		got := defaultDisruptionBudget(nil)
		if got == nil {
			t.Fatal("expected a default budget, got nil")
		}
		if got.MinAvailable != nil {
			t.Errorf("default must not set MinAvailable (it pins single-replica nodes); got %d", *got.MinAvailable)
		}
		if got.MaxUnavailable == nil {
			t.Fatal("default must set MaxUnavailable")
		}
		if *got.MaxUnavailable != 1 {
			t.Errorf("default MaxUnavailable = %d, want 1", *got.MaxUnavailable)
		}
	})

	t.Run("explicit minAvailable is honored unchanged", func(t *testing.T) {
		in := &k8s.DisruptionBudget{MinAvailable: lo.ToPtr(2)}
		got := defaultDisruptionBudget(in)
		if got != in {
			t.Fatal("explicit budget must be returned as-is")
		}
		if got.MinAvailable == nil || *got.MinAvailable != 2 {
			t.Errorf("MinAvailable = %v, want 2", got.MinAvailable)
		}
		if got.MaxUnavailable != nil {
			t.Errorf("explicit minAvailable budget must not gain a MaxUnavailable; got %d", *got.MaxUnavailable)
		}
	})

	t.Run("explicit maxUnavailable is honored unchanged", func(t *testing.T) {
		in := &k8s.DisruptionBudget{MaxUnavailable: lo.ToPtr(3)}
		got := defaultDisruptionBudget(in)
		if got != in {
			t.Fatal("explicit budget must be returned as-is")
		}
		if got.MaxUnavailable == nil || *got.MaxUnavailable != 3 {
			t.Errorf("MaxUnavailable = %v, want 3", got.MaxUnavailable)
		}
	})
}
