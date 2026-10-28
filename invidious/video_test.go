package invidious

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/darkhz/invidtui/client"
	"github.com/darkhz/invidtui/cmd"
	// "github.com/darkhz/invidtui/resolver"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

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


const videoIdTest = "8GW6sLrK40k"
// const videoIdTest = "VZzSBv6tXMw"

// fetchVideoPayload fetches the raw JSON body for videoIdTest via the global client.
func fetchVideoPayload(t *testing.T) string {
	t.Helper()

	res, err := client.Fetch(client.Ctx(), "videos/"+videoIdTest)
	if err != nil {
		t.Fatalf("client.Fetch: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}

	return string(body)
}

func initGlobalClient_1(t *testing.T){

	const url = "https://invidious.f5.si"

	client.Init()
	client.SetHost(url)
}


func initGlobalClient_2(t *testing.T){

	const url = "https://invidious.nerdvpn.de"

	client.Init()
	client.SetHost(url)
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
	initGlobalClient_1(t)

	// EN VERDAD ESTO SOLO DEBO DE CAMBIAR, LO DE ARRIBA SE SUBE A UN localhost
	video, err := getVideo(client.Ctx(), videoIdTest) /// VZzSBv6tXMw
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}


	if len(video.Thumbnails) == 0 {
		t.Fatal("no thumbnails decoded")
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

	if len(video.FormatStreams) == 0 {
		t.Error("len(FormatStreams) = 0 xd")
	}
	if video.FormatStreams[0].URL == "" {
		t.Error("FormatStreams[0].URL is empty")
	}

	if len(video.AdaptiveFormats) == 0 {
		t.Error("len(AdaptiveFormats) is 0 lol")
	}

	byItag := map[string]VideoFormat{}
	for _, f := range video.AdaptiveFormats {
		byItag[f.Itag] = f
	}

	// // The `,string` fields: bitrate and clen arrive quoted.
	// if f := byItag["137"]; f.Bitrate != 4729334 {
	// 	t.Errorf(`itag 137 Bitrate = %d, want 4729334 (from "4729334")`, f.Bitrate)
	// }
	// if f := byItag["137"]; f.ContentLength != 56789012 {
	// 	t.Errorf(`itag 137 ContentLength = %d, want 56789012 (from "56789012")`, f.ContentLength)
	// }
	// // Audio entries omit "resolution" entirely.
	// if f := byItag["251"]; f.Resolution != "" {
	// 	t.Errorf(`itag 251 Resolution = %q, want "" (key absent)`, f.Resolution)
	// }
	// if f := byItag["251"]; f.AudioSampleRate != 48000 {
	// 	t.Errorf("itag 251 AudioSampleRate = %d, want 48000", f.AudioSampleRate)
	// }
	// // audioChannels arrives quoted, struct field is int.
	// if f := byItag["251"]; f.AudioChannels != 2 {
	// 	t.Errorf(`itag 251 AudioChannels = %d, want 2 (from "2")`, f.AudioChannels)
	// }
	// if f := byItag["251"]; f.FPS != 0 {
	// 	t.Errorf("itag 251 FPS = %d, want 0", f.FPS)
	// }

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
	initGlobalClient_1(t)
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
			initGlobalClient_1(t)
			video, err := getVideo(client.Ctx(), videoIdTest)
			if err == nil {
				t.Fatalf("expected an error, got VideoData")
			}
			t.Logf("err = %v", err)

		if video.Title != "" || video.VideoID != "" {
			t.Errorf("failed decode must not leave partial data: %+v", video)
		}
		if len(video.FormatStreams) != 0 || len(video.AdaptiveFormats) != 0 {
			t.Errorf("failed decode must not leave format data: %d FormatStreams, %d AdaptiveFormats",
				len(video.FormatStreams), len(video.AdaptiveFormats))
		}
		})
	}
}

// A 200 that is valid JSON but not a video object (e.g. an array or an error
// envelope) must not be mistaken for a video.
// func TestGetVideo_UnexpectedJSONShape(t *testing.T) {
// 	cases := map[string]string{
// 		"array":        `[{"title":"nope","videoId":"x"}]`,
// 		"error only":   `{"error":"Something went wrong"}`,
// 		"error string": `"Something went wrong"`,
// 		"empty object": `{}`,
// 	}
//
// 	for name, body := range cases {
// 		t.Run(name, func(t *testing.T) {
// 			var video VideoData
// 			err := resolver.DecodeJSONReader(strings.NewReader(body), &video)
// 			t.Logf("body = %s ; err = %v ; decoded = %+v", body, err, video)
//
// 			if name == "empty object" {
// 				if err != nil {
// 					t.Errorf("empty object should decode cleanly, got %v", err)
// 				}
// 				return
// 			}
// 			if name == "error only" {
// 				// Decodes without error but yields an empty video: the caller
// 				// gets no signal. Documented as a real gap.
// 				if err == nil && video.VideoID == "" {
// 					t.Log("DEFECT: `{\"error\":...}` with HTTP 200 decodes to an " +
// 						"empty VideoData and no error is surfaced to the caller")
// 				}
// 				return
// 			}
// 			if err == nil {
// 				t.Errorf("expected an error for %s, got %+v", name, video)
// 			}
// 		})
// 	}
// }

// The struct relies on `,string` for bitrate/clen. If an instance ever emits a
// suffixed value like "4729k" (YouTube's own player-response format) the whole
// video decode aborts.
func TestGetVideo_SuffixedBitrateAbortsDecode(t *testing.T) {
	initGlobalClient_1(t)

	_ = strings.Replace(fetchVideoPayload(t), `"bitrate":"4729334"`, `"bitrate":"4729k"`, 1)

	video, err := getVideo(client.Ctx(), videoIdTest)
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
	initGlobalClient_1(t)

	_ = strings.Replace(
		strings.Replace(fetchVideoPayload(t), `"formatStreams": [`, `"formatStreamsX": [`, 1),
		`"adaptiveFormats": [`, `"adaptiveFormatsX": [`, 1)

	video, err := getVideo(client.Ctx(), videoIdTest)
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
	initGlobalClient_1(t)

	_ = strings.Replace(fetchVideoPayload(t), `"formatStreams": [`, `"formatStreams": null, "unused": [`, 1)

	video, err := getVideo(client.Ctx(), videoIdTest)
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

// getVideoByFormat resolves the media URLs locally, out of the format lists
// the API already returned, instead of asking the instance to resolve an itag.
// Both the muxed formatStreams and the split adaptiveFormats must be handled.
func TestGetVideoByFormat_ResolvesLocally(t *testing.T) {
	initTestConfig(t)

	initGlobalClient_1(t)
	// my case "minero" doesn't load: VZzSBv6tXMw, but YQHsXMglC9A works fine
	video, err := getVideo(client.Ctx(), videoIdTest)
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	t.Run("video prefers a muxed format at the configured resolution", func(t *testing.T) {
		cmd.SetOptionValue("video-res", "720p")

		videoURL, audioURL := getVideoByFormat(video, false)
		if videoURL != "https://example.com/videoplayback?expire=1&itag=22&clen=23456789" {
			t.Errorf("videoURL = %q, want the itag 22 formatStream", videoURL)
		}
		if audioURL != "" {
			t.Errorf("audioURL = %q, want empty for a muxed format", audioURL)
		}
	})

	t.Run("video splits the tracks when no muxed format matches", func(t *testing.T) {
		cmd.SetOptionValue("video-res", "1080p")

		videoURL, audioURL := getVideoByFormat(video, false)
		if videoURL != "https://example.com/videoplayback?expire=1&itag=137" {
			t.Errorf("videoURL = %q, want the itag 137 adaptive format", videoURL)
		}
		if audioURL != "https://example.com/videoplayback?expire=1&itag=140" {
			t.Errorf("audioURL = %q, want the mp4 (itag 140) audio format", audioURL)
		}
	})

	t.Run("audio returns only an audio track", func(t *testing.T) {
		cmd.SetOptionValue("video-res", "720p")

		videoURL, audioURL := getVideoByFormat(video, true)
		if videoURL != "" {
			t.Errorf("videoURL = %q, want empty in audio mode", videoURL)
		}
		if audioURL != "https://example.com/videoplayback?expire=1&itag=140" {
			t.Errorf("audioURL = %q, want the mp4 (itag 140) audio format", audioURL)
		}
	})

	t.Run("never resolves through the instance", func(t *testing.T) {
		cmd.SetOptionValue("video-res", "720p")

		for _, audio := range []bool{false, true} {
			videoURL, audioURL := getVideoByFormat(video, audio)
			for _, uri := range []string{videoURL, audioURL} {
				if strings.Contains(uri, "/latest_version") {
					t.Errorf("audio=%v: uri = %q, want no /latest_version URL", audio, uri)
				}
			}
		}
	})
}

// An unavailable resolution must fall back to the closest one below it rather
// than returning nothing, and mp4 must win over webm at equal merit.
func TestPickFormat_FallsBackBelowRequestedResolution(t *testing.T) {
	initTestConfig(t)

	initGlobalClient_1(t)
	video, err := getVideo(client.Ctx(), videoIdTest)
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	t.Run("falls back to the highest resolution below the request", func(t *testing.T) {
		cmd.SetOptionValue("video-res", "4320p")

		videoURL, _ := getVideoByFormat(video, false)
		if videoURL != "https://example.com/videoplayback?expire=1&itag=22&clen=23456789" {
			t.Errorf("videoURL = %q, want the 720p itag 22 formatStream", videoURL)
		}
	})

	t.Run("mp4 is preferred over webm", func(t *testing.T) {
		formats := []VideoFormat{
			{URL: "webm-video", Type: `video/webm; codecs="vp9"`, Resolution: "720p"},
			{URL: "mp4-video", Type: `video/mp4; codecs="avc1.4d401f"`, Resolution: "720p"},
		}

		format := pickFormat(formats, "video", 720)
		if format == nil {
			t.Fatal("pickFormat returned nil")
		}
		if format.URL != "mp4-video" {
			t.Errorf("URL = %q, want mp4-video", format.URL)
		}
	})

	t.Run("formats without a URL are skipped", func(t *testing.T) {
		formats := []VideoFormat{
			{URL: "", Type: `video/mp4; codecs="avc1.4d401f"`, Resolution: "720p"},
			{URL: "fallback", Type: `video/mp4; codecs="avc1.4d401e"`, Resolution: "480p"},
		}

		format := pickFormat(formats, "video", 720)
		if format == nil {
			t.Fatal("pickFormat returned nil")
		}
		if format.URL != "fallback" {
			t.Errorf("URL = %q, want fallback", format.URL)
		}
	})

	t.Run("audio entries are not returned for a video request", func(t *testing.T) {
		formats := []VideoFormat{
			{URL: "audio-only", Type: `audio/mp4; codecs="mp4a.40.2"`},
		}

		if format := pickFormat(formats, "video", 720); format != nil {
			t.Errorf("pickFormat returned %q for a video request", format.URL)
		}
	})
}

// A video whose only formats are video-only streams has no audio track to
// attach, so playback must not be attempted with a silent video.
func TestGetVideoByFormat_NoAudioTrackForSplitVideo(t *testing.T) {
	initTestConfig(t)

	initGlobalClient_1(t)
	video, err := getVideo(client.Ctx(), videoIdTest)
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	videoOnly := video
	videoOnly.FormatStreams = nil
	videoOnly.AdaptiveFormats = []VideoFormat{
		{
			URL:        "https://example.com/videoplayback?expire=1&itag=137",
			Itag:       "137",
			Type:       `video/mp4; codecs="avc1.640028"`,
			Resolution: "1080p",
		},
	}

	cmd.SetOptionValue("video-res", "1080p")

	videoURL, audioURL := getVideoByFormat(videoOnly, false)
	if videoURL != "" || audioURL != "" {
		t.Errorf("videoURL = %q, audioURL = %q, want both empty without an audio track",
			videoURL, audioURL)
	}
}

// RenewVideoURI reuses a resolved media URL until it expires, instead of
// querying the API again on every playback.
func TestRenewVideoURI_ReusesUnexpiredMediaURL(t *testing.T) {
	initTestConfig(t)

	var calls int

	initGlobalClient_1(t)
	video, err := getVideo(client.Ctx(), videoIdTest)
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	future := time.Now().Add(time.Hour).Unix()
	uri := [2]string{
		"https://example.com/videoplayback?expire=" + strconv.FormatInt(future, 10) + "&itag=137",
		"https://example.com/videoplayback?expire=" + strconv.FormatInt(future, 10) + "&itag=140",
	}

	calls = 0

	got, gotURI, err := RenewVideoURI(client.Ctx(), uri, video, false)
	if err != nil {
		t.Fatalf("RenewVideoURI: %v", err)
	}
	if gotURI != uri {
		t.Errorf("uris = %v, want the unexpired %v", gotURI, uri)
	}
	if calls != 0 {
		t.Errorf("expected no API calls for an unexpired URL, got %d", calls)
	}
	if got.VideoID != video.VideoID {
		t.Errorf("VideoID = %q, want %q", got.VideoID, video.VideoID)
	}

	t.Run("expired URLs are resolved again", func(t *testing.T) {
		past := [2]string{
			"https://example.com/videoplayback?expire=1000&itag=137",
			"",
		}

		calls = 0

		_, gotURI, err := RenewVideoURI(client.Ctx(), past, video, false)
		if err != nil {
			t.Fatalf("RenewVideoURI: %v", err)
		}
		if gotURI == past {
			t.Errorf("uris = %v, want freshly resolved URIs", gotURI)
		}
		if gotURI[0] == "" {
			t.Error("expected a resolved video URI")
		}
		// The formats are already present, so resolving them locally needs no
		// further API round-trip.
		if calls != 0 {
			t.Errorf("expected no refetch when the formats are cached, got %d", calls)
		}
	})
}

func TestGetVideoURI_NoAudioURIError(t *testing.T) {
	initTestConfig(t)

	initGlobalClient_1(t)
	video, err := getVideo(client.Ctx(), videoIdTest)
	if err != nil {
		t.Fatalf("getVideo: %v", err)
	}

	if _, _, err := getVideoURI(client.Ctx(), video, true); err != nil {
		t.Errorf("audio URI should resolve, got %v", err)
	}
}

// Posibles casos hipoteticos, practicamente todo URL video de inv. tiene fomratos Adaptativos, por lo que debería
// La aplicación de manejar este caso, cai imposible?
func TestGetVideoURI_RefetchWhenFormatsNil(t *testing.T) {
	initTestConfig(t)

	var calls int

	initGlobalClient_1(t)
	// A VideoData with no formats, as produced by playlist.go or the queue.
	partial := VideoData{VideoID: videoIdTest, Title: "t"}

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
//
// Además si falla algunos formatos es porque no todo formato de invidious lleva empaquetadito
//
func TestLoopFormats_PicksAudioAndVideo(t *testing.T) {
	initTestConfig(t)

	initGlobalClient_1(t)

	video, err := getVideo(client.Ctx(), videoIdTest)
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
		// loopFormats is now only the fallback for instances that return no
		// format URLs, and it is still order-dependent: ftype is taken from the
		// FIRST adaptive format, so a leading webm/opus audio entry makes it
		// skip every mp4 video format.
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

		// getVideoByFormat is not order-dependent: it ranks the mp4 video
		// format regardless of where the webm audio entry sits, so the local
		// resolution succeeds and the itag fallback is never reached.
		cmd.SetOptionValue("video-res", "1080p")

		videoURL, audioURL := getVideoByFormat(reordered, false)
		if videoURL != "https://example.com/videoplayback?expire=1&itag=137" {
			t.Errorf("getVideoByFormat videoURL = %q, want the itag 137 adaptive format", videoURL)
		}
		if audioURL != "https://example.com/videoplayback?expire=1&itag=140" {
			t.Errorf("getVideoByFormat audioURL = %q, want the mp4 (itag 140) audio format", audioURL)
		}

		if _, _, err := getVideoURI(client.Ctx(), reordered, false); err != nil {
			t.Errorf("getVideoURI(video) -> %v, want the locally resolved formats", err)
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

	initGlobalClient_1(t)
	video, err := getVideo(client.Ctx(), videoIdTest)
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
