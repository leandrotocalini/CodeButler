package tools

import "testing"

func TestRiskTier_String(t *testing.T) {
	tests := []struct {
		tier RiskTier
		want string
	}{
		{Read, "READ"},
		{WriteLocal, "WRITE_LOCAL"},
		{WriteVisible, "WRITE_VISIBLE"},
		{Destructive, "DESTRUCTIVE"},
		{RiskTier(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tier.String(); got != tt.want {
				t.Errorf("RiskTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}
