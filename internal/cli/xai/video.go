package xai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost          = "127.0.0.1"
	defaultPort          = 20128
	defaultModel         = "xai/grok-imagine-video"
	defaultTimeout       = 10 * time.Minute
	defaultPollInterval  = 5 * time.Second
	maxDownloadRedirects = 5
)

type Options struct {
	Prompt, Output, Model, AspectRatio, Resolution, Image, APIKey, Host string
	Port, Duration                                                      int
	Timeout, PollInterval                                               time.Duration
}

func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	opts, help, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(errOut, "❌", err)
		return 1
	}
	if help {
		fmt.Fprintln(out, helpText())
		return 0
	}
	if opts.Prompt == "" {
		fmt.Fprintln(errOut, "❌ --prompt is required")
		return 1
	}
	body := map[string]any{"model": opts.Model, "prompt": opts.Prompt}
	if opts.Duration != 0 {
		body["duration"] = opts.Duration
	}
	if opts.AspectRatio != "" {
		body["aspect_ratio"] = opts.AspectRatio
	}
	if opts.Resolution != "" {
		body["resolution"] = opts.Resolution
	}
	if opts.Image != "" {
		image, err := imageInput(opts.Image)
		if err != nil {
			fmt.Fprintln(errOut, "❌", err)
			return 1
		}
		body["image"] = map[string]string{"url": image}
	}
	client := &http.Client{}
	base := "http://" + opts.Host + ":" + strconv.Itoa(opts.Port)
	created, status, err := request(ctx, client, http.MethodPost, base+"/v1/videos/generations", opts.APIKey, body)
	if err != nil {
		fmt.Fprintln(errOut, "❌", err)
		return 1
	}
	requestID, _ := created["request_id"].(string)
	if status != http.StatusOK || requestID == "" {
		fmt.Fprintf(errOut, "❌ Create failed: HTTP %d\n", status)
		return 1
	}
	deadline := time.Now().Add(opts.Timeout)
	var result map[string]any
	for {
		if time.Now().After(deadline) {
			fmt.Fprintln(errOut, "❌ video generation timed out")
			return 1
		}
		result, status, err = request(ctx, client, http.MethodGet, base+"/v1/videos/"+url.PathEscape(requestID), opts.APIKey, nil)
		if err != nil {
			fmt.Fprintln(errOut, "❌", err)
			return 1
		}
		if status < 200 || status >= 300 {
			fmt.Fprintf(errOut, "❌ Poll failed: HTTP %d\n", status)
			return 1
		}
		state, _ := result["status"].(string)
		if state == "done" || state == "completed" {
			break
		}
		if state == "failed" || state == "error" || state == "expired" || state == "cancelled" {
			fmt.Fprintln(errOut, "❌ video generation", state)
			return 1
		}
		select {
		case <-ctx.Done():
			fmt.Fprintln(errOut, "❌", ctx.Err())
			return 1
		case <-time.After(opts.PollInterval):
		}
	}
	video, _ := result["video"].(map[string]any)
	videoURL, _ := video["url"].(string)
	if videoURL == "" {
		if output, _ := video["file_output"].(map[string]any); output != nil {
			videoURL, _ = output["public_url"].(string)
		}
	}
	if videoURL == "" {
		fmt.Fprintln(errOut, "❌ Job finished but no video URL was returned")
		return 1
	}
	if err := download(ctx, client, videoURL, opts.Output); err != nil {
		fmt.Fprintln(errOut, "❌", err)
		return 1
	}
	fmt.Fprintln(out, "✅ Saved", opts.Output)
	return 0
}

func parseArgs(args []string) (Options, string, error) {
	opts := Options{Output: "video.mp4", Model: defaultModel, Host: defaultHost, Port: defaultPort, Timeout: defaultTimeout, PollInterval: defaultPollInterval, APIKey: os.Getenv("NINE_ROUTER_API_KEY")}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			return opts, helpText(), nil
		}
		value := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing value for %s", arg)
			}
			i++
			return args[i], nil
		}
		switch arg {
		case "--prompt":
			opts.Prompt, _ = value()
		case "--output", "-o":
			opts.Output, _ = value()
		case "--model":
			opts.Model, _ = value()
		case "--aspect-ratio":
			opts.AspectRatio, _ = value()
		case "--resolution":
			opts.Resolution, _ = value()
		case "--image":
			opts.Image, _ = value()
		case "--api-key":
			opts.APIKey, _ = value()
		case "--port", "-p":
			v, err := value()
			if err != nil {
				return opts, "", err
			}
			opts.Port, err = strconv.Atoi(v)
			if err != nil {
				return opts, "", err
			}
		case "--duration":
			v, err := value()
			if err != nil {
				return opts, "", err
			}
			opts.Duration, err = strconv.Atoi(v)
			if err != nil {
				return opts, "", err
			}
		case "--timeout":
			v, err := value()
			if err != nil {
				return opts, "", err
			}
			seconds, err := strconv.Atoi(v)
			if err != nil {
				return opts, "", err
			}
			opts.Timeout = time.Duration(seconds) * time.Second
		case "--poll-interval-ms":
			v, err := value()
			if err != nil {
				return opts, "", err
			}
			ms, err := strconv.Atoi(v)
			if err != nil {
				return opts, "", err
			}
			opts.PollInterval = time.Duration(ms) * time.Millisecond
		case "--host", "-H":
			opts.Host, _ = value()
		default:
			return opts, "", fmt.Errorf("unknown option: %s", arg)
		}
	}
	return opts, "", nil
}

func imageInput(input string) (string, error) {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "data:") {
		return input, nil
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(input))
	mime := "image/jpeg"
	if ext == ".png" {
		mime = "image/png"
	} else if ext == ".webp" {
		mime = "image/webp"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func request(ctx context.Context, client *http.Client, method, target, apiKey string, payload any) (map[string]any, int, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, err
	}
	var parsed map[string]any
	if len(data) != 0 {
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, res.StatusCode, err
		}
	}
	return parsed, res.StatusCode, nil
}

func download(ctx context.Context, client *http.Client, target, output string) error {
	part := output + ".part"
	defer os.Remove(part)
	for redirects := 0; redirects <= maxDownloadRedirects; redirects++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 && res.Header.Get("Location") != "" {
			target, err = res.Location()
			res.Body.Close()
			if err != nil {
				return err
			}
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return fmt.Errorf("download failed: HTTP %d", res.StatusCode)
		}
		file, err := os.Create(part)
		if err != nil {
			res.Body.Close()
			return err
		}
		_, copyErr := io.Copy(file, res.Body)
		closeErr := file.Close()
		res.Body.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Rename(part, output)
	}
	return errors.New("too many redirects")
}

func helpText() string { return "Usage: 9router xai video --prompt \"...\" [options]" }
