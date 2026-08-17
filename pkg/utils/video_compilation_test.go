package utils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeCompilationOptions(t *testing.T) {
	got := normalizeCompilationOptions(CompilationOptions{})
	if got.Width != 1920 || got.Height != 1080 || got.FPS != 30 || got.CRF != 23 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if got.Preset != "veryfast" || got.AudioBitrate != "192k" {
		t.Fatalf("unexpected encoder defaults: %+v", got)
	}
}

func TestSafeFilename(t *testing.T) {
	if got := safeFilename("a/b:中 文"); got != "a_b____" {
		t.Fatalf("safeFilename() = %q", got)
	}
}

func TestBuildVideoCompilationNormalizesMixedSources(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	root := t.TempDir()
	withAudio := filepath.Join(root, "landscape-with-audio.mp4")
	withoutAudio := filepath.Join(root, "portrait-silent.mp4")
	runFFmpegFixture(t, ffmpeg,
		"-f", "lavfi", "-i", "color=c=red:s=320x180:d=0.8:r=24",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.8",
		"-shortest", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", withAudio,
	)
	runFFmpegFixture(t, ffmpeg,
		"-f", "lavfi", "-i", "color=c=blue:s=180x320:d=0.6:r=30",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-an", withoutAudio,
	)

	output := filepath.Join(root, "compiled.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := BuildVideoCompilation(
		ctx,
		[]CompilationSource{
			{VideoID: "first", Title: "first source", Path: withAudio},
			{VideoID: "second", Title: "second source", Path: withoutAudio},
		},
		root,
		output,
		CompilationOptions{
			Width:        320,
			Height:       240,
			FPS:          15,
			CRF:          30,
			Preset:       "ultrafast",
			AudioBitrate: "64k",
		},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildVideoCompilation() error = %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("compiled output missing: %v", err)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("segment count = %d, want 2", len(result.Segments))
	}
	if result.Segments[0].StartMS != 0 || result.Segments[1].StartMS != result.Segments[0].EndMS {
		t.Fatalf("timeline is not contiguous: %+v", result.Segments)
	}
	if !result.Probe.HasVideo || !result.Probe.HasAudio {
		t.Fatalf("compiled streams = video:%v audio:%v, want both", result.Probe.HasVideo, result.Probe.HasAudio)
	}
	if result.Probe.Width != 320 || result.Probe.Height != 240 {
		t.Fatalf("compiled dimensions = %dx%d, want 320x240", result.Probe.Width, result.Probe.Height)
	}
	if result.DurationMS < 1200 || result.DurationMS > 1800 {
		t.Fatalf("compiled duration = %dms, want approximately 1400ms", result.DurationMS)
	}
	if result.ManifestJSON == "" {
		t.Fatal("manifest is empty")
	}
	if err := ValidateMediaFile(ctx, output); err != nil {
		t.Fatalf("ValidateMediaFile() error = %v", err)
	}
}

func TestBuildVideoCompilationReusesNormalizedSegments(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	runFFmpegFixture(t, ffmpeg,
		"-f", "lavfi", "-i", "color=c=green:s=320x180:d=0.6:r=24",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.6",
		"-shortest", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", source,
	)
	options := CompilationOptions{
		Width: 320, Height: 240, FPS: 15, CRF: 30, Preset: "ultrafast", AudioBitrate: "64k",
	}
	output := filepath.Join(root, "compiled.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := BuildVideoCompilation(
		ctx,
		[]CompilationSource{{VideoID: "source", Title: "source", Path: source}},
		root,
		output,
		options,
		nil,
	); err != nil {
		t.Fatalf("first BuildVideoCompilation() error = %v", err)
	}

	normalized := filepath.Join(root, "normalized", "01_source.mp4")
	markerTime := time.Now().Add(time.Hour)
	if err := os.Chtimes(normalized, markerTime, markerTime); err != nil {
		t.Fatal(err)
	}
	var messages []string
	if _, err := BuildVideoCompilation(
		ctx,
		[]CompilationSource{{VideoID: "source", Title: "source", Path: source}},
		root,
		output,
		options,
		func(_ int, message string) { messages = append(messages, message) },
	); err != nil {
		t.Fatalf("second BuildVideoCompilation() error = %v", err)
	}
	reused := false
	for _, message := range messages {
		if message == "复用第1/1段" {
			reused = true
			break
		}
	}
	if !reused {
		t.Fatalf("normalized segment was not reused; progress messages = %v", messages)
	}
	info, err := os.Stat(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(markerTime) {
		t.Fatalf("normalized segment was rewritten: modtime = %v, want %v", info.ModTime(), markerTime)
	}
}

func runFFmpegFixture(t *testing.T, ffmpeg string, args ...string) {
	t.Helper()
	command := exec.Command(ffmpeg, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create media fixture: %v (%s)", err, output)
	}
}
