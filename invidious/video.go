package invidious

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/darkhz/invidtui/client"
	"github.com/darkhz/invidtui/cmd"
	"github.com/darkhz/invidtui/resolver"
	"github.com/darkhz/invidtui/utils"
	"github.com/etherlabsio/go-m3u8/m3u8"
)

// VideoData stores information about a video.
type VideoData struct {
	Title             string            `json:"title"`
	Author            string            `json:"author"`
	AuthorID          string            `json:"authorId"`
	VideoID           string            `json:"videoId"`
	HlsURL            string            `json:"hlsUrl"`
	LengthSeconds     int64             `json:"lengthSeconds"`
	LiveNow           bool              `json:"liveNow"`
	ViewCount         int               `json:"viewCount"`
	LikeCount         int               `json:"likeCount"`
	PublishedText     string            `json:"publishedText"`
	SubCountText      string            `json:"subCountText"`
	Description       string            `json:"description"`
	Thumbnails        []VideoThumbnails `json:"videoThumbnails"`
	FormatStreams     []VideoFormat     `json:"formatStreams"`
	AdaptiveFormats   []VideoFormat     `json:"adaptiveFormats"`
	RecommendedVideos []VideoData       `json:"recommendedVideos"`

	MediaType string
	Timestamp *int64
}

// VideoFormat stores information about the video's format.
type VideoFormat struct {
	Type            string `json:"type"`
	URL             string `json:"url"`
	Itag            string `json:"itag"`
	Container       string `json:"container"`
	Encoding        string `json:"encoding"`
	Resolution      string `json:"resolution,omitempty"`
	Bitrate         int64  `json:"bitrate,string"`
	ContentLength   int64  `json:"clen,string"`
	FPS             int    `json:"fps"`
	AudioSampleRate int    `json:"audioSampleRate"`
	AudioChannels   int    `json:"audioChannels"`
}

// VideoThumbnails stores the video's thumbnails.
type VideoThumbnails struct {
	Quality string `json:"quality"`
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

// Video retrieves a video.
func Video(id string, ctx ...context.Context) (VideoData, error) {
	if ctx == nil {
		ctx = append(ctx, client.Ctx())
	}

	return getVideo(ctx[0], id)
}

// VideoThumbnail returns the response of a video thumbnail request.
// The provided uri is the thumbnail's URL as returned by the API. Only its
// path and query are used, as the request is served by the instance itself.
func VideoThumbnail(ctx context.Context, uri string) (*http.Response, error) {
	url, err := utils.IsValidURL(uri)
	if err != nil {
		return nil, err
	}

	res, err := client.Get(ctx, url.RequestURI())
	if err != nil {
		return nil, err
	}

	return res, nil
}

// RenewVideoURI renews the video's media URIs.
func RenewVideoURI(ctx context.Context, uri [2]string, video VideoData, audio bool) (VideoData, [2]string, error) {
	if uri[0] != "" {
		switch {
		case video.LiveNow:
			if _, renew := CheckLiveURL(uri[0], audio); !renew {
				return video, uri, nil
			}

		case !mediaURLExpired(uri[0]):
			// A resolved media URL stays valid until it expires, so there
			// is no need to query the API again for it.
			return video, uri, nil
		}
	}

	v, uris, err := getVideoURI(ctx, video, audio)
	if err != nil {
		return VideoData{}, uri, err
	}

	return v, uris, nil
}

// mediaURLExpired reports whether a resolved media URL has expired.
// videoplayback URLs carry an "expire" unix timestamp in their query string;
// URLs without one are treated as valid.
func mediaURLExpired(uri string) bool {
	parsed, err := url.Parse(uri)
	if err != nil {
		return true
	}

	expire := parsed.Query().Get("expire")
	if expire == "" {
		return false
	}

	timestamp, err := strconv.ParseInt(expire, 10, 64)
	if err != nil {
		return true
	}

	return time.Now().Unix() >= timestamp
}

// CheckLiveURL returns whether the provided live video's URL has expired or not.
func CheckLiveURL(uri string, audio bool) (string, bool) {
	var id string

	renew := true

	// Split the uri parameters.
	uriSplit := strings.Split(uri, "/")
	for i, v := range uriSplit {
		if v == "expire" {
			// Return if the uri is not expired.
			exptime, err := strconv.ParseInt(uriSplit[i+1], 10, 64)
			if err == nil && time.Now().Unix() < exptime {
				renew = false
				continue
			}
		}

		if v == "id" {
			// Get the id value from the uri path.
			id = strings.Split(uriSplit[i+1], ".")[0]
			break
		}
	}

	return id, renew
}

// getVideo queries for and returns a video according to the provided video ID.
func getVideo(ctx context.Context, id string) (VideoData, error) {
	var data VideoData

	res, err := client.Fetch(ctx, "videos/"+id)
	if err != nil {
		return VideoData{}, err
	}
	defer res.Body.Close()

	err = resolver.DecodeJSONReader(res.Body, &data)
	if err != nil {
		return VideoData{}, err
	}

	return data, nil
}

// getVideoURI returns the video's media URIs.
func getVideoURI(ctx context.Context, video VideoData, audio bool) (VideoData, [2]string, error) {
	var uris [2]string
	var audioURL, videoURL string

	if video.FormatStreams == nil || video.AdaptiveFormats == nil {
		v, err := getVideo(ctx, video.VideoID)
		if err != nil {
			return VideoData{}, uris, err
		}

		video.AdaptiveFormats = v.AdaptiveFormats
		video.FormatStreams = v.FormatStreams
	}

	if video.LiveNow {
		audio = false
		videoURL, audioURL = getLiveVideo(ctx, video.VideoID, audio)
	} else {
		videoURL, audioURL = getVideoByFormat(video, audio)

		// Instances that have lost access to YouTube omit the format URLs
		// altogether, so the format has to be resolved by the instance itself.
		if (audio && audioURL == "") || (!audio && videoURL == "") {
			videoURL, audioURL = getVideoByItag(video, audio)
		}
	}

	if audio && audioURL == "" {
		return VideoData{}, uris, fmt.Errorf("No audio URI")
	} else if !audio && videoURL == "" {
		return VideoData{}, uris, fmt.Errorf("No video URI")
	}

	if audio {
		uris[0] = audioURL
	} else {
		uris[0], uris[1] = videoURL, audioURL
	}

	return video, uris, nil
}

// getLiveVideo gets the hls playlist, parses and finds the appropriate live video stream.
func getLiveVideo(ctx context.Context, id string, audio bool) (string, string) {
	var videoURL, audioURL string

	video, err := getVideo(ctx, id)
	if err != nil || video.HlsURL == "" {
		return "", ""
	}

	url, _ := utils.IsValidURL(video.HlsURL)
	res, err := client.Get(ctx, url.RequestURI())
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()

	pl, err := m3u8.Read(res.Body)
	if err != nil {
		return "", ""
	}

	for _, p := range pl.Playlists() {
		resolution := cmd.GetOptionValue("video-res")
		height := strconv.Itoa(p.Resolution.Height) + "p"

		// Since the retrieved HLS playlist is sorted in ascending order of resolutions,
		// for the audio stream, we grab the first stream (with the lowest quality),
		// and instruct mpv not to play video for the audio stream. For the video stream,
		// we grab the stream where the playlist entry's resolution and the required
		// resolution are equal.
		if audio || (!audio && height == resolution) {
			url, _ := utils.IsValidURL(p.URI)
			videoURL = "https://manifest.googlevideo.com" + url.RequestURI()

			break
		}
	}

	return videoURL, audioURL
}

// matchVideoResolution returns a URL that is associated with the video's format.
func matchVideoResolution(video VideoData, urlType string) string {
	var uri string

	resolution := cmd.GetOptionValue("video-res")

	for _, format := range video.AdaptiveFormats {
		if len(format.Resolution) <= 0 {
			continue
		}

		switch urlType {
		case "url":
			if format.Resolution == resolution {
				return format.URL
			}

			uri = format.URL

		case "itag":
			if format.Resolution == resolution {
				return getLatestURL(video.VideoID, format.Itag)
			}

			uri = getLatestURL(video.VideoID, format.Itag)
		}
	}

	return uri
}

// formatKind splits a format's MIME type into its kind ("video"/"audio") and
// container ("mp4"/"webm"). Invidious emits types such as
// `video/mp4; codecs="avc1.640028"`, and audio entries omit the codec part
// entirely in some responses.
func formatKind(format VideoFormat) (kind, container string) {
	main := strings.TrimSpace(strings.Split(format.Type, ";")[0])

	parts := strings.SplitN(main, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

// resolutionHeight returns the vertical resolution of the format in pixels,
// or 0 when the format does not carry one (audio entries, for instance).
func resolutionHeight(format VideoFormat) int {
	res := strings.TrimSuffix(format.Resolution, "p")
	if res == "" || res == format.Resolution {
		// Not a "<n>p" value, e.g. "audio_only" or an empty resolution.
		return 0
	}

	height, err := strconv.Atoi(res)
	if err != nil {
		return 0
	}

	return height
}

// formatScore ranks a format against the requested resolution and container.
// A lower score is better, and the fields are compared in order.
type formatScore struct {
	class     int
	container int
	distance  int
}

// rankFormat scores a single format. Formats are grouped so that an exact
// resolution match always wins, then anything below the request (closest
// first), and only then anything above it. mp4 is preferred over webm within
// the same group, as it plays back most reliably.
func rankFormat(format VideoFormat, wantHeight int) formatScore {
	_, container := formatKind(format)

	score := formatScore{}
	if container == "mp4" {
		score.container = 1
	}

	height := resolutionHeight(format)
	if wantHeight <= 0 || height <= 0 {
		// Either no resolution was requested or this format carries none,
		// so only the container matters.
		return score
	}

	switch {
	case height == wantHeight:
		score.class = 0
	case height < wantHeight:
		score.class = 1
		score.distance = wantHeight - height
	default:
		score.class = 2
		score.distance = height - wantHeight
	}

	return score
}

// betterFormat reports whether a scores strictly better than b.
func betterFormat(a, b formatScore) bool {
	if a.class != b.class {
		return a.class < b.class
	}
	if a.container != b.container {
		return a.container > b.container
	}

	return a.distance < b.distance
}

// pickFormat returns the format that best satisfies the requested kind,
// preferring mp4 over webm and, for videos, the configured resolution with a
// graceful fallback to whatever the instance does offer.
func pickFormat(formats []VideoFormat, kind string, wantHeight int) *VideoFormat {
	var (
		best  *VideoFormat
		score formatScore
		found bool
	)

	for i, format := range formats {
		if format.URL == "" {
			continue
		}

		if fkind, _ := formatKind(format); fkind != kind {
			continue
		}

		current := rankFormat(format, wantHeight)
		if !found || betterFormat(current, score) {
			best, score, found = &formats[i], current, true
		}
	}

	return best
}

// getVideoByFormat resolves the media URLs from the formats the API already
// returned, rather than asking the instance to resolve an itag. Both the
// muxed FormatStreams and the split AdaptiveFormats are considered.
func getVideoByFormat(video VideoData, audio bool) (string, string) {
	wantHeight := 0
	if res := cmd.GetOptionValue("video-res"); res != "" {
		if height, err := strconv.Atoi(strings.TrimSuffix(res, "p")); err == nil {
			wantHeight = height
		}
	}

	audioFormat := pickFormat(video.AdaptiveFormats, "audio", 0)

	if audio {
		if audioFormat == nil {
			return "", ""
		}

		return "", audioFormat.URL
	}

	// A muxed format carries audio and video in a single stream, so mpv needs
	// no external audio track. It is preferred, but only when it resolves to
	// at least the same resolution as the split formats would.
	muxed := pickFormat(video.FormatStreams, "video", wantHeight)
	videoFormat := pickFormat(video.AdaptiveFormats, "video", wantHeight)

	switch {
	case muxed == nil && videoFormat == nil:
		return "", ""

	case videoFormat == nil:
		return muxed.URL, ""

	case muxed == nil:
		// Without a separate audio track a video-only stream would be silent.
		if audioFormat == nil {
			return "", ""
		}

		return videoFormat.URL, audioFormat.URL
	}

	// Only trade the single-stream convenience for the split formats when they
	// land in a strictly better resolution group, so that an exact match wins
	// over a muxed format that is merely close.
	if rankFormat(*videoFormat, wantHeight).class < rankFormat(*muxed, wantHeight).class &&
		audioFormat != nil {
		return videoFormat.URL, audioFormat.URL
	}

	return muxed.URL, ""
}

// getVideoByItag gets the appropriate itag of the video format, and
// returns a video and audio url using getLatestURL().
func getVideoByItag(video VideoData, audio bool) (string, string) {
	var videoURL, audioURL string

	videoURL, audioURL = loopFormats(
		"itag", audio, video,
		func(v VideoData, f VideoFormat) string {
			video := getLatestURL(v.VideoID, f.Itag)

			return video
		},
		func(v VideoData, f VideoFormat) string {
			return matchVideoResolution(v, "itag")
		},
	)

	return videoURL, audioURL
}

// loopFormats loops over a video's AdaptiveFormats data and gets the
// audio/video URL according to the values returned by afunc/vfunc.
func loopFormats(
	urlType string, audio bool, video VideoData,
	afunc, vfunc func(video VideoData, format VideoFormat) string,
) (string, string) {
	var ftype, videoURL, audioURL string

	// For videos, we loop through FormatStreams first and get the videoURL.
	// This works mainly for 720p, 360p and 144p video streams.
	if !audio && urlType != "itag" {
		for _, format := range video.FormatStreams {
			if format.Resolution == cmd.GetOptionValue("video-res") {
				videoURL = format.URL
				return videoURL, audioURL
			}
		}
	}

	// If the required resolution wasn't found in FormatStreams, we loop through
	// AdaptiveFormats and get a video of the required resolution, along with the
	// audio stream so that MPV can merge them and play. Or if only audio is required,
	// return a blank videoURL and a non-empty audioURL.
	for _, format := range video.AdaptiveFormats {
		v := strings.Split(format.Type, ";")
		p := strings.Split(v[0], "/")

		if (audio && audioURL != "") || (!audio && videoURL != "") {
			break
		}

		if ftype == "" {
			ftype = p[1]
		}

		if p[1] == ftype {
			if p[0] == "audio" {
				audioURL = afunc(video, format)
			} else if p[0] == "video" {
				videoURL = vfunc(video, format)
			}
		}
	}

	return videoURL, audioURL
}

// getLatestURL appends the latest_version query to the current client's host URL.
// For example: https://invidious.snopyta.org/latest_version?id=mWDOxRWcoPE&itag=22&local=true
func getLatestURL(id, itag string) string {
	var itagstr string

	host := client.FullHost()

	idstr := "id=" + id

	if itag != "" {
		itagstr += "&itag=" + itag
	}

	return host + "/latest_version?" + idstr + itagstr + "&local=true"
}
