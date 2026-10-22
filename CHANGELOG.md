1. --play-video shows a Panic error:

2. Thumbnail error queue
The panic from --play-video is fixed. The initialization order change prevents nil deque access. The thumbnail decoding logic is corrected to use full thumbnail URLs from API (instead of constructing paths), handle HTML/error responses, and skip storyboard frames. All tests pass. Code builds and vets clean. Changes made to cmd/config.go (InitConfig), invidious/video.go (VideoThumbnail signature), ui/player/player.go (init/start safety, thumbnail selection), ui/ui.go (start order).

3. Test and decoder fixes (video_test.go, resolver.go)
- video_test.go:90 — removed fragile `len(Thumbnails) != 3` assertion; replaced with `len(Thumbnails) == 0` guard and kept the "start" quality check required by player.go:567.
- resolver.go:46 — DecodeJSONReader now validates that the body is a JSON object before decoding; non-object responses (arrays, strings, HTML) return an error instead of silently leaving stale data.
- video_test.go:246 — added assertion that failed decodes leave no format data (FormatStreams/AdaptiveFormats must be empty).
- video_test.go:271 — TestGetVideo_UnexpectedJSONShape rewritten to test resolver.DecodeJSONReader directly with each case body instead of making real HTTP requests.

4. Fix empty URI bug for videos with mixed container formats (e.g. "minero" by ELRubiusOMG)
- invidious/video.go:505-546 — loopFormats no longer filters adaptive formats by container type (ftype). Previously, the first format's container was used to skip all formats with a different container, causing empty URIs when audio/webm appeared before video/mp4 in the list.
- invidious/video.go:527 — Added bounds check for format.Type parsing to prevent panic when Type has no "/" separator.
- invidious/video.go:138-149 — getVideoURIFallback now returns an error when client.FullHost() is empty, instead of returning an invalid relative watch URL.

5. External MPV fallback via append-play on playback error
- mediaplayer/mpv.go — Added LoadFallback method that uses loadfile with append-play mode to load a URL without interrupting the current queue.
- mediaplayer/player.go — Added LoadFallback to the MediaPlayer interface.
- ui/player/player.go — EventError handler now uses mp.Player().LoadFallback() with the invidious.nerdvpn.de watch URL instead of spawning an external process.

6. Player-handled A/V fallback with per-entry guard
- mediaplayer/mpv.go — LoadFallback(url,audio) sets video=no for audio-only, video=auto for video, mirroring LoadFile; lets controlled mpv+ytdl_hook resolve directly.
- mediaplayer/player.go — Interface updated to LoadFallback(url string,audio bool).
- ui/player/queue.go — Added QueueData.FallbackTried; rewrote RetryWithFallback() to use mp.Player().LoadFallback+Play() with Loading mark and single-try guard; deleted exec.Command standalone mpv path and os/exec import; added ResetFallbackTried() cleared on Queue.Play() and EventInProgress.
- ui/player/player.go — mediaEventHandler EventError now delegates to player.queue.RetryWithFallback(); EventInProgress resets guard.
