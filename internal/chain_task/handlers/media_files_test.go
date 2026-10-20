package handlers

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeTestWAV(t *testing.T, path string) {
	t.Helper()
	const sampleRate = uint32(16000)
	const sampleCount = uint32(160)
	dataSize := sampleCount * 2
	header := make([]byte, 44+dataSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], 1)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	binary.LittleEndian.PutUint32(header[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)
	if err := os.WriteFile(path, header, 0644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
}

func TestAudioOnlyFileCanRecoverWAVButCannotCompleteDownload(t *testing.T) {
	dir := t.TempDir()
	audioPath := filepath.Join(dir, "video.f251.wav")
	writeTestWAV(t, audioPath)

	source, err := findAudioSource(filepath.Join(dir, "video.mp4"), "", dir)
	if err != nil {
		t.Fatalf("audio-only source should be usable for transcription: %v", err)
	}
	if source != audioPath {
		t.Fatalf("got audio source %q, want %q", source, audioPath)
	}

	if complete, err := findCompleteVideo(dir); err == nil || complete != "" {
		t.Fatalf("audio-only source must not be accepted as complete video: path=%q err=%v", complete, err)
	}
}

func TestPartialDownloadIsNeverACompletedMediaCandidate(t *testing.T) {
	dir := t.TempDir()
	partPath := filepath.Join(dir, "video.f401.mp4.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}
	if isFinalizedMediaFile(partPath) {
		t.Fatal(".part file must never be treated as finalized media")
	}
	if got := listFinalizedMediaFiles(dir); len(got) != 0 {
		t.Fatalf("partial file leaked into media candidates: %v", got)
	}
}
