package flags

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want Parsed
	}{
		{
			name: "chart only",
			argv: []string{"./mychart"},
			want: Parsed{Chart: "./mychart"},
		},
		{
			name: "-f translates to --values",
			argv: []string{"./c", "-f", "values-prod.yaml"},
			want: Parsed{Chart: "./c", ValueFiles: []string{"values-prod.yaml"}},
		},
		{
			name: "--values long form",
			argv: []string{"./c", "--values", "v.yaml"},
			want: Parsed{Chart: "./c", ValueFiles: []string{"v.yaml"}},
		},
		{
			name: "--values=PATH inline form",
			argv: []string{"./c", "--values=v.yaml"},
			want: Parsed{Chart: "./c", ValueFiles: []string{"v.yaml"}},
		},
		{
			name: "-f a.yaml,b.yaml is two files (Helm splits on comma)",
			argv: []string{"./c", "-f", "a.yaml,b.yaml"},
			want: Parsed{Chart: "./c", ValueFiles: []string{"a.yaml", "b.yaml"}},
		},
		{
			name: "repeatable -f",
			argv: []string{"./c", "-f", "a.yaml", "-f", "b.yaml"},
			want: Parsed{Chart: "./c", ValueFiles: []string{"a.yaml", "b.yaml"}},
		},
		{
			name: "-n translates to --release-namespace",
			argv: []string{"./c", "-n", "prod"},
			want: Parsed{Chart: "./c", ReleaseNamespace: "prod"},
		},
		{
			name: "--namespace long form translates to --release-namespace",
			argv: []string{"./c", "--namespace", "prod"},
			want: Parsed{Chart: "./c", ReleaseNamespace: "prod"},
		},
		{
			name: "--release-namespace wins over -n when both given",
			argv: []string{"./c", "-n", "from-n", "--release-namespace", "from-rn"},
			want: Parsed{Chart: "./c", ReleaseNamespace: "from-rn"},
		},
		{
			name: "--release-name passthrough",
			argv: []string{"./c", "--release-name", "myrel"},
			want: Parsed{Chart: "./c", ReleaseName: "myrel"},
		},
		{
			name: "--set tolerations={a,b} preserves brace-comma",
			argv: []string{"./c", "--set", "tolerations={a,b}"},
			want: Parsed{Chart: "./c", SetValues: []string{"tolerations={a,b}"}},
		},
		{
			name: "repeatable --set",
			argv: []string{"./c", "--set", "a=1", "--set", "b=2"},
			want: Parsed{Chart: "./c", SetValues: []string{"a=1", "b=2"}},
		},
		{
			name: "--set-string and --set-file passthrough",
			argv: []string{"./c", "--set-string", "tag=1", "--set-file", "cert=./tls.pem"},
			want: Parsed{Chart: "./c", SetStringValues: []string{"tag=1"}, SetFileValues: []string{"cert=./tls.pem"}},
		},
		{
			name: "kubescape-native flags fall through to passthrough",
			argv: []string{"./c", "--format", "json", "--severity-threshold", "high"},
			want: Parsed{Chart: "./c", Passthrough: []string{"--format", "json", "--severity-threshold", "high"}},
		},
		{
			name: "passthrough boolean flag (no value)",
			argv: []string{"./c", "--verbose"},
			want: Parsed{Chart: "./c", Passthrough: []string{"--verbose"}},
		},
		{
			name: "-- terminates flag parsing",
			argv: []string{"./c", "--", "--values", "looks-like-flag-but-isnt"},
			want: Parsed{Chart: "./c", Passthrough: []string{"--values", "looks-like-flag-but-isnt"}},
		},
		{
			name: "mixed helm and kubescape flags",
			argv: []string{"./c", "-f", "v.yaml", "--set", "a=1", "--release-name", "r", "-n", "ns", "--format", "json"},
			want: Parsed{
				Chart:            "./c",
				ValueFiles:       []string{"v.yaml"},
				SetValues:        []string{"a=1"},
				ReleaseName:      "r",
				ReleaseNamespace: "ns",
				Passthrough:      []string{"--format", "json"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.argv)
			if err != nil {
				t.Fatalf("Parse(%v) returned error: %v", tt.argv, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%v)\n got: %+v\nwant: %+v", tt.argv, got, tt.want)
			}
		})
	}
}

func TestKubescapeArgs(t *testing.T) {
	p := Parsed{
		Chart:            "./c",
		ValueFiles:       []string{"a.yaml", "b.yaml"},
		SetValues:        []string{"a=1"},
		SetStringValues:  []string{"tag=v2"},
		SetFileValues:    []string{"cert=./tls.pem"},
		ReleaseName:      "myrel",
		ReleaseNamespace: "prod",
		Passthrough:      []string{"--format", "json", "--output", "out.json"},
	}
	want := []string{
		"./c",
		"--values", "a.yaml",
		"--values", "b.yaml",
		"--set", "a=1",
		"--set-string", "tag=v2",
		"--set-file", "cert=./tls.pem",
		"--release-name", "myrel",
		"--release-namespace", "prod",
		"--format", "json", "--output", "out.json",
	}
	got := p.KubescapeArgs()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KubescapeArgs()\n got: %v\nwant: %v", got, want)
	}
}

func TestKubescapeArgs_chartOnly(t *testing.T) {
	p := Parsed{Chart: "./c"}
	got := p.KubescapeArgs()
	want := []string{"./c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KubescapeArgs()\n got: %v\nwant: %v", got, want)
	}
}
