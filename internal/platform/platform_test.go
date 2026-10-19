package platform

import "testing"

func TestParsePmsetBatt(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		wantBat   bool
		wantKnown bool
	}{
		{
			name: "macOS AC power charging",
			fixture: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=1234567)	85%; charging; 1:23 remaining present: true
`,
			wantBat:   false,
			wantKnown: true,
		},
		{
			name: "macOS AC power charged",
			fixture: `Now drawing from 'AC Power'
 -InternalBattery-0 (id=1234567)	100%; charged; 0:00 remaining present: true
`,
			wantBat:   false,
			wantKnown: true,
		},
		{
			name: "macOS Battery power discharging",
			fixture: `Now drawing from 'Battery Power'
 -InternalBattery-0 (id=1234567)	98%; discharging; 5:12 remaining present: true
`,
			wantBat:   true,
			wantKnown: true,
		},
		{
			name: "macOS desktop without battery",
			fixture: `Now drawing from 'AC Power'
`,
			wantBat:   false,
			wantKnown: true,
		},
		{
			name:      "empty output",
			fixture:   "",
			wantBat:   false,
			wantKnown: false,
		},
		{
			name:      "unknown output",
			fixture:   "Error reading battery status",
			wantBat:   false,
			wantKnown: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBat, gotKnown := parsePmsetBatt(tt.fixture)
			if gotBat != tt.wantBat || gotKnown != tt.wantKnown {
				t.Errorf("parsePmsetBatt() = (%v, %v); want (%v, %v)", gotBat, gotKnown, tt.wantBat, tt.wantKnown)
			}
		})
	}
}
