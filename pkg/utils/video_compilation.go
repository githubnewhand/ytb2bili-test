package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type MediaProbe struct {
	DurationMS int64  `json:"duration_ms"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	VideoCodec string `json:"video_codec"`
	AudioCodec string `json:"audio_codec,omitempty"`
	FrameRate  string `json:"frame_rate,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	HasVideo   bool   `json:"has_video"`
	HasAudio   bool   `json:"has_audio"`
	RawJSON    string `json:"-"`
}

type CompilationOptions struct {
	Width        int
	Height       int
	FPS          int
	CRF          int
	Preset       string
	AudioBitrate string
}

type CompilationSource struct {
	VideoID string
	Title   string
	Path    string
}

type CompilationSegment struct {
	VideoID        string `json:"video_id"`
	Title          string `json:"title"`
	SourcePath     string `json:"source_path"`
	NormalizedPath string `json:"normalized_path"`
	DurationMS     int64  `json:"duration_ms"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
}

type CompilationResult struct {
	OutputPath   string               `json:"output_path"`
	DurationMS   int64                `json:"duration_ms"`
	Probe        MediaProbe           `json:"probe"`
	Segments     []CompilationSegment `json:"segments"`
	ManifestJSON string               `json:"manifest_json"`
}

type CompilationProgress func(percent int, message string)

func ProbeMediaFile(ctx context.Context, path string) (*MediaProbe, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("媒体路径为空")
	}
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("读取媒体文件失败: %w", err)
	} else if info.IsDir() || info.Size() == 0 {
		return nil, fmt.Errorf("媒体文件无效: %s", path)
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("未找到 ffprobe: %w", err)
	}
	command := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-show_streams",
		"-show_format",
		"-of", "json",
		path,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffprobe 检测失败: %w (%s)", err, compactCommandOutput(output))
	}

	var payload struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			FrameRate  string `json:"avg_frame_rate"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			Duration   string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 结果失败: %w", err)
	}
	probe := &MediaProbe{RawJSON: string(output)}
	for _, stream := range payload.Streams {
		switch stream.CodecType {
		case "video":
			if !probe.HasVideo {
				probe.HasVideo = true
				probe.VideoCodec = stream.CodecName
				probe.Width = stream.Width
				probe.Height = stream.Height
				probe.FrameRate = stream.FrameRate
			}
		case "audio":
			if !probe.HasAudio {
				probe.HasAudio = true
				probe.AudioCodec = stream.CodecName
				probe.Channels = stream.Channels
				probe.SampleRate, _ = strconv.Atoi(stream.SampleRate)
			}
		}
	}
	duration, _ := strconv.ParseFloat(payload.Format.Duration, 64)
	probe.DurationMS = int64(duration * 1000)
	if !probe.HasVideo || probe.DurationMS <= 0 {
		return nil, fmt.Errorf("媒体缺少有效视频流或时长")
	}
	return probe, nil
}

func BuildVideoCompilation(
	ctx context.Context,
	sources []CompilationSource,
	outputDir string,
	outputPath string,
	options CompilationOptions,
	report CompilationProgress,
) (*CompilationResult, error) {
	if len(sources) < 1 {
		return nil, fmt.Errorf("没有可拼接的素材")
	}
	options = normalizeCompilationOptions(options)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建拼接目录失败: %w", err)
	}
	segmentsDir := filepath.Join(outputDir, "normalized")
	if err := os.MkdirAll(segmentsDir, 0755); err != nil {
		return nil, fmt.Errorf("创建规范化目录失败: %w", err)
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("未找到 ffmpeg: %w", err)
	}
	segments := make([]CompilationSegment, 0, len(sources))
	normalizedPaths := make([]string, 0, len(sources))
	timelineMS := int64(0)
	for index, source := range sources {
		probe, err := ProbeMediaFile(ctx, source.Path)
		if err != nil {
			return nil, fmt.Errorf("素材 %s 检测失败: %w", source.VideoID, err)
		}
		normalizedPath := filepath.Join(segmentsDir, fmt.Sprintf("%02d_%s.mp4", index+1, safeFilename(source.VideoID)))
		normalizedProbe, reusable := reusableNormalizedSegment(ctx, source.Path, normalizedPath, probe, options)
		if report != nil {
			message := fmt.Sprintf("规范化第%d/%d段", index+1, len(sources))
			if reusable {
				message = fmt.Sprintf("复用第%d/%d段", index+1, len(sources))
			}
			report(5+(index*55/len(sources)), message)
		}
		if !reusable {
			normalizedProbe, err = normalizeCompilationSourceAtomic(ctx, ffmpegPath, source.Path, normalizedPath, probe, options)
			if err != nil {
				return nil, fmt.Errorf("素材 %s 规范化失败: %w", source.VideoID, err)
			}
		}
		segment := CompilationSegment{
			VideoID:        source.VideoID,
			Title:          source.Title,
			SourcePath:     source.Path,
			NormalizedPath: normalizedPath,
			DurationMS:     normalizedProbe.DurationMS,
			StartMS:        timelineMS,
			EndMS:          timelineMS + normalizedProbe.DurationMS,
		}
		timelineMS = segment.EndMS
		segments = append(segments, segment)
		normalizedPaths = append(normalizedPaths, normalizedPath)
	}

	if report != nil {
		report(65, "拼接规范化片段")
	}
	temporaryOutput := outputPath + ".tmp.mp4"
	if err := concatNormalizedSources(ctx, ffmpegPath, outputDir, normalizedPaths, temporaryOutput); err != nil {
		return nil, err
	}
	if report != nil {
		report(85, "校验拼接成片")
	}
	if err := ValidateMediaFile(ctx, temporaryOutput); err != nil {
		return nil, err
	}
	finalProbe, err := ProbeMediaFile(ctx, temporaryOutput)
	if err != nil {
		return nil, fmt.Errorf("成片检测失败: %w", err)
	}
	if err := os.Rename(temporaryOutput, outputPath); err != nil {
		return nil, fmt.Errorf("保存最终成片失败: %w", err)
	}

	manifest := struct {
		Version     int                  `json:"version"`
		GeneratedAt time.Time            `json:"generated_at"`
		OutputPath  string               `json:"output_path"`
		DurationMS  int64                `json:"duration_ms"`
		Segments    []CompilationSegment `json:"segments"`
	}{
		Version:     1,
		GeneratedAt: time.Now(),
		OutputPath:  outputPath,
		DurationMS:  finalProbe.DurationMS,
		Segments:    segments,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("生成拼接清单失败: %w", err)
	}
	if report != nil {
		report(100, "拼接成片已生成")
	}
	return &CompilationResult{
		OutputPath:   outputPath,
		DurationMS:   finalProbe.DurationMS,
		Probe:        *finalProbe,
		Segments:     segments,
		ManifestJSON: string(manifestBytes),
	}, nil
}

func ValidateMediaFile(ctx context.Context, path string) error {
	probe, err := ProbeMediaFile(ctx, path)
	if err != nil {
		return err
	}
	if probe.DurationMS <= 5*60*1000 {
		return validateMediaWindow(ctx, path)
	}
	if err := validateMediaWindow(ctx, path, "-t", "30"); err != nil {
		return fmt.Errorf("成片开头校验失败: %w", err)
	}
	if err := validateMediaWindow(ctx, path, "-sseof", "-30"); err != nil {
		return fmt.Errorf("成片结尾校验失败: %w", err)
	}
	return nil
}

func validateMediaWindow(ctx context.Context, path string, inputArgs ...string) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("未找到 ffmpeg: %w", err)
	}
	args := []string{"-v", "error"}
	args = append(args, inputArgs...)
	args = append(args,
		"-i", path,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-f", "null",
		"-",
	)
	output, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("成片解码校验失败: %w (%s)", err, compactCommandOutput(output))
	}
	return nil
}

func reusableNormalizedSegment(ctx context.Context, sourcePath, normalizedPath string, sourceProbe *MediaProbe, options CompilationOptions) (*MediaProbe, bool) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, false
	}
	normalizedInfo, err := os.Stat(normalizedPath)
	if err != nil || normalizedInfo.IsDir() || normalizedInfo.Size() == 0 || normalizedInfo.ModTime().Before(sourceInfo.ModTime()) {
		return nil, false
	}
	probe, err := ProbeMediaFile(ctx, normalizedPath)
	if err != nil || !normalizedProbeMatches(probe, sourceProbe, options) {
		return nil, false
	}
	return probe, true
}

func normalizedProbeMatches(probe, sourceProbe *MediaProbe, options CompilationOptions) bool {
	if probe == nil || sourceProbe == nil || !probe.HasVideo || !probe.HasAudio {
		return false
	}
	if probe.VideoCodec != "h264" || probe.AudioCodec != "aac" || probe.Width != options.Width || probe.Height != options.Height {
		return false
	}
	if probe.SampleRate != 48000 || probe.Channels != 2 || !frameRateMatches(probe.FrameRate, options.FPS) {
		return false
	}
	difference := probe.DurationMS - sourceProbe.DurationMS
	if difference < 0 {
		difference = -difference
	}
	return difference <= 3000
}

func frameRateMatches(value string, target int) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return false
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return false
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator == 0 {
		return false
	}
	rate := numerator / denominator
	return rate > float64(target)-0.02 && rate < float64(target)+0.02
}

func normalizeCompilationSourceAtomic(ctx context.Context, ffmpegPath, inputPath, outputPath string, probe *MediaProbe, options CompilationOptions) (*MediaProbe, error) {
	temporaryPath := outputPath + ".tmp.mp4"
	_ = os.Remove(temporaryPath)
	if err := normalizeCompilationSource(ctx, ffmpegPath, inputPath, temporaryPath, probe, options); err != nil {
		_ = os.Remove(temporaryPath)
		return nil, err
	}
	normalizedProbe, err := ProbeMediaFile(ctx, temporaryPath)
	if err != nil {
		_ = os.Remove(temporaryPath)
		return nil, fmt.Errorf("检测规范化结果失败: %w", err)
	}
	if !normalizedProbeMatches(normalizedProbe, probe, options) {
		_ = os.Remove(temporaryPath)
		return nil, fmt.Errorf("规范化结果参数不符合目标格式")
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		_ = os.Remove(temporaryPath)
		return nil, fmt.Errorf("保存规范化结果失败: %w", err)
	}
	return normalizedProbe, nil
}

func normalizeCompilationSource(ctx context.Context, ffmpegPath, inputPath, outputPath string, probe *MediaProbe, options CompilationOptions) error {
	videoFilter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps=%d,format=yuv420p",
		options.Width, options.Height, options.Width, options.Height, options.FPS,
	)
	args := []string{"-y", "-i", inputPath}
	if !probe.HasAudio {
		args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
	}
	args = append(args,
		"-map", "0:v:0",
	)
	if probe.HasAudio {
		args = append(args, "-map", "0:a:0")
	} else {
		args = append(args, "-map", "1:a:0")
	}
	args = append(args,
		"-vf", videoFilter,
		"-c:v", "libx264",
		"-preset", options.Preset,
		"-crf", strconv.Itoa(options.CRF),
		"-c:a", "aac",
		"-b:a", options.AudioBitrate,
		"-ar", "48000",
		"-ac", "2",
		"-movflags", "+faststart",
	)
	if !probe.HasAudio {
		args = append(args, "-shortest")
	}
	args = append(args, outputPath)
	output, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg规范化失败: %w (%s)", err, compactCommandOutput(output))
	}
	return nil
}

func concatNormalizedSources(ctx context.Context, ffmpegPath, outputDir string, paths []string, outputPath string) error {
	listPath := filepath.Join(outputDir, "concat.txt")
	lines := make([]string, 0, len(paths))
	for _, path := range paths {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("解析片段路径失败: %w", err)
		}
		escaped := strings.ReplaceAll(absolutePath, "'", "'\\''")
		lines = append(lines, "file '"+escaped+"'")
	}
	if err := os.WriteFile(listPath, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return fmt.Errorf("写入拼接列表失败: %w", err)
	}
	output, err := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg拼接失败: %w (%s)", err, compactCommandOutput(output))
	}
	return nil
}

func normalizeCompilationOptions(options CompilationOptions) CompilationOptions {
	if options.Width <= 0 {
		options.Width = 1920
	}
	if options.Height <= 0 {
		options.Height = 1080
	}
	if options.FPS <= 0 {
		options.FPS = 30
	}
	if options.CRF <= 0 {
		options.CRF = 23
	}
	if options.Preset == "" {
		options.Preset = "veryfast"
	}
	if options.AudioBitrate == "" {
		options.AudioBitrate = "192k"
	}
	return options
}

func safeFilename(value string) string {
	var builder strings.Builder
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
			builder.WriteRune(character)
		case character >= 'A' && character <= 'Z':
			builder.WriteRune(character)
		case character >= '0' && character <= '9':
			builder.WriteRune(character)
		case character == '-' || character == '_':
			builder.WriteRune(character)
		default:
			builder.WriteRune('_')
		}
	}
	if builder.Len() == 0 {
		return "segment"
	}
	return builder.String()
}

func compactCommandOutput(output []byte) string {
	value := strings.TrimSpace(string(output))
	if len(value) > 1200 {
		value = value[len(value)-1200:]
	}
	return value
}
