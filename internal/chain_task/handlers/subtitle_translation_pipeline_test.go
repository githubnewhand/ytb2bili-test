package handlers

import (
	"strings"
	"testing"
)

func testSubtitleEntries() []SRTEntry {
	return []SRTEntry{
		{Index: 1, TimeCode: "00:00:01,000 --> 00:00:03,000", Text: "Please close your eyes."},
		{Index: 2, TimeCode: "00:00:04,000 --> 00:00:06,000", Text: "Right."},
		{Index: 3, TimeCode: "00:00:07,000 --> 00:00:09,000", Text: "Please close your eyes."},
		{Index: 4, TimeCode: "00:00:10,000 --> 00:00:12,000", Text: "Right."},
		{Index: 5, TimeCode: "00:00:13,000 --> 00:00:15,000", Text: "谢谢。"},
	}
}

func TestTranslationQueueDeduplicatesOnlySafeRequestItems(t *testing.T) {
	entries := testSubtitleEntries()
	if err := assignSubtitleCueIDs("video-1", entries); err != nil {
		t.Fatalf("assign cue IDs: %v", err)
	}
	items, bypassed, stats := buildSubtitleTranslationQueue(entries, "deepseek-chat")

	if len(entries) != 5 {
		t.Fatalf("original timeline was modified: got %d entries", len(entries))
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 queue items (one safe duplicate plus two short cues), got %d", len(items))
	}
	if stats.Deduplicated != 1 {
		t.Fatalf("expected one deduplicated queue item, got %d", stats.Deduplicated)
	}
	if len(items[0].CueIDs) != 2 {
		t.Fatalf("safe duplicate should map back to two CueIDs, got %d", len(items[0].CueIDs))
	}
	if len(items[1].CueIDs) != 1 || len(items[2].CueIDs) != 1 {
		t.Fatal("short ambiguous subtitles must keep separate queue items")
	}
	if got := bypassed[entries[4].CueID]; got != "谢谢。" {
		t.Fatalf("Chinese cue should bypass translation, got %q", got)
	}
	for i, entry := range entries {
		if entry.Index != i+1 || entry.TimeCode == "" || entry.CueID == "" {
			t.Fatalf("original entry %d lost its timeline identity: %+v", i, entry)
		}
	}
}

func TestSubtitleIntegrityTripleValidation(t *testing.T) {
	entries := testSubtitleEntries()
	if err := assignSubtitleCueIDs("video-2", entries); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotSubtitleIntegrity(entries)
	if err != nil {
		t.Fatal(err)
	}
	translations := make(map[string]string, len(entries))
	for _, entry := range entries {
		translations[entry.CueID] = "译文"
	}
	if err := validateSubtitleIntegrity(snapshot, entries, translations); err != nil {
		t.Fatalf("valid subtitles failed integrity check: %v", err)
	}

	fewer := append([]SRTEntry(nil), entries[:len(entries)-1]...)
	if err := validateSubtitleIntegrity(snapshot, fewer, translations); err == nil || !strings.Contains(err.Error(), "原始条目数量") {
		t.Fatalf("count validation did not fail correctly: %v", err)
	}

	missing := make(map[string]string, len(translations)-1)
	for cueID, value := range translations {
		missing[cueID] = value
	}
	delete(missing, entries[0].CueID)
	if err := validateSubtitleIntegrity(snapshot, entries, missing); err == nil || !strings.Contains(err.Error(), "CueID") {
		t.Fatalf("CueID validation did not fail correctly: %v", err)
	}

	changedTimeline := append([]SRTEntry(nil), entries...)
	changedTimeline[0].TimeCode = "00:00:02,000 --> 00:00:03,000"
	if err := validateSubtitleIntegrity(snapshot, changedTimeline, translations); err == nil || !strings.Contains(err.Error(), "时间轴") {
		t.Fatalf("timeline validation did not fail correctly: %v", err)
	}
}

func TestRenderedSubtitleRetainsEveryCueAndTimeline(t *testing.T) {
	entries := testSubtitleEntries()
	if err := assignSubtitleCueIDs("video-3", entries); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotSubtitleIntegrity(entries)
	if err != nil {
		t.Fatal(err)
	}
	translations := make(map[string]string, len(entries))
	for _, entry := range entries {
		translations[entry.CueID] = "译文-" + entry.CueID[:6]
	}
	content := generateTranslatedSRTContentByCue(entries, translations)
	translator := &TranslateSubtitle{}
	if err := translator.validateRenderedSubtitle(content, "video-3", snapshot); err != nil {
		t.Fatalf("rendered subtitle failed validation: %v", err)
	}

	broken := strings.Replace(content, entries[0].TimeCode, "00:00:02,000 --> 00:00:03,000", 1)
	if err := translator.validateRenderedSubtitle(broken, "video-3", snapshot); err == nil {
		t.Fatal("changed rendered timeline should fail validation")
	}
}

func TestDynamicSubtitleGroupingUsesLineAndCharacterBudgets(t *testing.T) {
	translator := &TranslateSubtitle{GroupSize: 50}
	items := make([]subtitleTranslationItem, 0, 101)
	for i := 0; i < 101; i++ {
		items = append(items, subtitleTranslationItem{ID: "T", Text: "short subtitle"})
	}
	groups := translator.buildSubtitleTranslationItemGroups(items)
	if len(groups) != 3 || len(groups[0]) != 50 || len(groups[1]) != 50 || len(groups[2]) != 1 {
		t.Fatalf("unexpected line-budget grouping: got %d groups", len(groups))
	}

	longItems := []subtitleTranslationItem{
		{ID: "T1", Text: strings.Repeat("a", 4000)},
		{ID: "T2", Text: strings.Repeat("b", 3000)},
	}
	longGroups := translator.buildSubtitleTranslationItemGroups(longItems)
	if len(longGroups) != 2 {
		t.Fatalf("character budget should split long items, got %d groups", len(longGroups))
	}
}

func TestParseSubtitleTranslationJSON(t *testing.T) {
	parsed, err := parseSubtitleTranslationJSON("```json\n{\"T000001\":\"闭上眼睛。\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if parsed["T000001"] != "闭上眼睛。" {
		t.Fatalf("unexpected translation: %q", parsed["T000001"])
	}
	if _, err := parseSubtitleTranslationJSON("not json"); err == nil {
		t.Fatal("invalid response should fail")
	}
}

func TestEnsureSubtitleTranslationMapMakesFailureAccumulatorWritable(t *testing.T) {
	translations := ensureSubtitleTranslationMap(nil)
	translations["T000001"] = "降级译文"
	if got := translations["T000001"]; got != "降级译文" {
		t.Fatalf("unexpected fallback translation: %q", got)
	}

	existing := map[string]string{"T000002": "已有译文"}
	if got := ensureSubtitleTranslationMap(existing); got["T000002"] != "已有译文" {
		t.Fatal("existing translations must be preserved")
	}
}
