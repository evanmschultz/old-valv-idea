package networkpolicy

import (
	"testing"

	dockeradapter "github.com/evanmschultz/valv/internal/adapters/docker"
)

func TestProxyImageRef(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   dockeradapter.ImageRef
	}{
		{
			name:   "no env var set uses default valv-proxy:dev",
			envVal: "",
			want:   dockeradapter.ImageRef{Repository: "valv-proxy", Tag: "dev"},
		},
		{
			name:   "VALV_PROXY_IMAGE with tag parses repo and tag",
			envVal: "repo/img:v1",
			want:   dockeradapter.ImageRef{Repository: "repo/img", Tag: "v1"},
		},
		{
			name:   "VALV_PROXY_IMAGE without tag sets repo and empty tag",
			envVal: "repo/img",
			want:   dockeradapter.ImageRef{Repository: "repo/img", Tag: ""},
		},
		{
			name:   "VALV_PROXY_IMAGE with colon after final slash uses tag",
			envVal: "registry.example.com/org/img:latest",
			want:   dockeradapter.ImageRef{Repository: "registry.example.com/org/img", Tag: "latest"},
		},
		{
			name:   "VALV_PROXY_IMAGE colon before final slash treats whole string as repo",
			envVal: "registry.example.com:5000/img",
			want:   dockeradapter.ImageRef{Repository: "registry.example.com:5000/img", Tag: ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VALV_PROXY_IMAGE", tc.envVal)
			got := proxyImageRef()
			if got != tc.want {
				t.Errorf("proxyImageRef() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
