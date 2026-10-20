package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/difyz9/ytb2bili/internal/chain_task/handlers"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"go.uber.org/zap"
)

func main() {
	videoID := flag.String("video-id", "", "YouTube video ID")
	date := flag.String("date", time.Now().Format("2006-01-02"), "media directory date")
	sourceSRT := flag.String("source-srt", "", "source language SRT")
	metadata := flag.Bool("metadata", false, "also run the formal metadata handler in file-only mode")
	flag.Parse()
	if *videoID == "" || *sourceSRT == "" {
		fmt.Fprintln(os.Stderr, "-video-id and -source-srt are required")
		os.Exit(2)
	}
	if err := run(*videoID, *date, *sourceSRT, *metadata); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(videoID, date, sourceSRT string, metadata bool) error {
	config, err := types.LoadConfig("config.toml")
	if err != nil {
		return err
	}
	if config.SubtitleTranslationConfig == nil || config.SubtitleTranslationConfig.Provider != "ollama" {
		return fmt.Errorf("set SubtitleTranslationConfig.provider to ollama")
	}
	root, err := filepath.Abs(config.FileUpDir)
	if err != nil {
		return err
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return err
	}
	logger, err := zap.NewDevelopment()
	if err != nil {
		return err
	}
	defer logger.Sync()
	app := core.NewServer(config, logger.Sugar())
	state := manager.NewStateManager(0, videoID, root, day)
	context := map[string]interface{}{
		types.TaskProgressReporterKey: types.ProgressReporter(func(percent int, message string) {
			fmt.Printf("progress=%d%% %s\n", percent, message)
		}),
	}
	if info, err := os.Stat(state.OriginalWAV); err != nil || info.Size() == 0 {
		if !handlers.NewExtractAudio("分离音频", app, state, nil).Execute(context) {
			return fmt.Errorf("audio extraction: %v", context["error"])
		}
	}
	source, err := os.ReadFile(sourceSRT)
	if err != nil {
		return err
	}
	target := filepath.Join(state.CurrentDir, videoID+".srt")
	if err := os.WriteFile(target, source, 0644); err != nil {
		return err
	}
	if !handlers.NewTranslateSubtitle("翻译字幕", app, state, nil, nil, "").Execute(context) {
		return fmt.Errorf("subtitle translation: %v", context["error"])
	}
	if metadata {
		generator := handlers.NewGenerateMetadata("生成元数据", app, state, nil, "", nil, nil)
		generator.FileOnly = true
		if !generator.Execute(context) {
			return fmt.Errorf("metadata generation: %v", context["error"])
		}
		fmt.Printf("metadata: %s\n", filepath.Join(state.CurrentDir, "meta.json"))
	}
	fmt.Printf("completed: video=%s audio=%s source=%s target=%s cues=%v stats=%v\n",
		state.InputVideoPath, state.OriginalWAV, target, filepath.Join(state.CurrentDir, "zh.srt"),
		context["translated_count"], context["translation_stats"])
	return nil
}
