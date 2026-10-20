package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type subtitleTranslationCheckpoint struct {
	Signature    string            `json:"signature"`
	Translations map[string]string `json:"translations"`
}

func subtitleCheckpointSignature(entries []SRTEntry, provider, model string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x1f%s\x1f%s\n", provider, model, subtitleTranslationPromptVersion)
	for _, entry := range entries {
		fmt.Fprintf(hash, "%s\x1f%s\n", entry.CueID, entry.Text)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (t *TranslateSubtitle) loadSubtitleCheckpoint() map[string]string {
	results := make(map[string]string)
	if t.CheckpointPath == "" || t.CheckpointSignature == "" {
		return results
	}
	contents, err := os.ReadFile(t.CheckpointPath)
	if err != nil {
		return results
	}
	var checkpoint subtitleTranslationCheckpoint
	if json.Unmarshal(contents, &checkpoint) != nil || checkpoint.Signature != t.CheckpointSignature {
		return results
	}
	for id, translation := range checkpoint.Translations {
		if isValidatedTranslation(translation) {
			results[id] = translation
		}
	}
	return results
}

func (t *TranslateSubtitle) saveSubtitleCheckpoint() error {
	if t.CheckpointPath == "" || t.CheckpointSignature == "" {
		return nil
	}
	valid := make(map[string]string)
	for id, translation := range t.CheckpointResults {
		if isValidatedTranslation(translation) {
			valid[id] = strings.TrimSpace(translation)
		}
	}
	contents, err := json.Marshal(subtitleTranslationCheckpoint{Signature: t.CheckpointSignature, Translations: valid})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.CheckpointPath), 0755); err != nil {
		return err
	}
	temporary := t.CheckpointPath + ".tmp"
	if err := os.WriteFile(temporary, contents, 0644); err != nil {
		return err
	}
	return os.Rename(temporary, t.CheckpointPath)
}
