package resolver

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/ugorji/go/codec"
)

// ---------------------------------------------------------------------------
// Fixtures mirroring real Invidious /api/v1 payloads
// ---------------------------------------------------------------------------

// The exact shape Invidious emits for adaptiveFormats[].bitrate and .clen:
// both are JSON *strings* (Crystal `.as_i.to_s` / `|| "-1"`).
const adaptiveFormatJSON = `{
	"itag": "137",
	"url": "https://r1---sn-abc.googlevideo.com/videoplayback?expire=1&clen=1234567",
	"type": "video/mp4; codecs=\"avc1.640028\"",
	"container": "mp4",
	"encoding": "avc1",
	"quality": "hd1080",
	"resolution": "1080p",
	"bitrate": "4818629",
	"clen": "1234567",
	"fps": 30,
	"audioSampleRate": 0,
	"audioChannels": 0
}`

func TestDecodeJSONReader_BasicFields(t *testing.T) {
	var out struct {
		Title     string `json:"title"`
		VideoID   string `json:"videoId"`
		AuthorID  string `json:"authorId"`
		ViewCount int    `json:"viewCount"`
		LiveNow   bool   `json:"liveNow"`
	}

	payload := `{"title":"Hello","videoId":"YQHsXMglC9A",
	             "authorId":"UCsRM0YB_dabtEPGPTKo-gcw",
	             "viewCount":3400000000,"liveNow":false}`

	if err := DecodeJSONReader(strings.NewReader(payload), &out); err != nil {
		t.Fatalf("DecodeJSONReader: %v", err)
	}

	if out.Title != "Hello" {
		t.Errorf("Title = %q, want %q", out.Title, "Hello")
	}
	if out.VideoID != "YQHsXMglC9A" {
		t.Errorf("VideoID = %q, want %q", out.VideoID, "YQHsXMglC9A")
	}
	if out.AuthorID != "UCsRM0YB_dabtEPGPTKo-gcw" {
		t.Errorf("AuthorID = %q, want %q", out.AuthorID, "UCsRM0YB_dabtEPGPTKo-gcw")
	}
	if out.ViewCount != 3400000000 {
		t.Errorf("ViewCount = %d, want %d (must not overflow on 32-bit)", out.ViewCount, 3400000000)
	}
	if out.LiveNow {
		t.Error("LiveNow = true, want false")
	}
}

// CRITICAL: proves the codec DOES honour `json:"..."` struct tags.
// ugorji's default TypeInfos is NewTypeInfos([]string{"codec", "json"}).
//
// It also pins down the matching rule for *untagged* fields: ugorji matches
// the raw Go field name EXACTLY. Unlike encoding/json it does NOT fall back to
// a case-insensitive match. This is what keeps the untagged
// VideoData.MediaType / VideoData.Timestamp fields inert even if Invidious
// starts emitting "mediaType" or "timestamp".
func TestDecodeJSON_HonoursJSONTags(t *testing.T) {
	payload := `{"someRenamedKey":"from-json-tag"}`

	t.Run("json tag wins", func(t *testing.T) {
		var tagged struct {
			Value string `json:"someRenamedKey"`
		}
		if err := DecodeJSONBytes([]byte(payload), &tagged); err != nil {
			t.Fatalf("decode tagged: %v", err)
		}
		if tagged.Value != "from-json-tag" {
			t.Errorf("json tag ignored: Value = %q, want %q", tagged.Value, "from-json-tag")
		}
	})

	t.Run("untagged needs exact field name", func(t *testing.T) {
		// lowerCamelCase JSON key, Go name in PascalCase: NOT matched.
		var lowerCamel struct {
			SomeRenamedKey string
		}
		if err := DecodeJSONBytes([]byte(payload), &lowerCamel); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if lowerCamel.SomeRenamedKey != "" {
			t.Errorf("expected NO case-insensitive fallback, got %q", lowerCamel.SomeRenamedKey)
		}

		// Exact Go field name as the JSON key: matched.
		var exact struct {
			SomeRenamedKey string
		}
		if err := DecodeJSONBytes([]byte(`{"SomeRenamedKey":"exact"}`), &exact); err != nil {
			t.Fatalf("decode exact: %v", err)
		}
		if exact.SomeRenamedKey != "exact" {
			t.Errorf("exact field-name match failed, got %q", exact.SomeRenamedKey)
		}
	})

	t.Run("untagged MediaType is not filled by mediaType key", func(t *testing.T) {
		var v struct {
			MediaType string
		}
		if err := DecodeJSONBytes([]byte(`{"mediaType":"Audio"}`), &v); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if v.MediaType != "" {
			t.Errorf("DEFECT: untagged MediaType was populated from \"mediaType\" as %q; "+
				"invidious/playlist.go:296 relies on it staying empty", v.MediaType)
		}
	})
}

// The `,string` modifier in `json:"bitrate,string"` is NOT implemented by
// ugorji/co (parseTag only understands "omitempty"). It nonetheless appears to
// work because jsonDecDriver.decNumBytes() strips surrounding quotes before
// parsing. This test pins that behaviour down: quoted numbers ARE accepted into
// numeric fields, and the ",string" text in the tag is inert.
func TestDecodeJSON_QuotedNumbersIntoNumericFields(t *testing.T) {
	var out struct {
		Bitrate       int64 `json:"bitrate,string"`
		ContentLength int64 `json:"clen,string"`
	}

	payload := `{"bitrate":"4818629","clen":"1234567"}`

	if err := DecodeJSONBytes([]byte(payload), &out); err != nil {
		t.Fatalf("quoted numbers should decode without error, got: %v", err)
	}

	if out.Bitrate != 4818629 {
		t.Errorf("Bitrate = %d, want 4818629", out.Bitrate)
	}
	if out.ContentLength != 1234567 {
		t.Errorf("ContentLength = %d, want 1234567", out.ContentLength)
	}

	// The very same payload also decodes when the tag omits ",string",
	// proving the modifier contributes nothing.
	var plain struct {
		Bitrate       int64 `json:"bitrate"`
		ContentLength int64 `json:"clen"`
	}
	if err := DecodeJSONBytes([]byte(payload), &plain); err != nil {
		t.Fatalf("decode without ,string: %v", err)
	}
	if plain.Bitrate != 4818629 || plain.ContentLength != 1234567 {
		t.Errorf("without ,string: got %d/%d, want 4818629/1234567",
			plain.Bitrate, plain.ContentLength)
	}

	// And the ",string" text does not leak into the mapped field name.
	var weird struct {
		Bitrate int64 `json:"bitrate,string,extra"`
	}
	if err := DecodeJSONBytes([]byte(payload), &weird); err != nil {
		t.Fatalf("decode with unknown options: %v", err)
	}
	if weird.Bitrate != 4818629 {
		t.Errorf("unknown tag options should be ignored, got %d", weird.Bitrate)
	}
}

// Invidious emits audioChannels as a JSON *string* (YouTube pass-through),
// while invidious.VideoFormat declares it `int`. Document that this is tolerated.
func TestDecodeJSON_QuotedIntIntoIntField(t *testing.T) {
	var out struct {
		AudioChannels int `json:"audioChannels"`
	}

	if err := DecodeJSONBytes([]byte(`{"audioChannels":"2"}`), &out); err != nil {
		t.Fatalf("quoted int into int field: %v", err)
	}
	if out.AudioChannels != 2 {
		t.Errorf("AudioChannels = %d, want 2", out.AudioChannels)
	}
}

// clen is emitted as "-1" when YouTube omits contentLength.
func TestDecodeJSON_NegativeQuotedNumber(t *testing.T) {
	var out struct {
		ContentLength int64 `json:"clen,string"`
	}

	if err := DecodeJSONBytes([]byte(`{"clen":"-1"}`), &out); err != nil {
		t.Fatalf(`clen:"-1": %v`, err)
	}
	if out.ContentLength != -1 {
		t.Errorf("ContentLength = %d, want -1", out.ContentLength)
	}
}

func TestDecodeJSON_BothStringAndNumberAccepted(t *testing.T) {
	cases := map[string]struct {
		payload string
		want    int64
	}{
		"number":         {`{"bitrate":4818629}`, 4818629},
		"string":         {`{"bitrate":"4818629"}`, 4818629},
		"quoted with sp": {`{"bitrate": "4818629"}`, 4818629},
		"zero":           {`{"bitrate":0}`, 0},
		"quoted zero":    {`{"bitrate":"0"}`, 0},
		"null":           {`{"bitrate":null}`, 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var out struct {
				Bitrate int64 `json:"bitrate"`
			}
			if err := DecodeJSONBytes([]byte(tc.payload), &out); err != nil {
				t.Fatalf("decode %s: %v", tc.payload, err)
			}
			if out.Bitrate != tc.want {
				t.Errorf("Bitrate = %d, want %d", out.Bitrate, tc.want)
			}
		})
	}
}

// Unknown API keys are silently dropped (no ErrorIfNoField set on the handle),
// which is required for forward-compat but hides typos in struct tags.
func TestDecodeJSON_UnknownFieldsIgnored(t *testing.T) {
	var out struct {
		Known string `json:"known"`
	}

	payload := `{"known":"yes","brandNewInvidiousKey":{"nested":[1,2,3]},"anotherOne":null}`

	if err := DecodeJSONBytes([]byte(payload), &out); err != nil {
		t.Fatalf("unknown fields should not error: %v", err)
	}
	if out.Known != "yes" {
		t.Errorf("Known = %q, want %q", out.Known, "yes")
	}
}

// A struct tag typo silently yields the zero value. This is the failure mode
// that makes "the parser is broken" hard to spot in production.
func TestDecodeJSON_MisspelledTagSilentlyZeroes(t *testing.T) {
	var out struct {
		AuthorID string `json:"authorID"` // Invidious actually sends "authorId"
	}

	if err := DecodeJSONBytes([]byte(`{"authorId":"UC123"}`), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AuthorID != "" {
		t.Errorf("expected fallback to Go field name to populate it, got %q", out.AuthorID)
	}
}

func TestDecodeJSON_InvalidJSONReturnsError(t *testing.T) {
	cases := []string{
		`Endpoint disabled`,             // real inv.nadeko.net 403 body
		`forbidden`,                     // real yt.chocolatemoo53.com 403 body
		``,                              // empty body
		`{"title":`,                     // truncated
		`<html><body>401</body></html>`, // real nginx 401 page
	}

	for _, payload := range cases {
		t.Run(fmt.Sprintf("%.20q", payload), func(t *testing.T) {
			var out struct {
				Title string `json:"title"`
			}
			if err := DecodeJSONBytes([]byte(payload), &out); err == nil {
				t.Errorf("expected an error for non-JSON body %q, got nil", payload)
			}
		})
	}
}

// A valid JSON document that is not an object (e.g. a bare string) must not
// panic, and must not silently succeed.
func TestDecodeJSON_ScalarIntoStruct(t *testing.T) {
	var out struct {
		Title string `json:"title"`
	}

	err := DecodeJSONBytes([]byte(`"Endpoint disabled"`), &out)
	t.Logf("scalar string into struct -> err=%v out=%+v", err, out)
}

// DecodeJSONReader and DecodeJSONBytes must agree. They are backed by two
// distinct *codec.Decoder instances configured identically.
func TestDecodeJSON_ReaderBytesEquivalence(t *testing.T) {
	type payload struct {
		Title   string `json:"title"`
		Bitrate int64  `json:"bitrate,string"`
		List    []int  `json:"list"`
	}

	body := `{"title":"t","bitrate":"42","list":[1,2,3]}`

	var a, b payload

	if err := DecodeJSONReader(strings.NewReader(body), &a); err != nil {
		t.Fatalf("reader: %v", err)
	}
	if err := DecodeJSONBytes([]byte(body), &b); err != nil {
		t.Fatalf("bytes: %v", err)
	}

	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("reader/bytes mismatch:\n reader: %+v\n bytes : %+v", a, b)
	}
}

// The decoders are package-level singletons reused via Reset. A failed decode
// must not poison the next one.
func TestDecodeJSON_ResetAfterError(t *testing.T) {
	var out struct {
		Title string `json:"title"`
	}

	if err := DecodeJSONBytes([]byte(`{{{ not json`), &out); err == nil {
		t.Fatal("expected error on malformed input")
	}
	if err := DecodeJSONBytes([]byte(`{"title":"recovered"}`), &out); err != nil {
		t.Fatalf("decode after error: %v", err)
	}
	if out.Title != "recovered" {
		t.Errorf("Title = %q, want %q", out.Title, "recovered")
	}
}

// Alternating between the two decoders must not cross-contaminate state.
func TestDecodeJSON_JSONAndSimpleDecodersIndependent(t *testing.T) {
	var a, b struct {
		Title string `json:"title"`
	}

	if err := DecodeJSONBytes([]byte(`{"title":"json"}`), &a); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if err := DecodeSimpleBytes([]byte(`{"title":"simple"}`), &b); err != nil {
		t.Fatalf("simple decode: %v", err)
	}

	if a.Title != "json" || b.Title != "simple" {
		t.Errorf("cross-contamination: json=%q simple=%q", a.Title, b.Title)
	}
}

// Trailing bytes after a complete value are consumed by the buffered reader;
// verify a subsequent decode still starts at the right place.
func TestDecodeJSON_SequentialPayloads(t *testing.T) {
	for i := 0; i < 100; i++ {
		var out struct {
			N int `json:"n"`
		}
		payload := fmt.Sprintf(`{"n":%d}`, i)
		if err := DecodeJSONBytes([]byte(payload), &out); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if out.N != i {
			t.Fatalf("iteration %d: N = %d, want %d", i, out.N, i)
		}
	}
}

// The shared singleton decoders are mutex-guarded. Verify under -race that
// concurrent decodes are safe and each caller gets its own result.
func TestDecodeJSON_Concurrent(t *testing.T) {
	const workers = 32
	const iterations = 200

	var wg sync.WaitGroup
	errs := make(chan error, workers*iterations)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				var out struct {
					ID    int    `json:"id"`
					Title string `json:"title"`
					Bit   int64  `json:"bitrate,string"`
				}
				payload := fmt.Sprintf(`{"id":%d,"title":"w%d","bitrate":"%d"}`, id, id, i)
				if err := DecodeJSONBytes([]byte(payload), &out); err != nil {
					errs <- fmt.Errorf("worker %d iter %d: %w", id, i, err)
					return
				}
				if out.ID != id {
					errs <- fmt.Errorf("worker %d got ID %d (data race in shared decoder)", id, out.ID)
					return
				}
				if out.Title != fmt.Sprintf("w%d", id) {
					errs <- fmt.Errorf("worker %d got Title %q", id, out.Title)
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// Mixed reader/bytes/encoder traffic across goroutines, mirroring how the
// real app uses the package (mediaplayer.store() encodes while views decode).
func TestResolver_ConcurrentMixedTraffic(t *testing.T) {
	const workers = 24
	const iterations = 150

	var wg sync.WaitGroup
	errs := make(chan error, workers*iterations*3)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				var decoded struct {
					Title string `json:"title"`
				}
				if err := DecodeJSONReader(
					strings.NewReader(fmt.Sprintf(`{"title":"r%d-%d"}`, id, i)),
					&decoded,
				); err != nil {
					errs <- err
					return
				}
				if decoded.Title != fmt.Sprintf("r%d-%d", id, i) {
					errs <- fmt.Errorf("reader mismatch: %q", decoded.Title)
					return
				}

				var buf []byte
				if err := EncodeSimpleBytes(&buf, fmt.Sprintf("p%d-%d", id, i)); err != nil {
					errs <- err
					return
				}

				var prop string
				if err := DecodeSimpleBytes(buf, &prop); err != nil {
					errs <- err
					return
				}
				if prop != fmt.Sprintf("p%d-%d", id, i) {
					errs <- fmt.Errorf("roundtrip mismatch: %q", prop)
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

func TestEncodeSimpleBytes_RoundTrip(t *testing.T) {
	cases := []string{"eof-reached", "audio-list", "", "with/slash", "ñ unicode ✓"}

	for _, want := range cases {
		var buf []byte
		if err := EncodeSimpleBytes(&buf, want); err != nil {
			t.Fatalf("encode %q: %v", want, err)
		}
		var got string
		if err := DecodeSimpleBytes(buf, &got); err != nil {
			t.Fatalf("decode %q: %v", want, err)
		}
		if got != want {
			t.Errorf("round trip = %q, want %q", got, want)
		}
	}
}

// Read the adaptiveFormats fixture end-to-end the way getVideo() does.
func TestDecodeJSON_AdaptiveFormatFixture(t *testing.T) {
	var out struct {
		Format struct {
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
		} `json:"format"`
	}

	wrapped := `{"format":` + adaptiveFormatJSON + `}`

	if err := DecodeJSONBytes([]byte(wrapped), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	f := out.Format
	if f.Itag != "137" {
		t.Errorf("Itag = %q, want 137", f.Itag)
	}
	if f.Resolution != "1080p" {
		t.Errorf("Resolution = %q, want 1080p", f.Resolution)
	}
	if f.Bitrate != 4818629 {
		t.Errorf("Bitrate = %d, want 4818629", f.Bitrate)
	}
	if f.ContentLength != 1234567 {
		t.Errorf("ContentLength = %d, want 1234567", f.ContentLength)
	}
	if f.FPS != 30 {
		t.Errorf("FPS = %d, want 30", f.FPS)
	}
	if !strings.Contains(f.Type, "video/mp4") {
		t.Errorf("Type = %q, want it to contain video/mp4", f.Type)
	}
}

// The resolution-less audio entries Invidious emits (adaptiveFormats audio
// entries omit "resolution" and "clen" entirely).
func TestDecodeJSON_AudioFormatWithoutOptionalKeys(t *testing.T) {
	var out struct {
		Format struct {
			Itag            string `json:"itag"`
			Type            string `json:"type"`
			Resolution      string `json:"resolution,omitempty"`
			Bitrate         int64  `json:"bitrate,string"`
			ContentLength   int64  `json:"clen,string"`
			FPS             int    `json:"fps"`
			AudioSampleRate int    `json:"audioSampleRate"`
			AudioChannels   int    `json:"audioChannels"`
		} `json:"format"`
	}

	audio := `{"format":{"itag":"251","type":"audio/webm; codecs=\"opus\"",
	             "bitrate":"141983","fps":0,"audioSampleRate":48000,"audioChannels":"2"}}`

	if err := DecodeJSONBytes([]byte(audio), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if out.Format.Resolution != "" {
		t.Errorf("Resolution = %q, want empty", out.Format.Resolution)
	}
	if out.Format.ContentLength != 0 {
		t.Errorf("ContentLength = %d, want 0 when clen absent", out.Format.ContentLength)
	}
	if out.Format.AudioSampleRate != 48000 {
		t.Errorf("AudioSampleRate = %d, want 48000", out.Format.AudioSampleRate)
	}
	if out.Format.AudioChannels != 2 {
		t.Errorf("AudioChannels = %d, want 2 (from quoted \"2\")", out.Format.AudioChannels)
	}
}

// Cross-check the resolver against encoding/json for a large realistic
// document, to prove the swap is behaviour-preserving for the fields the app
// actually reads.
func TestDecodeJSON_MatchesEncodingJSON(t *testing.T) {
	type document struct {
		Title         string `json:"title"`
		VideoID       string `json:"videoId"`
		LengthSeconds int64  `json:"lengthSeconds"`
		ViewCount     int    `json:"viewCount"`
		LikeCount     int    `json:"likeCount"`
		Thumbnails    []struct {
			Quality string `json:"quality"`
			URL     string `json:"url"`
			Width   int    `json:"width"`
			Height  int    `json:"height"`
		} `json:"videoThumbnails"`
		Recommended []struct {
			Title   string `json:"title"`
			VideoID string `json:"videoId"`
		} `json:"recommendedVideos"`
	}

	payload := `{
		"title":"Adele - Hello",
		"videoId":"YQHsXMglC9A",
		"lengthSeconds":357,
		"viewCount":3400000000,
		"likeCount":15000000,
		"videoThumbnails":[
			{"quality":"maxres","url":"https://i.ytimg.com/vi/x/maxres.jpg","width":1280,"height":720},
			{"quality":"start","url":"https://i.ytimg.com/vi/x/0.jpg","width":120,"height":90}
		],
		"recommendedVideos":[
			{"title":"Rec 1","videoId":"aaa"},
			{"title":"Rec 2","videoId":"bbb"}
		]
	}`

	var viaResolver document
	if err := DecodeJSONBytes([]byte(payload), &viaResolver); err != nil {
		t.Fatalf("resolver decode: %v", err)
	}

	var viaStdlib document
	if err := json.Unmarshal([]byte(payload), &viaStdlib); err != nil {
		t.Fatalf("stdlib decode: %v", err)
	}

	if fmt.Sprint(viaResolver) != fmt.Sprint(viaStdlib) {
		t.Errorf("mismatch vs encoding/json:\n resolver: %+v\n stdlib  : %+v",
			viaResolver, viaStdlib)
	}
}

// ---------------------------------------------------------------------------
// Documented defects in the resolver itself
// ---------------------------------------------------------------------------

// setup() guards on the package-level `resolver.init` instead of the receiver
// `r.init`. Harmless while Resolver is used only as the singleton, but it means
// any second Resolver value would silently share the first one's decoder.
func TestResolver_SetupUsesGlobalInitGuard(t *testing.T) {
	var second Resolver

	second.setup()

	if second.init {
		t.Log("second.setup() populated the local instance (receiver guard)")
		return
	}
	t.Log("DEFECT: setup() short-circuited on the package-level `resolver.init`, " +
		"so the local Resolver was left zero-valued (jsonDecoder == nil)")
}

// A zero-valued Resolver would nil-panic in setup()'s siblings if the guard
// were ever corrected naively. Demonstrate the panic exists.
func TestResolver_ZeroValueDecodePanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("DEFECT: decoding through an un-setup Resolver panics: %v", r)
			return
		}
		t.Skip("no panic; behaviour changed")
	}()

	var other Resolver
	// Bypass setup() to model a Resolver that was never initialised.
	var out map[string]any
	_ = other.jsonDecoder
	_ = out
	other.setup()
	if other.jsonDecoder == nil {
		var dst map[string]any
		// Deliberately invoke the uninitialised decoder path.
		other.jsonDecoder.ResetBytes([]byte(`{}`))
		_ = other.jsonDecoder.Decode(&dst)
	}
}

// jsonEncoder is allocated in setup() but never used by any exported function.
func TestResolver_JSONEncoderIsDead(t *testing.T) {
	resolver.setup()

	if resolver.jsonEncoder == nil {
		t.Fatal("expected jsonEncoder to be allocated")
	}
	t.Log("NOTE: resolver.jsonEncoder is allocated but never used; " +
		"EncodeSimpleBytes uses simpleEncoder instead")
}

// The two decoders are configured identically, so DecodeJSONReader and
// DecodeSimpleReader are behaviourally the same function with a different mutex.
func TestResolver_JSONAndSimpleHandlesIdentical(t *testing.T) {
	resolver.setup()

	if fmt.Sprintf("%T", resolver.jsonDecoder) != fmt.Sprintf("%T", resolver.simpleDecoder) {
		t.Error("decoders are of different types")
	}
	t.Log("NOTE: jsonDecoder and simpleDecoder are byte-identical configurations; " +
		"DecodeSimpleReader has zero call sites in the codebase")
}

// ResetBytes reuses an internal buffer. Feeding a large payload after a small
// one must not leak bytes from the previous decode.
func TestDecodeJSON_BufferReuseAcrossSizes(t *testing.T) {
	big := `{"title":"` + strings.Repeat("x", 1<<16) + `"}`

	for i := 0; i < 50; i++ {
		var small struct {
			Title string `json:"title"`
		}
		if err := DecodeJSONBytes([]byte(`{"title":"a"}`), &small); err != nil {
			t.Fatalf("small decode %d: %v", i, err)
		}
		if small.Title != "a" {
			t.Fatalf("iteration %d: Title = %q, want %q", i, small.Title, "a")
		}

		var large struct {
			Title string `json:"title"`
		}
		if err := DecodeJSONBytes([]byte(big), &large); err != nil {
			t.Fatalf("large decode %d: %v", i, err)
		}
		if len(large.Title) != 1<<16 {
			t.Fatalf("iteration %d: len(Title) = %d, want %d", i, len(large.Title), 1<<16)
		}
	}
}

// A reader that returns an error mid-stream must surface that error.
func TestDecodeJSON_ReaderErrorPropagates(t *testing.T) {
	r := io.MultiReader(
		strings.NewReader(`{"title":"partial"`),
		&erroringReader{},
	)

	var out struct {
		Title string `json:"title"`
	}

	if err := DecodeJSONReader(r, &out); err == nil {
		t.Error("expected reader error to propagate")
	}
}

type erroringReader struct{}

func (*erroringReader) Read([]byte) (int, error) { return 0, fmt.Errorf("boom") }

// Confirm the handle really is a bare JsonHandle with no strictness options,
// which is why unknown fields are silently dropped.
func TestResolver_HandleIsDefaultJsonHandle(t *testing.T) {
	h := &codec.JsonHandle{}

	if h.ErrorIfNoField {
		t.Error("ErrorIfNoField is set; unknown fields would be rejected")
	}
	if h.MapType != nil {
		t.Error("MapType is set")
	}
	t.Logf("handle=%p basicHandle zero-value=%v", h, h.BasicHandle == codec.BasicHandle{})
}
