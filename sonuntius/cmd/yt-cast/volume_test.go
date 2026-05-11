// Maps to: N/A — Go-only tests for the delta-based volume routing.
package main

import "testing"

func TestComputeVolumeOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       int
		step      int
		reference int
		want      int
	}{
		{"first call: round to bucket (step=10)", 47, 10, -1, 50},
		{"first call: round down (step=10)", 23, 10, -1, 20},
		{"first call: round to bucket (step=5)", 23, 5, -1, 25},
		{"no change: keep reference", 50, 10, 50, 50},

		// The feedback-loop scenario this whole change is for:
		// phone is at 50, user presses down, phone sends 47, we bump
		// down one step. Phone then sees 40 echoed back, presses
		// down again, sends 37, we bump down to 30. Etc.
		{"press down inside bucket (47 vs 50)", 47, 10, 50, 40},
		{"press down inside bucket (37 vs 40)", 37, 10, 40, 30},
		{"press down inside bucket (27 vs 30)", 27, 10, 30, 20},
		{"press up inside bucket (53 vs 50)", 53, 10, 50, 60},
		{"press up inside bucket (63 vs 60)", 63, 10, 60, 70},

		// Slider drags — big delta, snap to rounded raw.
		{"drag up across buckets", 75, 10, 30, 80},
		{"drag down across buckets", 12, 10, 80, 10},
		{"drag to zero", 0, 10, 60, 0},
		{"drag to max", 100, 10, 30, 100},

		// Edge: at boundaries.
		{"step down from 10 clamps at 0", 7, 10, 10, 0},
		{"step up from 90 clamps at 100", 93, 10, 90, 100},

		// step=5 (default) — finer increments.
		{"step 5: bump down", 22, 5, 25, 20},
		{"step 5: bump up", 28, 5, 25, 30},
		{"step 5: drag", 80, 5, 25, 80},

		// step=1 effectively means pass-through (but still snaps).
		{"step 1: pass through", 73, 1, 50, 73},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeVolumeOutput(tc.raw, tc.step, tc.reference)
			if got != tc.want {
				t.Errorf("computeVolumeOutput(raw=%d, step=%d, ref=%d) = %d, want %d",
					tc.raw, tc.step, tc.reference, got, tc.want)
			}
		})
	}
}
