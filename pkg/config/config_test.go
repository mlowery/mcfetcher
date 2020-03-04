package config

import "testing"

func Test_parseGVKString(t *testing.T) {
	type args struct {
		gvkString string
	}
	tests := []struct {
		name        string
		args        args
		wantGroup   string
		wantVersion string
		wantKind    string
	}{
		{
			"kind only",
			args{"namespace"},
			"",
			"",
			"namespace",
		},
		{
			"gk only",
			args{"namespace."},
			"",
			"",
			"namespace",
		},
		{
			"gk only 2",
			args{"group.mlowery.github.com"},
			"mlowery.github.com",
			"",
			"group",
		},
		{
			"gvk",
			args{"group.v1alpha1.mlowery.github.com"},
			"mlowery.github.com",
			"v1alpha1",
			"group",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGroup, gotVersion, gotKind := parseGVKString(tt.args.gvkString)
			if gotGroup != tt.wantGroup {
				t.Errorf("parseGVKString() gotGroup = %v, wantGroup %v", gotGroup, tt.wantGroup)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("parseGVKString() gotVersion = %v, wantVersion %v", gotVersion, tt.wantVersion)
			}
			if gotKind != tt.wantKind {
				t.Errorf("parseGVKString() gotKind = %v, wantKind %v", gotKind, tt.wantKind)
			}
		})
	}
}
