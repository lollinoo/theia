package scheduler

import (
	"testing"
	"time"

	"github.com/lollinoo/theia/internal/domain"
)

func TestEffectiveInterval_PerformanceUsesOverride(t *testing.T) {
	override := 45
	coreDevice := domain.Device{
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(override),
	}
	lowDevice := domain.Device{
		PollClass:            domain.PollClassLow,
		PollIntervalOverride: intPtr(0),
	}

	if got := EffectiveInterval(coreDevice, domain.VolatilityClassPerformance); got != 45*time.Second {
		t.Fatalf("EffectiveInterval(core override) = %v, want %v", got, 45*time.Second)
	}

	if got := EffectiveInterval(lowDevice, domain.VolatilityClassPerformance); got != domain.PollClassLowInterval {
		t.Fatalf("EffectiveInterval(low fallback) = %v, want %v", got, domain.PollClassLowInterval)
	}
}

func TestEffectiveInterval_OperationalIgnoresOverride(t *testing.T) {
	device := domain.Device{
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(45),
	}

	if got := EffectiveInterval(device, domain.VolatilityClassOperational); got != domain.OperationalClassInterval {
		t.Fatalf("EffectiveInterval(operational) = %v, want %v", got, domain.OperationalClassInterval)
	}
}

func TestEffectiveInterval_StaticIgnoresOverride(t *testing.T) {
	device := domain.Device{
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(45),
	}

	if got := EffectiveInterval(device, domain.VolatilityClassStatic); got != domain.StaticClassInterval {
		t.Fatalf("EffectiveInterval(static) = %v, want %v", got, domain.StaticClassInterval)
	}
}

func TestVolatilityPriority(t *testing.T) {
	tests := []struct {
		name       string
		volatility domain.VolatilityClass
		want       int
	}{
		{name: "performance", volatility: domain.VolatilityClassPerformance, want: 0},
		{name: "operational", volatility: domain.VolatilityClassOperational, want: 1},
		{name: "static", volatility: domain.VolatilityClassStatic, want: 2},
		{name: "unknown", volatility: domain.VolatilityClass("unknown"), want: 99},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VolatilityPriority(tc.volatility); got != tc.want {
				t.Fatalf("VolatilityPriority(%q) = %d, want %d", tc.volatility, got, tc.want)
			}
		})
	}
}

func intPtr(value int) *int {
	return &value
}
