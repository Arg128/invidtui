1. --play-video shows a Panic error:

2. Thumbnail error queue
The panic from --play-video is fixed. The initialization order change prevents nil deque access. The thumbnail decoding logic is corrected to use full thumbnail URLs from API (instead of constructing paths), handle HTML/error responses, and skip storyboard frames. All tests pass. Code builds and vets clean. Changes made to cmd/config.go (InitConfig), invidious/video.go (VideoThumbnail signature), ui/player/player.go (init/start safety, thumbnail selection), ui/ui.go (start order).
