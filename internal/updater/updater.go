package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name      string `json:"name"`
	URL       string `json:"browser_download_url"`
	MediaType string `json:"content_type"`
}

func Latest(ctx context.Context, client *http.Client, repository string) (Release, error) {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+strings.Trim(repository, "/")+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("latest release: HTTP %s", response.Status)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func AssetName(goos, goarch string) string {
	return fmt.Sprintf("g9router-%s-%s.tar.gz", goos, goarch)
}

func Update(ctx context.Context, client *http.Client, release Release, executable string) error {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	var asset Asset
	for _, candidate := range release.Assets {
		if candidate.Name == name {
			asset = candidate
			break
		}
	}
	if asset.URL == "" {
		return fmt.Errorf("release %s has no asset %s", release.TagName, name)
	}
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("download release: HTTP %s", response.Status)
	}
	archive, err := os.CreateTemp(filepath.Dir(executable), ".g9router-update-*.tar.gz")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	if _, err := io.Copy(archive, io.LimitReader(response.Body, 512<<20)); err != nil {
		archive.Close()
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	input, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		input.Close()
		return err
	}
	tarReader := tar.NewReader(gzipReader)
	temp, err := os.CreateTemp(filepath.Dir(executable), ".g9router-binary-*")
	if err != nil {
		gzipReader.Close()
		input.Close()
		return err
	}
	tempPath := temp.Name()
	cleanup := func() { temp.Close(); os.Remove(tempPath); gzipReader.Close(); input.Close() }
	found := false
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			cleanup()
			return readErr
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "g9router" && filepath.Base(header.Name) != "g9router.exe" {
			continue
		}
		if _, err := io.Copy(temp, io.LimitReader(tarReader, 512<<20)); err != nil {
			cleanup()
			return err
		}
		found = true
		break
	}
	if err := temp.Close(); err != nil || gzipReader.Close() != nil || input.Close() != nil {
		cleanup()
		return fmt.Errorf("close update archive")
	}
	if !found {
		os.Remove(tempPath)
		return fmt.Errorf("archive %s contains no g9router binary", asset.Name)
	}
	if err := os.Chmod(tempPath, 0755); err != nil {
		os.Remove(tempPath)
		return err
	}
	backup := executable + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(executable, backup); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, executable); err != nil {
		_ = os.Rename(backup, executable)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
