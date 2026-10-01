//go:build invidious_live

// Live integration tests. They talk to the real Invidious public instance list
// and reproduce the production failure reported for getVideo().
//
// Run with:
//
//	go test -tags invidious_live -v -timeout 5m ./invidious/
package invidious

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/darkhz/invidtui/client"
	"github.com/darkhz/invidtui/cmd"
)

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// videoID is a stable, long-lived video used for probing.
const videoID = "dQw4w9WgXcQ"

type instanceInfo struct {
	Flag   string `json:"flag"`
	Region string `json:"region"`
	API    bool   `json:"api"`
	Type   string `json:"type"`
	URI    string `json:"uri"`
	Stats  struct {
		Version  string `json:"version"`
		Software struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Branch  string `json:"branch"`
		} `json:"software"`
	} `json:"stats"`
	Monitor struct {
		Uptime    float64 `json:"uptime"`
		Down      bool    `json:"down"`
		UpSince   string  `json:"up_since"`
		DownSince *string `json:"down_since"`
	} `json:"monitor"`
}

// instances.json is a list of [name, metadata] pairs.
type instanceEntry []json.RawMessage

func fetchInstances(t *testing.T) []instanceInfo {
	t.Helper()

	res, err := http.Get(client.InstanceData)
	if err != nil {
		t.Skipf("no network: %v", err)
	}
	defer res.Body.Close()

	var entries []instanceEntry
	if err := json.NewDecoder(res.Body).Decode(&entries); err != nil {
		t.Fatalf("decode instances.json: %v", err)
	}

	var out []instanceInfo
	for _, e := range entries {
		if len(e) < 2 {
			continue
		}
		var name string
		if err := json.Unmarshal(e[0], &name); err != nil {
			continue
		}
		var info instanceInfo
		if err := json.Unmarshal(e[1], &info); err != nil {
			continue
		}
		t.Logf("instance=%-30s api=%-5v type=%-8s region=%-3s invidious=%s uptime=%.1f%% down=%v",
			name, info.API, info.Type, info.Region,
			info.Stats.Software.Version, info.Monitor.Uptime, info.Monitor.Down)
		out = append(out, info)
	}
	return out
}

func TestLive_InstanceList(t *testing.T) {
	client.Init()

	instances := fetchInstances(t)
	if len(instances) == 0 {
		t.Skip("no instances returned")
	}

	var api int
	for _, i := range instances {
		if i.API {
			api++
		}
	}
	t.Logf("api.invidious.io returned %d instances, %d with api=true", len(instances), api)
}

// Probes /api/v1/videos/:id on every API-enabled instance and reports the
// status code and whether the body is decodable into a VideoData.
func TestLive_GetVideoAcrossInstances(t *testing.T) {
	client.Init()

	apiInstances := []instanceInfo{}
	for _, i := range fetchInstances(t) {
		if i.API {
			apiInstances = append(apiInstances, i)
		}
	}
	if len(apiInstances) == 0 {
		t.Skip("no API-enabled instances")
	}

	var ok, failed int

	for _, inst := range apiInstances {
		t.Run(inst.URI, func(t *testing.T) {
			client.Init()
			client.SetHost(inst.URI)

			ctx, cancel := contextWithTimeout(30 * time.Second)
			defer cancel()

			resp, fetchErr := client.Fetch(ctx, "videos/"+videoID)
			if fetchErr != nil {
				t.Errorf("getVideo(%s): %v", inst.URI, fetchErr)
				failed++
				return
			}
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			snippet := strings.TrimSpace(string(body[:n]))
			if len(snippet) > 160 {
				snippet = snippet[:160] + "..."
			}

			t.Logf("HTTP %d  %s", resp.StatusCode, snippet)

			video, err := getVideo(ctx, videoID)
			if err != nil {
				t.Errorf("getVideo(%s): %v", inst.URI, err)
				failed++
				return
			}

			t.Logf("decoded: title=%q author=%q %ds views=%d likes=%d",
				video.Title, video.Author, video.LengthSeconds,
				video.ViewCount, video.LikeCount)
			t.Logf("  thumbnails=%d formatStreams=%d adaptiveFormats=%d recommended=%d liveNow=%v hlsUrl=%q",
				len(video.Thumbnails), len(video.FormatStreams),
				len(video.AdaptiveFormats), len(video.RecommendedVideos),
				video.LiveNow, video.HlsURL)

			if len(video.AdaptiveFormats) == 0 {
				t.Errorf("adaptiveFormats is empty: playback will fail with " +
					"\"No video URI\" (video.go:170)")
				failed++
				return
			}
			for i, f := range video.AdaptiveFormats {
				if i >= 4 {
					break
				}
				t.Logf("  fmt[%d] itag=%-4s res=%-6s br=%-9d clen=%-10d type=%s",
					i, f.Itag, f.Resolution, f.Bitrate, f.ContentLength, f.Type)
			}

			// Exercise the format-selection path against real data.
			cmd.InitConfig()
			for _, res := range []string{"1080p", "720p", "480p", "360p"} {
				cmd.SetOptionValue("video-res", res)
				vURL, aURL := getVideoByItag(video, false)
				t.Logf("  res=%-6s video=%s audio=%s", res, truncate(vURL, 90), truncate(aURL, 40))
			}
			cmd.SetOptionValue("video-res", "720p")

			ok++
		})
	}

	t.Logf("SUMMARY: %d/%d API instances served a usable /api/v1/videos/%s",
		ok, ok+failed, videoID)
}

// Probes the lighter /search endpoint on the same instances for contrast: it
// still works even where /videos/:id does not.
func TestLive_SearchAcrossInstances(t *testing.T) {
	client.Init()

	for _, inst := range fetchInstances(t) {
		if !inst.API {
			continue
		}

		t.Run(inst.URI, func(t *testing.T) {
			client.Init()
			client.SetHost(inst.URI)

			// Search() calls client.Cancel() internally, so it uses its own ctx.
			data, _, err := Search("video", "hello", nil, 1)
			if err != nil {
				t.Errorf("search(%s): %v", inst.URI, err)
				return
			}
			if len(data) == 0 {
				t.Logf("search(%s): 0 results", inst.URI)
				return
			}
			t.Logf("search returned %d results; first: %q by %q",
				len(data), data[0].Title, data[0].Author)
		})
	}
}

// Probes /latest_version, the non-API endpoint getLatestURL() points mpv at.
func TestLive_LatestVersionEndpoint(t *testing.T) {
	client.Init()

	for _, inst := range fetchInstances(t) {
		if !inst.API {
			continue
		}

		t.Run(inst.URI, func(t *testing.T) {
			client.Init()
			client.SetHost(inst.URI)

			ctx, cancel := contextWithTimeout(30 * time.Second)
			defer cancel()

			uri := getLatestURL(videoID, "22")
			t.Logf("getLatestURL -> %s", uri)

			resp, err := client.Get(ctx, "/latest_version?id="+videoID+"&itag=22&local=true")
			if err != nil {
				t.Errorf("latest_version(%s): %v", inst.URI, err)
				return
			}
			defer resp.Body.Close()

			t.Logf("HTTP %d  content-type=%q  content-length=%d",
				resp.StatusCode, resp.Header.Get("Content-Type"), resp.ContentLength)
		})
	}
}

// Probes the raw endpoint for a single instance given by INVIDIOUS_HOST.
func TestLive_SingleHost(t *testing.T) {
	host := os.Getenv("INVIDIOUS_HOST")
	if host == "" {
		t.Skip("set INVIDIOUS_HOST to probe a specific instance")
	}

	client.Init()
	client.SetHost(host)

	ctx, cancel := contextWithTimeout(60 * time.Second)
	defer cancel()

	video, err := getVideo(ctx, videoID)
	if err != nil {
		t.Fatalf("getVideo(%s): %v", host, err)
	}

	fmt.Printf("title:    %s\n", video.Title)
	fmt.Printf("author:   %s (%s)\n", video.Author, video.AuthorID)
	fmt.Printf("duration: %ds\n", video.LengthSeconds)
	fmt.Printf("views:    %d\n", video.ViewCount)
	fmt.Printf("likes:    %d\n", video.LikeCount)
	fmt.Printf("liveNow:  %v\n", video.LiveNow)
	fmt.Printf("thumbs:   %d\n", len(video.Thumbnails))
	fmt.Printf("formats:  %d / %d\n", len(video.FormatStreams), len(video.AdaptiveFormats))

	for i, f := range video.AdaptiveFormats {
		if i >= 8 {
			break
		}
		fmt.Printf("  itag=%-4s res=%-6s br=%-9d clen=%-10d ch=%d rate=%d %s\n",
			f.Itag, f.Resolution, f.Bitrate, f.ContentLength,
			f.AudioChannels, f.AudioSampleRate, f.Type)
	}
}
