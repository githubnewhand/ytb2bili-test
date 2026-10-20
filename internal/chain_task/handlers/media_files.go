package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const mediaProbeTimeout = 30 * time.Second
const mediaVerificationVersion = 1

type mediaStreams struct {
	hasVideo bool
	hasAudio bool
	duration float64
}

type mediaIntegrityError struct {
	path   string
	reason string
}

func (e *mediaIntegrityError) Error() string {
	return fmt.Sprintf("媒体数据损坏: %s (%s)", e.path, e.reason)
}

type mediaVerificationStamp struct {
	Version     int   `json:"version"`
	Size        int64 `json:"size"`
	ModTimeNano int64 `json:"mod_time_nano"`
}

// probeMediaStreams inspects the actual streams instead of trusting an
// extension. yt-dlp intermediate files can have a normal .mp4/.webm suffix
// while containing only one of the two streams.
func probeMediaStreams(path string) (mediaStreams, error) {
	if !isFinalizedMediaFile(path) {
		return mediaStreams{}, fmt.Errorf("不是已完成的媒体文件: %s", path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type:format=duration", "-of", "json", path)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return mediaStreams{}, fmt.Errorf("ffprobe 检查超时: %s", path)
	}
	if err != nil {
		return mediaStreams{}, fmt.Errorf("ffprobe 检查失败: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	var result struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return mediaStreams{}, fmt.Errorf("解析 ffprobe 结果失败: %w", err)
	}

	var streams mediaStreams
	for _, stream := range result.Streams {
		switch strings.ToLower(strings.TrimSpace(stream.CodecType)) {
		case "video":
			streams.hasVideo = true
		case "audio":
			streams.hasAudio = true
		}
	}
	if result.Format.Duration != "" {
		streams.duration, _ = strconv.ParseFloat(result.Format.Duration, 64)
	}
	return streams, nil
}

func mediaVerificationPath(path string) string {
	return path + ".media-verified.json"
}

func hasCurrentMediaVerification(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(mediaVerificationPath(path))
	if err != nil {
		return false
	}
	var stamp mediaVerificationStamp
	if json.Unmarshal(data, &stamp) != nil {
		return false
	}
	return stamp.Version == mediaVerificationVersion && stamp.Size == info.Size() && stamp.ModTimeNano == info.ModTime().UnixNano()
}

func saveMediaVerification(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stamp := mediaVerificationStamp{Version: mediaVerificationVersion, Size: info.Size(), ModTimeNano: info.ModTime().UnixNano()}
	data, err := json.Marshal(stamp)
	if err != nil {
		return err
	}
	tempPath := mediaVerificationPath(path) + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tempPath, mediaVerificationPath(path))
}

func truncateValidationOutput(output []byte) string {
	const maxBytes = 1200
	if len(output) <= maxBytes {
		return strings.TrimSpace(string(output))
	}
	return strings.TrimSpace(string(output[:maxBytes])) + "..."
}

// validateCompleteMedia performs short strict decodes at the beginning,
// middle, and end. This catches files whose MP4 metadata is readable but whose
// actual audio/video packets were corrupted by an interrupted ranged download.
// A size+mtime stamp makes subsequent task retries and upload checks cheap.
func validateCompleteMedia(path string) error {
	if hasCurrentMediaVerification(path) {
		return nil
	}

	streams, err := probeMediaStreams(path)
	if err != nil {
		return err
	}
	if !streams.hasVideo || !streams.hasAudio {
		return fmt.Errorf("文件不是完整音视频: %s", path)
	}
	if streams.duration < 1 {
		return &mediaIntegrityError{path: path, reason: "时长小于 1 秒或无法读取"}
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("未找到 ffmpeg，无法执行上传前媒体完整性校验: %w", err)
	}

	offsets := []float64{0}
	if streams.duration > 12 {
		offsets = append(offsets, streams.duration/2, streams.duration-3)
	}
	for _, offset := range offsets {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		args := []string{"-v", "error", "-xerror", "-err_detect", "explode"}
		if offset > 0 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", offset))
		}
		args = append(args, "-i", path, "-t", "0.5", "-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-")
		output, runErr := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if timedOut {
			return fmt.Errorf("媒体完整性校验超时（%.1f 秒处）: %s", offset, path)
		}
		if runErr != nil {
			return &mediaIntegrityError{path: path, reason: fmt.Sprintf("%.1f 秒处无法解码: %s", offset, truncateValidationOutput(output))}
		}
	}

	return saveMediaVerification(path)
}

func isFinalizedMediaFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return false
	}

	lower := strings.ToLower(path)
	for _, suffix := range []string{".part", ".ytdl", ".tmp", ".temp"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".webm", ".mov", ".flv", ".m4a", ".mp3", ".wav", ".aac", ".ogg", ".opus":
		return true
	default:
		return false
	}
}

func listFinalizedMediaFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	type candidate struct {
		path    string
		modTime time.Time
	}
	candidates := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if !isFinalizedMediaFile(path) {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			candidates = append(candidates, candidate{path: path, modTime: info.ModTime()})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].modTime.After(candidates[j].modTime) })

	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.path)
	}
	return paths
}

// findCompleteVideo returns only a playable, merged video with both streams.
// Pure yt-dlp video/audio fragments are intentionally excluded.
func findCompleteVideo(dir string) (string, error) {
	var probeErrors []string
	for _, path := range listFinalizedMediaFiles(dir) {
		if err := validateCompleteMedia(path); err != nil {
			probeErrors = append(probeErrors, err.Error())
			continue
		}
		return path, nil
	}
	if len(probeErrors) > 0 {
		return "", fmt.Errorf("没有找到同时含视频和音频的完整文件；检查详情: %s", strings.Join(probeErrors, "; "))
	}
	return "", fmt.Errorf("没有找到同时含视频和音频的完整文件")
}

// quarantineCorruptCompleteVideos moves only files conclusively proven to have
// corrupt decodable packets. Tooling errors and incomplete single-stream files
// are left untouched. The original bytes remain recoverable for diagnosis.
func quarantineCorruptCompleteVideos(dir string) []string {
	var quarantined []string
	for _, path := range listFinalizedMediaFiles(dir) {
		streams, err := probeMediaStreams(path)
		if err != nil || !streams.hasVideo || !streams.hasAudio {
			continue
		}
		err = validateCompleteMedia(path)
		var integrityErr *mediaIntegrityError
		if !errors.As(err, &integrityErr) {
			continue
		}
		quarantinePath := fmt.Sprintf("%s.corrupt-%s", path, time.Now().Format("20060102-150405"))
		if renameErr := os.Rename(path, quarantinePath); renameErr == nil {
			_ = os.Remove(mediaVerificationPath(path))
			quarantined = append(quarantined, quarantinePath)
		}
	}
	return quarantined
}

// findAudioSource is deliberately more permissive than findCompleteVideo:
// Whisper only needs audio, so a completed yt-dlp audio stream can recover a
// missing WAV without forcing a multi-gigabyte video download to start over.
func findAudioSource(stateInput string, contextPath string, dir string) (string, error) {
	seen := make(map[string]struct{})
	candidates := make([]string, 0, 8)
	for _, path := range append([]string{stateInput, contextPath}, listFinalizedMediaFiles(dir)...) {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}

	var probeErrors []string
	for _, path := range candidates {
		streams, err := probeMediaStreams(path)
		if err != nil {
			probeErrors = append(probeErrors, err.Error())
			continue
		}
		if streams.hasAudio {
			return path, nil
		}
	}
	if len(probeErrors) > 0 {
		return "", fmt.Errorf("没有找到可用于转录的音频流；检查详情: %s", strings.Join(probeErrors, "; "))
	}
	return "", fmt.Errorf("没有找到可用于转录的音频流")
}

func nonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
