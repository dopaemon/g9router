package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetName(t *testing.T) {
	if got := AssetName("linux", "arm64"); got != "g9router-linux-arm64.tar.gz" {
		t.Fatalf("asset = %q", got)
	}
}

func TestLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/dopaemon/g9router/releases/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Accept") == "" {
			t.Fatal("missing Accept header")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","assets":[{"name":"g9router-linux-amd64.tar.gz","browser_download_url":"https://example.test/a"}]}`))
	}))
	defer server.Close()
	client := server.Client()
	transport := client.Transport
	client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		request.URL.Scheme = "http"
		request.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return transport.RoundTrip(request)
	})
	release, err := Latest(context.Background(), client, "dopaemon/g9router")
	if err != nil || release.TagName != "v1.2.3" || len(release.Assets) != 1 {
		t.Fatalf("release=%#v err=%v", release, err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }
