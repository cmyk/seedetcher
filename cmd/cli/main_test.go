package main

import "testing"

func TestParseLaserPassOrderFlag(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "", want: "grouped"},
		{in: "grouped", want: "grouped"},
		{in: "local", want: "local"},
		{in: "sweep", want: "sweep"},
		{in: "global", want: "sweep"},
		{in: "GLOBAL", want: "sweep"},
		{in: "invalid", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseLaserPassOrderFlag(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("parseLaserPassOrderFlag(%q): expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseLaserPassOrderFlag(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("parseLaserPassOrderFlag(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}
