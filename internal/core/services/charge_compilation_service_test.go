package services

import (
	"testing"

	"github.com/difyz9/ytb2bili/internal/core/types"
)

func TestChargeTierAudienceMapping(t *testing.T) {
	tests := []struct {
		audience string
		tier     int
		ok       bool
	}{
		{audience: "charge_30", tier: 30, ok: true},
		{audience: "charge_50", tier: 50, ok: true},
		{audience: "free", ok: false},
		{audience: "", ok: false},
	}
	for _, test := range tests {
		t.Run(test.audience, func(t *testing.T) {
			tier, ok := ChargeTierFromAudience(test.audience)
			if ok != test.ok || tier != test.tier {
				t.Fatalf("ChargeTierFromAudience(%q) = (%d, %v), want (%d, %v)", test.audience, tier, ok, test.tier, test.ok)
			}
			if !test.ok {
				return
			}
			audience, reverseOK := ChargeAudienceFromTier(tier)
			if !reverseOK || audience != test.audience {
				t.Fatalf("ChargeAudienceFromTier(%d) = (%q, %v), want (%q, true)", tier, audience, reverseOK, test.audience)
			}
		})
	}
}

func TestSelectionBounds(t *testing.T) {
	tests := []struct {
		name    string
		config  *types.AppConfig
		wantMin int
		wantMax int
	}{
		{name: "nil config uses safe defaults", wantMin: 3, wantMax: 4},
		{
			name: "custom bounds",
			config: &types.AppConfig{ChargeCompilation: types.ChargeCompilationConfig{
				MinItems: 4,
				MaxItems: 6,
			}},
			wantMin: 4,
			wantMax: 6,
		},
		{
			name: "invalid maximum cannot undercut minimum",
			config: &types.AppConfig{ChargeCompilation: types.ChargeCompilationConfig{
				MinItems: 5,
				MaxItems: 2,
			}},
			wantMin: 5,
			wantMax: 5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &ChargeCompilationService{Config: test.config}
			gotMin, gotMax := service.selectionBounds()
			if gotMin != test.wantMin || gotMax != test.wantMax {
				t.Fatalf("selectionBounds() = (%d, %d), want (%d, %d)", gotMin, gotMax, test.wantMin, test.wantMax)
			}
		})
	}
}
