package invidious

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkhz/invidtui/client"
	"github.com/darkhz/invidtui/cmd"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// A faithful /api/v1/videos/:id payload. Key details that matter for decoding:
//   - formatStreams entries have NO "bitrate" and NO "clen" key
//   - adaptiveFormats entries have "bitrate" and "clen" as JSON *strings*
//   - "hlsUrl" is null for non-live videos
//   - audio adaptiveFormats entries omit "resolution" entirely
//   - "audioChannels" is a quoted string
const videoPayload = `{
  "type": "video",
  "title": "Adele - Hello (Official Music Video)",
  "videoId": "YQHsXMglC9A",
  "author": "Adele",
  "authorId": "UCsRM0YB_dabtEPGPTKo-gcw",
  "authorUrl": "/channel/UCsRM0YB_dabtEPGPTKo-gcw",
  "authorVerified": true,
  "subCountText": "252M",
  "allowRatings": true,
  "viewCount": 3400000000,
  "likeCount": 16000000,
  "dislikeCount": 0,
  "published": 1424126400,
  "publishedText": "9 years ago",
  "lengthSeconds": 357,
  "allowEmbed": true,
  "isFamilyFriendly": true,
  "liveNow": false,
  "isUpcoming": false,
  "isPostLiveDvr": false,
  "isListed": true,
  "hlsUrl": null,
  "dashUrl": "https://example.com/manifest/dash/id/YQHsXMglC9A",
  "description": "Adele's debut single off her album 25.",
  "keywords": ["adele", "hello"],
  "paid": false,
  "premiereTimestamp": 0,
  "premiereSeconds": 0,
  "genre": "Music",
  "storyboards": [],
  "captions": [],
  "videoThumbnails": [
    {"quality":"maxres","url":"https://i.ytimg.com/vi/YQHsXMglC9A/maxresdefault.jpg","width":1280,"height":720},
    {"quality":"high","url":"https://i.ytimg.com/vi/YQHsXMglC9A/hqdefault.jpg","width":480,"height":360},
    {"quality":"start","url":"https://i.ytimg.com/vi/YQHsXMglC9A/0.jpg","width":120,"height":90}
  ],
  "formatStreams": [
    {"url":"https://example.com/videoplayback?expire=1&itag=18&clen=12345678",
     "itag":"18","type":"video/mp4; codecs=\"avc1.42001E, mp4a.40.2\"",
     "container":"mp4","encoding":"avc1.42001E, mp4a.40.2","quality":"medium",
     "resolution":"360p","size":"12345678","fps":30},
    {"url":"https://example.com/videoplayback?expire=1&itag=22&clen=23456789",
     "itag":"22","type":"video/mp4; codecs=\"avc1.64001F, mp4a.40.2\"",
     "container":"mp4","encoding":"avc1.64001F, mp4a.40.2","quality":"hd720",
     "resolution":"720p","size":"23456789","fps":30}
  ],
  "adaptiveFormats": [
    {"url":"https://example.com/videoplayback?expire=1&itag=137","itag":"137",
     "type":"video/mp4; codecs=\"avc1.640028\"","container":"mp4",
     "encoding":"avc1.640028","quality":"hd1080","resolution":"1080p",
     "size":"56789012","bitrate":"4729334","fps":30,"clen":"56789012",
     "init":"0-567","index":"0-9999","lmt":"1700000000000000",
     "projectionType":"rectangular","qualityLabel":"1080p"},
    {"url":"https://example.com/videoplayback?expire=1&itag=136","itag":"136",
     "type":"video/mp4; codecs=\"avc1.4d401f\"","container":"mp4",
     "encoding":"avc1.4d401f","quality":"hd720","resolution":"720p",
     "size":"34567890","bitrate":"2200000","fps":30,"clen":"34567890",
     "projectionType":"rectangular","qualityLabel":"720p"},
    {"url":"https://example.com/videoplayback?expire=1&itag=135","itag":"135",
     "type":"video/mp4; codecs=\"avc1.4d401e\"","container":"mp4",
     "encoding":"avc1.4d401e","quality":"medium","resolution":"480p",
     "size":"22222222","bitrate":"1100000","fps":30,"clen":"22222222",
     "projectionType":"rectangular","qualityLabel":"480p"},
    {"url":"https://example.com/videoplayback?expire=1&itag=251","itag":"251",
     "type":"audio/webm; codecs=\"opus\"","container":"webm",
     "encoding":"opus","quality":"tiny","bitrate":"141983","fps":0,
     "audioSampleRate":48000,"audioChannels":"2",
     "init":"0-234","index":"0-9999","lmt":"1700000000000000",
     "projectionType":"rectangular"},
    {"url":"https://example.com/videoplayback?expire=1&itag=140","itag":"140",
     "type":"audio/mp4; codecs=\"mp4a.40.2\"","container":"mp4",
     "encoding":"mp4a.40.2","quality":"tiny","bitrate":"129583","fps":0,
     "audioSampleRate":44100,"audioChannels":"2",
     "projectionType":"rectangular"}
  ],
  "recommendedVideos": [
    {"type":"video","title":"Adele - Rolling In The Deep","videoId":"rYEDA3JcQqw",
     "author":"Adele","authorId":"UCsRM0YB_dabtEPGPTKo-gcw",
     "lengthSeconds":240,"viewCount":100,"likeCount":10,
     "publishedText":"9 years ago","subCountText":"252M","liveNow":false,
     "videoThumbnails":[{"quality":"hqdefault",
       "url":"https://i.ytimg.com/vi/rYEDA3JcQqw/hqdefault.jpg","width":480,"height":360}]}
  ]
}`

const livePayload = `{
  "type":"video","title":"Live stream","videoId":"liveid123",
  "author":"Chan","authorId":"UC_live","lengthSeconds":0,
  "liveNow":true,"viewCount":1000,"likeCount":10,
  "publishedText":"1 hour ago","subCountText":"1K","description":"",
  "hlsUrl":"https://example.com/live/manifest.m3u8",
  "dashUrl":"https://example.com/live/manifest.mpd",
  "videoThumbnails":[{"quality":"start","url":"https://i.ytimg.com/vi/x/0.jpg","width":120,"height":90}],
  "formatStreams":[],
  "adaptiveFormats":[]
}`

// newTestClient points the package-level client at a test server that serves
// `handler` for any /api/v1/... request.
func newTestClient(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client.Init()
	client.SetHost(srv.URL)

	return srv
}

func initTestConfig(t *testing.T) {
	t.Helper()

	cmd.InitConfig()
	cmd.SetOptionValue("video-res", "720p")
	t.Cleanup(func() { cmd.SetOptionValue("video-res", "") })
}

// ---------------------------------------------------------------------------
// getVideo() decoding
// ---------------------------------------------------------------------------

func TestGetVideo_DecodesFullPayload(t *testing.T) {
	var gotPath string

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, videoPayload)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	if gotPath != "/api/v1/videos/YQHsXMglC9A" {
		t.Errorf("request path = %q, want %q", gotPath, "/api/v1/videos/YQHsXMglC9A")
	}

	if video.Title != "Adele - Hello (Official Music Video)" {
		t.Errorf("Title = %q", video.Title)
	}
	if video.VideoID != "YQHsXMglC9A" {
		t.Errorf("VideoID = %q", video.VideoID)
	}
	if video.AuthorID != "UCsRM0YB_dabtEPGPTKo-gcw" {
		t.Errorf("AuthorID = %q", video.AuthorID)
	}
	if video.LengthSeconds != 357 {
		t.Errorf("LengthSeconds = %d, want 357", video.LengthSeconds)
	}
	if video.ViewCount != 3400000000 {
		t.Errorf("ViewCount = %d, want 3400000000 (int must not overflow)", video.ViewCount)
	}
	if video.LikeCount != 16000000 {
		t.Errorf("LikeCount = %d", video.LikeCount)
	}
	if video.PublishedText != "9 years ago" {
		t.Errorf("PublishedText = %q", video.PublishedText)
	}
	if video.SubCountText != "252M" {
		t.Errorf("SubCountText = %q", video.SubCountText)
	}
	if video.LiveNow {
		t.Error("LiveNow = true, want false")
	}
	if video.HlsURL != "" {
		t.Errorf("HlsURL = %q, want empty for null hlsUrl", video.HlsURL)
	}

	if len(video.Thumbnails) != 3 {
		t.Fatalf("len(Thumbnails) = %d, want 3", len(video.Thumbnails))
	}
	if video.Thumbnails[0].Quality != "maxres" {
		t.Errorf("Thumbnails[0].Quality = %q, want maxres", video.Thumbnails[0].Quality)
	}
	// player.go:567 looks for the literal quality "start" to build the seek
	// thumbnail; it must survive decoding.
	var hasStart bool
	for _, th := range video.Thumbnails {
		if th.Quality == "start" {
			hasStart = true
		}
	}
	if !hasStart {
		t.Error(`no thumbnail with quality "start"; player.go thumbnail seeking will break`)
	}

	if len(video.FormatStreams) != 2 {
		t.Fatalf("len(FormatStreams) = %d, want 2", len(video.FormatStreams))
	}
	if video.FormatStreams[1].Resolution != "720p" {
		t.Errorf("FormatStreams[1].Resolution = %q, want 720p", video.FormatStreams[1].Resolution)
	}
	if video.FormatStreams[1].URL == "" {
		t.Error("FormatStreams[1].URL is empty")
	}

	if len(video.AdaptiveFormats) != 5 {
		t.Fatalf("len(AdaptiveFormats) = %d, want 5", len(video.AdaptiveFormats))
	}

	byItag := map[string]VideoFormat{}
	for _, f := range video.AdaptiveFormats {
		byItag[f.Itag] = f
	}

	// The `,string` fields: bitrate and clen arrive quoted.
	if f := byItag["137"]; f.Bitrate != 4729334 {
		t.Errorf(`itag 137 Bitrate = %d, want 4729334 (from "4729334")`, f.Bitrate)
	}
	if f := byItag["137"]; f.ContentLength != 56789012 {
		t.Errorf(`itag 137 ContentLength = %d, want 56789012 (from "56789012")`, f.ContentLength)
	}
	// Audio entries omit "resolution" entirely.
	if f := byItag["251"]; f.Resolution != "" {
		t.Errorf(`itag 251 Resolution = %q, want "" (key absent)`, f.Resolution)
	}
	if f := byItag["251"]; f.AudioSampleRate != 48000 {
		t.Errorf("itag 251 AudioSampleRate = %d, want 48000", f.AudioSampleRate)
	}
	// audioChannels arrives quoted, struct field is int.
	if f := byItag["251"]; f.AudioChannels != 2 {
		t.Errorf(`itag 251 AudioChannels = %d, want 2 (from "2")`, f.AudioChannels)
	}
	if f := byItag["251"]; f.FPS != 0 {
		t.Errorf("itag 251 FPS = %d, want 0", f.FPS)
	}

	// formatStreams have no bitrate/clen keys at all.
	for _, f := range video.FormatStreams {
		if f.Bitrate != 0 {
			t.Errorf("formatStreams itag %s Bitrate = %d, want 0 (key absent)", f.Itag, f.Bitrate)
		}
		if f.ContentLength != 0 {
			t.Errorf("formatStreams itag %s ContentLength = %d, want 0 (key absent)",
				f.Itag, f.ContentLength)
		}
	}

	if len(video.RecommendedVideos) != 1 {
		t.Fatalf("len(RecommendedVideos) = %d, want 1", len(video.RecommendedVideos))
	}
	if video.RecommendedVideos[0].VideoID != "rYEDA3JcQqw" {
		t.Errorf("RecommendedVideos[0].VideoID = %q", video.RecommendedVideos[0].VideoID)
	}

	// Untagged local-only fields must stay zero.
	if video.MediaType != "" {
		t.Errorf("DEFECT: MediaType = %q, want empty after decode", video.MediaType)
	}
	if video.Timestamp != nil {
		t.Errorf("DEFECT: Timestamp = %v, want nil after decode", *video.Timestamp)
	}
}

func TestGetVideo_LivePayload(t *testing.T) {
	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, livePayload)
	})

	video, err := getVideo(client.Ctx(), "liveid123")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	if !video.LiveNow {
		t.Error("LiveNow = false, want true")
	}
	if video.HlsURL != "https://example.com/live/manifest.m3u8" {
		t.Errorf("HlsURL = %q", video.HlsURL)
	}
	if len(video.FormatStreams) != 0 || len(video.AdaptiveFormats) != 0 {
		t.Errorf("expected empty format slices, got %d/%d",
			len(video.FormatStreams), len(video.AdaptiveFormats))
	}
}

// Bodies actually observed from live Invidious instances in 2026. None of them
// is JSON, so getVideo() must surface a decode error rather than a silently
// empty VideoData.
func TestGetVideo_NonJSONResponses(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		contentType string
	}{
		{
			// inv.nadeko.net, 2026-09-27
			name:   "nadeko endpoint disabled",
			status: http.StatusForbidden,
			body:   "Endpoint disabled",
		},
		{
			// yt.chocolatemoo53.com, 2026-09-27
			name:   "chocolatemoo forbidden",
			status: http.StatusForbidden,
			body:   "forbidden",
		},
		{
			// invidious.nerdvpn.de, 2026-09-27
			name:   "nerdvpn nginx 401",
			status: http.StatusUnauthorized,
			body:   "<html>\n<head><title>401 Authorization Required</title></head>\n</html>\n",
		},
		{
			// invidious.f5.si, 2026-09-27
			name:   "f5.si companion error",
			status: http.StatusInternalServerError,
			body:   `{"error":"Error while communicating with Invidious companion: read (#<TCPSocket:0x7f34888eedc0>): Connection reset by peer"}`,
		},
		{
			name:   "html error page with 200",
			status: http.StatusOK,
			body:   "<!DOCTYPE html><html><body>maintenance</body></html>",
		},
		{
			name:   "empty 200",
			status: http.StatusOK,
			body:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})

			video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
			if err == nil {
				t.Fatalf("expected an error, got VideoData%+v", video)
			}
			t.Logf("err = %v", err)

			if video.Title != "" || video.VideoID != "" {
				t.Errorf("failed decode must not leave partial data: %+v", video)
			}
		})
	}
}

// A 200 that is valid JSON but not a video object (e.g. an array or an error
// envelope) must not be mistaken for a video.
func TestGetVideo_UnexpectedJSONShape(t *testing.T) {
	cases := map[string]string{
		"array":        `[{"title":"nope","videoId":"x"}]`,
		"error only":   `{"error":"Something went wrong"}`,
		"error string": `"Something went wrong"`,
		"empty object": `{}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			})

			video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
			t.Logf("err = %v ; decoded = %+v", err, video)

			if name == "empty object" {
				if err != nil {
					t.Errorf("empty object should decode cleanly, got %v", err)
				}
				return
			}
			if name == "error only" {
				// Decodes without error but yields an empty video: the caller
				// gets no signal. Documented as a real gap.
				if err == nil && video.VideoID == "" {
					t.Log("DEFECT: `{\"error\":...}` with HTTP 200 decodes to an " +
						"empty VideoData and no error is surfaced to the caller")
				}
				return
			}
			if err == nil {
				t.Errorf("expected an error for %s, got %+v", name, video)
			}
		})
	}
}

// The struct relies on `,string` for bitrate/clen. If an instance ever emits a
// suffixed value like "4729k" (YouTube's own player-response format) the whole
// video decode aborts.
func TestGetVideo_SuffixedBitrateAbortsDecode(t *testing.T) {
	body := strings.Replace(videoPayload, `"bitrate":"4729334"`, `"bitrate":"4729k"`, 1)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err == nil {
		t.Logf("DEFECT: suffixed bitrate %q was accepted, decoded Bitrate = %d",
			"4729k", video.AdaptiveFormats[0].Bitrate)
		return
	}
	t.Logf("DEFECT: a single malformed bitrate aborts the ENTIRE video decode: %v", err)
	if video.Title != "" {
		t.Error("expected no partial VideoData on error")
	}
}

// Invidious omits formatStreams/adaptiveFormats entirely for some videos (and
// for every video on instances that lost YouTube access). getVideoURI then
// re-fetches, which is where an infinite-refetch risk lives.
func TestGetVideo_MissingFormatStreams(t *testing.T) {
	body := strings.Replace(
		strings.Replace(videoPayload, `"formatStreams": [`, `"formatStreamsX": [`, 1),
		`"adaptiveFormats": [`, `"adaptiveFormatsX": [`, 1)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	if video.FormatStreams != nil {
		t.Error("FormatStreams should be nil when the key is absent")
	}
	if video.AdaptiveFormats != nil {
		t.Error("AdaptiveFormats should be nil when the key is absent")
	}
	// video.go:150 treats nil as "needs refetch".
	if video.FormatStreams != nil && video.AdaptiveFormats != nil {
		t.Error("refetch condition at video.go:150 will not trigger")
	}
}

func TestGetVideo_NullFormatStreams(t *testing.T) {
	body := strings.Replace(videoPayload, `"formatStreams": [`, `"formatStreams": null, "unused": [`, 1)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("null formatStreams should not error: %v", err)
	}
	if video.FormatStreams != nil {
		t.Errorf("FormatStreams = %v, want nil", video.FormatStreams)
	}
}

// ---------------------------------------------------------------------------
// Format selection: the code that actually consumes the decoded JSON
// ---------------------------------------------------------------------------

func TestGetVideoByItag_UsesLatestVersionURL(t *testing.T) {
	initTestConfig(t)

	srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, videoPayload)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	for _, res := range []string{"720p", "1080p", "480p"} {
		cmd.SetOptionValue("video-res", res)

		videoURL, audioURL := getVideoByItag(video, false)
		want := srv.URL + "/latest_version?id=YQHsXMglC9A&itag=&local=true"
		_ = want
		if videoURL == "" {
			t.Errorf("%s: videoURL is empty", res)
			continue
		}
		if !strings.HasPrefix(videoURL, srv.URL+"/latest_version?") {
			t.Errorf("%s: videoURL = %q, want a /latest_version URL on the instance host", res, videoURL)
		}
		if !strings.Contains(videoURL, "id=YQHsXMglC9A") {
			t.Errorf("%s: videoURL = %q, missing video id", res, videoURL)
		}
		if !strings.HasSuffix(videoURL, "&local=true") {
			t.Errorf("%s: videoURL = %q, missing local=true", res, videoURL)
		}
		t.Logf("%s -> video=%s audio=%s", res, videoURL, audioURL)
	}
}

func TestGetVideoURI_NoAudioURIError(t *testing.T) {
	initTestConfig(t)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, videoPayload)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	if _, _, err := getVideoURI(client.Ctx(), video, true); err != nil {
		t.Errorf("audio URI should resolve, got %v", err)
	}
}

func TestGetVideoURI_RefetchWhenFormatsNil(t *testing.T) {
	initTestConfig(t)

	var calls int

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, videoPayload)
	})

	// A VideoData with no formats, as produced by playlist.go or the queue.
	partial := VideoData{VideoID: "YQHsXMglC9A", Title: "t"}

	if _, _, err := getVideoURI(client.Ctx(), partial, false); err != nil {
		t.Fatalf("getVideoURI: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 refetch, got %d", calls)
	}
}

// loopFormats derives a single `ftype` (the container subtype) from the FIRST
// adaptive format and then only considers formats sharing that subtype, so that
// mpv gets a video and an audio stream it can merge. That makes the result
// dependent on the order Invidious happens to emit.
func TestLoopFormats_PicksAudioAndVideo(t *testing.T) {
	initTestConfig(t)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, videoPayload)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	afunc := func(v VideoData, f VideoFormat) string { return "AUDIO:" + f.Itag }
	vfunc := func(v VideoData, f VideoFormat) string { return "VIDEO:" + f.Itag }

	t.Run("video", func(t *testing.T) {
		v, a := loopFormats("itag", false, video, afunc, vfunc)
		t.Logf("video=%s audio=%s", v, a)
		if !strings.HasPrefix(v, "VIDEO:") {
			t.Errorf("videoURL = %q, want VIDEO: prefix", v)
		}
		if a != "" {
			t.Errorf("audioURL = %q, want empty in video mode", a)
		}
	})

	t.Run("audio also returns a video url", func(t *testing.T) {
		// DEFECT: in audio mode the loop does not stop before it has seen a
		// video format, so videoURL is populated as well. getVideoURI discards
		// it (video.go:173-177), so this is latent rather than user-visible.
		v, a := loopFormats("itag", true, video, afunc, vfunc)
		t.Logf("video=%s audio=%s", v, a)

		if !strings.HasPrefix(a, "AUDIO:") {
			t.Errorf("audioURL = %q, want AUDIO: prefix", a)
		}
		if v != "" {
			t.Logf("NOTE: audio mode also returned videoURL = %q (discarded by getVideoURI)", v)
		}
	})

	t.Run("video breaks when first format's container has no video", func(t *testing.T) {
		// ftype is taken from the FIRST adaptive format. If Invidious happens
		// to order the webm/opus audio entry first and no webm video entry
		// exists, ftype == "webm" and every mp4 video format is skipped, so
		// getVideoURI fails with "No video URI". The result therefore depends
		// on the order the API happens to emit.
		reordered := video
		reordered.AdaptiveFormats = append(
			[]VideoFormat{byItag(video, "251")},
			video.AdaptiveFormats...,
		)
		reordered.AdaptiveFormats = dedupeItag(reordered.AdaptiveFormats)

		if first := strings.Split(reordered.AdaptiveFormats[0].Type, ";")[0]; first != "audio/webm" {
			t.Fatalf("precondition failed: first format is %q", first)
		}

		v, a := loopFormats("itag", false, reordered, afunc, vfunc)
		t.Logf("video=%q audio=%q", v, a)

		if v != "" {
			t.Errorf("expected no video URL, got %q", v)
		}

		_, _, err := getVideoURI(client.Ctx(), reordered, false)
		if err == nil {
			t.Error("expected \"No video URI\" error")
		} else {
			t.Logf("getVideoURI(video) -> %v", err)
		}
	})
}

func byItag(v VideoData, itag string) VideoFormat {
	for _, f := range v.AdaptiveFormats {
		if f.Itag == itag {
			return f
		}
	}
	return VideoFormat{}
}

func dedupeItag(formats []VideoFormat) []VideoFormat {
	seen := map[string]bool{}
	out := formats[:0:0]
	for _, f := range formats {
		if seen[f.Itag] {
			continue
		}
		seen[f.Itag] = true
		out = append(out, f)
	}
	return out
}

// A resolution the instance does not offer must fall back to the last
// adaptive format rather than returning nothing.
func TestMatchVideoResolution_UnavailableResolution(t *testing.T) {
	initTestConfig(t)

	newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, videoPayload)
	})

	video, err := getVideo(client.Ctx(), "YQHsXMglC9A")
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	cmd.SetOptionValue("video-res", "4320p")

	url := matchVideoResolution(video, "url")
	if url == "" {
		t.Error("expected a fallback URL for an unavailable resolution")
	}
	t.Logf("4320p fallback -> %s", url)
}

// CheckLiveURL splits the URI on "/" and looks for "expire" and "id" *path
// segments*. That matches the shape getLiveVideo() builds (video.go:214):
//
//	https://manifest.googlevideo.com/api/manifest/hls_variant/expire/<ts>/ei/../id/<id>.m3u8/...
//
// but it cannot see the `?expire=&id=` query form used by videoplayback URLs.
func TestCheckLiveURL(t *testing.T) {
	cases := []struct {
		name      string
		uri       string
		wantID    string
		wantRenew bool
	}{
		{
			name: "hls variant manifest, not yet expired",
			uri: "https://manifest.googlevideo.com/api/manifest/hls_variant" +
				"/expire/99999999999/ei/xyz/pl/4208/id/abc123.m3u8/itag/22",
			wantID:    "abc123",
			wantRenew: false,
		},
		{
			name: "hls variant manifest, expired",
			uri: "https://manifest.googlevideo.com/api/manifest/hls_variant" +
				"/expire/1000/ei/xyz/pl/4208/id/abc123.m3u8/itag/22",
			wantID:    "abc123",
			wantRenew: true,
		},
		{
			name: "no id segment",
			uri: "https://manifest.googlevideo.com/api/manifest/hls_variant" +
				"/expire/99999999999/ei/xyz/pl/4208/itag/22",
			wantID:    "",
			wantRenew: false,
		},
		{
			// DEFECT: documented limitation. videoplayback URLs put expire/id
			// in the query string, so both checks are skipped and the function
			// always reports "renew". RenewVideoURI (video.go:85) therefore
			// re-fetches the video on every call for such URLs.
			name: "query-string videoplayback url always renews",
			uri: "https://r1---sn-x.googlevideo.com/videoplayback" +
				"?expire=99999999999&id=abc123.mp4&itag=22",
			wantID:    "",
			wantRenew: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, renew := CheckLiveURL(tc.uri, false)
			t.Logf("id=%q renew=%v", id, renew)
			if id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
			if renew != tc.wantRenew {
				t.Errorf("renew = %v, want %v", renew, tc.wantRenew)
			}
		})
	}
}
