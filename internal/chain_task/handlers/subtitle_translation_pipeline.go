package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/pkg/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	subtitleTranslationPromptVersion = "subtitle-a-plus-v1"
	defaultTranslationGroupSize      = 50
	maximumTranslationGroupSize      = 60
	maximumTranslationGroupChars     = 6000
)

type subtitleTranslationItem struct {
	ID        string
	Text      string
	CacheKey  string
	Cacheable bool
	CueIDs    []string
}

type subtitleTranslationStats struct {
	OriginalEntries int
	BypassedEntries int
	UniqueTexts     int
	Deduplicated    int
	CacheHitTexts   int
	CacheHitCues    int
	RequestedTexts  int
	RequestedGroups int
	CachedTexts     int
}

type subtitleTimelineStamp struct {
	Index    int
	TimeCode string
}

type subtitleIntegritySnapshot struct {
	Count  int
	ByCue  map[string]subtitleTimelineStamp
	CueIDs []string
}

type subtitleCacheKeyLock struct {
	mutex sync.Mutex
	refs  int
}

var subtitleCacheLockRegistry = struct {
	sync.Mutex
	locks map[string]*subtitleCacheKeyLock
}{locks: make(map[string]*subtitleCacheKeyLock)}

// assignSubtitleCueIDs gives every original timeline entry a stable identity.
// Text is deliberately excluded: translated text must resolve to the same cue.
func assignSubtitleCueIDs(videoID string, entries []SRTEntry) error {
	seen := make(map[string]struct{}, len(entries))
	for i := range entries {
		raw := fmt.Sprintf("%s\x1f%d\x1f%s", videoID, entries[i].Index, strings.TrimSpace(entries[i].TimeCode))
		sum := sha256.Sum256([]byte(raw))
		entries[i].CueID = hex.EncodeToString(sum[:16])
		if _, exists := seen[entries[i].CueID]; exists {
			return fmt.Errorf("检测到重复CueID：字幕序号%d，时间轴%s", entries[i].Index, entries[i].TimeCode)
		}
		seen[entries[i].CueID] = struct{}{}
	}
	return nil
}

func snapshotSubtitleIntegrity(entries []SRTEntry) (*subtitleIntegritySnapshot, error) {
	snapshot := &subtitleIntegritySnapshot{
		Count:  len(entries),
		ByCue:  make(map[string]subtitleTimelineStamp, len(entries)),
		CueIDs: make([]string, 0, len(entries)),
	}
	for _, entry := range entries {
		if entry.CueID == "" {
			return nil, fmt.Errorf("字幕序号%d缺少CueID", entry.Index)
		}
		if _, exists := snapshot.ByCue[entry.CueID]; exists {
			return nil, fmt.Errorf("CueID重复：%s", entry.CueID)
		}
		snapshot.ByCue[entry.CueID] = subtitleTimelineStamp{Index: entry.Index, TimeCode: entry.TimeCode}
		snapshot.CueIDs = append(snapshot.CueIDs, entry.CueID)
	}
	return snapshot, nil
}

// validateSubtitleIntegrity performs the three mandatory checks:
// original entry count, exact CueID set and an unchanged timeline.
func validateSubtitleIntegrity(snapshot *subtitleIntegritySnapshot, entries []SRTEntry, translations map[string]string) error {
	if snapshot == nil {
		return fmt.Errorf("字幕完整性快照不存在")
	}
	if len(entries) != snapshot.Count {
		return fmt.Errorf("原始条目数量校验失败：期望%d，实际%d", snapshot.Count, len(entries))
	}
	if len(translations) != snapshot.Count {
		return fmt.Errorf("CueID校验失败：期望%d个翻译，实际%d个", snapshot.Count, len(translations))
	}

	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		original, exists := snapshot.ByCue[entry.CueID]
		if !exists {
			return fmt.Errorf("CueID校验失败：出现未知CueID %s", entry.CueID)
		}
		if _, duplicate := seen[entry.CueID]; duplicate {
			return fmt.Errorf("CueID校验失败：CueID %s 被重复使用", entry.CueID)
		}
		seen[entry.CueID] = struct{}{}
		if original.Index != entry.Index || original.TimeCode != entry.TimeCode {
			return fmt.Errorf("时间轴校验失败：CueID %s 的序号或时间码被修改", entry.CueID)
		}
		translation, ok := translations[entry.CueID]
		if !ok || strings.TrimSpace(translation) == "" {
			return fmt.Errorf("CueID校验失败：CueID %s 没有有效译文", entry.CueID)
		}
	}
	return nil
}

func (t *TranslateSubtitle) validateRenderedSubtitle(content, videoID string, snapshot *subtitleIntegritySnapshot) error {
	entries, err := t.parseSRTContent(content)
	if err != nil {
		return fmt.Errorf("解析生成字幕失败: %w", err)
	}
	if err := assignSubtitleCueIDs(videoID, entries); err != nil {
		return err
	}
	translations := make(map[string]string, len(entries))
	for _, entry := range entries {
		translations[entry.CueID] = entry.Text
	}
	return validateSubtitleIntegrity(snapshot, entries, translations)
}

func (t *TranslateSubtitle) translateEntriesAPlus(entries []SRTEntry, context map[string]interface{}) (map[string]string, subtitleTranslationStats, error) {
	provider := subtitleTranslationProvider(t.App.Config)
	modelName := "deepseek-chat"
	if provider == "ollama" {
		_, model, _, err := ollamaSubtitleSettings(t.App.Config)
		if err != nil {
			return nil, subtitleTranslationStats{}, err
		}
		modelName = model
	} else if t.App.Config.DeepSeekTransConfig != nil && t.App.Config.DeepSeekTransConfig.Model != "" {
		modelName = t.App.Config.DeepSeekTransConfig.Model
	}

	// Cache keys include the provider so equal model labels cannot cross services.
	items, translations, stats := buildSubtitleTranslationQueue(entries, provider+":"+modelName)
	stats.OriginalEntries = len(entries)
	stats.UniqueTexts = len(items)

	cacheKeys := make([]string, 0, len(items))
	for _, item := range items {
		if item.Cacheable {
			cacheKeys = append(cacheKeys, item.CacheKey)
		}
	}
	unlock := acquireSubtitleCacheLocks(cacheKeys)
	defer unlock()

	remaining := items
	if t.DB != nil && len(cacheKeys) > 0 {
		cached, err := t.loadSubtitleTranslationCache(cacheKeys)
		if err != nil {
			t.App.Logger.Warnf("⚠️ 读取字幕翻译缓存失败，将继续调用翻译服务: %v", err)
		} else {
			remaining = make([]subtitleTranslationItem, 0, len(items))
			hitKeys := make([]string, 0)
			for _, item := range items {
				entry, hit := cached[item.CacheKey]
				if !item.Cacheable || !hit || strings.TrimSpace(entry.Translation) == "" {
					remaining = append(remaining, item)
					continue
				}
				stats.CacheHitTexts++
				stats.CacheHitCues += len(item.CueIDs)
				hitKeys = append(hitKeys, item.CacheKey)
				for _, cueID := range item.CueIDs {
					translations[cueID] = entry.Translation
				}
			}
			if len(hitKeys) > 0 {
				t.touchSubtitleTranslationCache(hitKeys)
			}
		}
	}

	if provider == "ollama" {
		t.CheckpointSignature = subtitleCheckpointSignature(entries, provider, modelName)
		t.CheckpointResults = t.loadSubtitleCheckpoint()
		pending := make([]subtitleTranslationItem, 0, len(remaining))
		for _, item := range remaining {
			if value, ok := t.CheckpointResults[item.ID]; ok {
				stats.CacheHitTexts++
				stats.CacheHitCues += len(item.CueIDs)
				for _, cueID := range item.CueIDs {
					translations[cueID] = value
				}
			} else {
				pending = append(pending, item)
			}
		}
		remaining = pending
	}
	stats.RequestedTexts = len(remaining)
	groups := t.buildSubtitleTranslationItemGroups(remaining)
	stats.RequestedGroups = len(groups)
	types.ReportTaskProgress(context, 20, fmt.Sprintf("待翻译%d条唯一字幕，共%d组", len(remaining), len(groups)))

	translatedByID, groupErr := t.translateSubtitleItemGroupsConcurrent(groups, context)
	if groupErr != nil && len(translatedByID) == 0 {
		return nil, stats, groupErr
	}

	cacheRecords := make([]model.TranslationCache, 0, len(remaining))
	now := time.Now()
	for _, item := range remaining {
		translated, ok := translatedByID[item.ID]
		if provider == "ollama" && !isValidatedTranslation(translated) {
			return nil, stats, fmt.Errorf("Ollama 未能翻译队列ID %s", item.ID)
		}
		if !ok || strings.TrimSpace(translated) == "" {
			return nil, stats, fmt.Errorf("翻译结果缺少队列ID %s", item.ID)
		}
		for _, cueID := range item.CueIDs {
			translations[cueID] = translated
		}
		if item.Cacheable && isValidatedTranslation(translated) {
			cacheRecords = append(cacheRecords, model.TranslationCache{
				CacheKey:       item.CacheKey,
				SourceText:     item.Text,
				Translation:    translated,
				SourceLanguage: "en",
				TargetLanguage: "zh-CN",
				Provider:       provider,
				Model:          modelName,
				PromptVersion:  subtitleTranslationPromptVersion,
				LastUsedAt:     now,
			})
		}
	}

	if t.DB != nil && len(cacheRecords) > 0 {
		if err := t.saveSubtitleTranslationCache(cacheRecords); err != nil {
			t.App.Logger.Warnf("⚠️ 保存字幕翻译缓存失败，不影响本次字幕结果: %v", err)
		} else {
			stats.CachedTexts = len(cacheRecords)
		}
	}

	return translations, stats, groupErr
}

// buildSubtitleTranslationQueue deduplicates only the translation queue. The
// original entries and their timeline are never removed or reordered.
func buildSubtitleTranslationQueue(entries []SRTEntry, modelName string) ([]subtitleTranslationItem, map[string]string, subtitleTranslationStats) {
	items := make([]subtitleTranslationItem, 0, len(entries))
	translations := make(map[string]string, len(entries))
	byKey := make(map[string]int, len(entries))
	stats := subtitleTranslationStats{}
	translatableCues := 0

	for _, entry := range entries {
		normalized := normalizeTranslationSourceText(entry.Text)
		if shouldBypassSubtitleTranslation(normalized) {
			translations[entry.CueID] = entry.Text
			stats.BypassedEntries++
			continue
		}
		translatableCues++
		cacheable := isSafeReusableSubtitle(normalized)
		cacheKey := ""
		queueKey := entry.CueID
		if cacheable {
			cacheKey = subtitleTranslationCacheKey(normalized, modelName)
			queueKey = cacheKey
		}
		if index, exists := byKey[queueKey]; exists {
			items[index].CueIDs = append(items[index].CueIDs, entry.CueID)
			continue
		}
		item := subtitleTranslationItem{
			Text:      entry.Text,
			CacheKey:  cacheKey,
			Cacheable: cacheable,
			CueIDs:    []string{entry.CueID},
		}
		byKey[queueKey] = len(items)
		items = append(items, item)
	}

	for i := range items {
		items[i].ID = fmt.Sprintf("T%06d", i+1)
	}
	stats.Deduplicated = translatableCues - len(items)
	return items, translations, stats
}

func normalizeTranslationSourceText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func shouldBypassSubtitleTranslation(text string) bool {
	if text == "" {
		return true
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
		return true
	}
	hasLetter := false
	hasHan := false
	hasNonHanLetter := false
	for _, r := range text {
		if unicode.IsLetter(r) {
			hasLetter = true
			if unicode.Is(unicode.Han, r) {
				hasHan = true
			} else {
				hasNonHanLetter = true
			}
		}
	}
	return !hasLetter || (hasHan && !hasNonHanLetter)
}

// Very short subtitles are context-sensitive (for example "Right.") and are
// deliberately excluded from both cross-position dedupe and persistent cache.
func isSafeReusableSubtitle(text string) bool {
	words := 0
	for _, field := range strings.Fields(text) {
		for _, r := range field {
			if unicode.IsLetter(r) {
				words++
				break
			}
		}
	}
	return words >= 3
}

func subtitleTranslationCacheKey(normalizedText, modelName string) string {
	provider := "deepseek"
	if parts := strings.SplitN(modelName, ":", 2); len(parts) == 2 && (parts[0] == "deepseek" || parts[0] == "ollama") {
		provider, modelName = parts[0], parts[1]
	}
	payload := strings.Join([]string{
		normalizedText,
		"en",
		"zh-CN",
		provider,
		modelName,
		subtitleTranslationPromptVersion,
	}, "\x1f")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func (t *TranslateSubtitle) buildSubtitleTranslationItemGroups(items []subtitleTranslationItem) [][]subtitleTranslationItem {
	groupSize := t.GroupSize
	if groupSize <= 0 {
		groupSize = defaultTranslationGroupSize
	}
	if groupSize > maximumTranslationGroupSize {
		groupSize = maximumTranslationGroupSize
	}

	var groups [][]subtitleTranslationItem
	var current []subtitleTranslationItem
	currentChars := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		group := append([]subtitleTranslationItem(nil), current...)
		groups = append(groups, group)
		current = nil
		currentChars = 0
	}
	for _, item := range items {
		itemChars := len([]rune(item.Text)) + len(item.ID) + 6
		if len(current) > 0 && (len(current) >= groupSize || currentChars+itemChars > maximumTranslationGroupChars) {
			flush()
		}
		current = append(current, item)
		currentChars += itemChars
	}
	flush()
	return groups
}

func (t *TranslateSubtitle) translateSubtitleItemGroupsConcurrent(groups [][]subtitleTranslationItem, context map[string]interface{}) (map[string]string, error) {
	results := make(map[string]string)
	if len(groups) == 0 {
		return results, nil
	}
	if subtitleTranslationProvider(t.App.Config) == "ollama" {
		for index, group := range groups {
			translated, err := t.translateSubtitleItemGroupResilient(group)
			for id, value := range translated {
				if isValidatedTranslation(value) {
					results[id] = value
					t.CheckpointResults[id] = value
				}
			}
			if saveErr := t.saveSubtitleCheckpoint(); saveErr != nil {
				return results, fmt.Errorf("保存 Ollama 翻译断点失败: %w", saveErr)
			}
			t.App.Logger.Infof("Ollama 字幕翻译进度: %d/%d 组", index+1, len(groups))
			types.ReportTaskProgress(context, 20+(index+1)*65/len(groups), fmt.Sprintf("已翻译%d/%d组", index+1, len(groups)))
			if err != nil {
				return results, fmt.Errorf("Ollama 第%d组翻译失败: %w", index+1, err)
			}
			for _, item := range group {
				if !isValidatedTranslation(results[item.ID]) {
					return results, fmt.Errorf("Ollama 第%d组缺少有效译文: %s (%q)", index+1, item.ID, item.Text)
				}
			}
		}
		return results, nil
	}
	type groupResult struct {
		index        int
		translations map[string]string
		err          error
	}
	tasks := make(chan int, len(groups))
	resultChannel := make(chan groupResult, len(groups))
	workerCount := t.MaxWorkers
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(groups) {
		workerCount = len(groups)
	}

	var workers sync.WaitGroup
	for workerID := 0; workerID < workerCount; workerID++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			for groupIndex := range tasks {
				translated, err := t.translateSubtitleItemGroupResilient(groups[groupIndex])
				resultChannel <- groupResult{index: groupIndex, translations: translated, err: err}
			}
		}(workerID)
	}
	for i := range groups {
		tasks <- i
	}
	close(tasks)
	go func() {
		workers.Wait()
		close(resultChannel)
	}()

	completed := 0
	var lastErr error
	for result := range resultChannel {
		completed++
		if result.err != nil {
			lastErr = result.err
			t.App.Logger.Warnf("⚠️ A+翻译第%d组发生降级: %v", result.index+1, result.err)
		}
		for id, translation := range result.translations {
			results[id] = translation
		}
		progress := 20 + completed*65/len(groups)
		if progress > 85 {
			progress = 85
		}
		types.ReportTaskProgress(context, progress, fmt.Sprintf("已翻译%d/%d组", completed, len(groups)))
	}
	if lastErr != nil {
		for _, group := range groups {
			for _, item := range group {
				if _, ok := results[item.ID]; !ok {
					return results, lastErr
				}
			}
		}
	}
	// Every queue ID has a value. Any per-item fallback is deliberately not
	// cached, but it still preserves the original CueID and timeline.
	return results, nil
}

func (t *TranslateSubtitle) translateSubtitleItemGroupResilient(items []subtitleTranslationItem) (map[string]string, error) {
	translated, err := t.translateSubtitleItemGroupSimple(items)
	// Simple translation failures (for example malformed/non-JSON model output)
	// return a nil result map. The resilient path still needs to add fallback or
	// recursively recovered values, so always make the accumulator writable.
	translated = ensureSubtitleTranslationMap(translated)
	missing := missingSubtitleTranslationItems(items, translated)
	if err == nil && len(missing) == 0 {
		return translated, nil
	}
	if err != nil && t.isFatalTranslationError(err) {
		return translated, err
	}
	if err == nil {
		err = fmt.Errorf("翻译响应缺少%d个队列ID", len(missing))
	}
	if len(missing) == 0 {
		return translated, err
	}
	if len(missing) == 1 {
		item := missing[0]
		if subtitleTranslationProvider(t.App.Config) == "ollama" {
			for attempt := 0; attempt < 3; attempt++ {
				retry, retryErr := t.translateSubtitleItemGroupSimple([]subtitleTranslationItem{item})
				if retryErr == nil && isValidatedTranslation(retry[item.ID]) {
					translated[item.ID] = retry[item.ID]
					return translated, nil
				}
			}
			return translated, fmt.Errorf("本机 Ollama 未返回字幕 %s 的有效译文，原文=%q: %w", item.ID, item.Text, err)
		}
		translated[item.ID] = fmt.Sprintf("[翻译失败] %s", item.Text)
		return translated, err
	}

	middle := len(missing) / 2
	left, leftErr := t.translateSubtitleItemGroupResilient(missing[:middle])
	right, rightErr := t.translateSubtitleItemGroupResilient(missing[middle:])
	for id, value := range left {
		translated[id] = value
	}
	for id, value := range right {
		translated[id] = value
	}
	if rightErr != nil {
		return translated, rightErr
	}
	if leftErr != nil {
		return translated, leftErr
	}
	if subtitleTranslationProvider(t.App.Config) == "ollama" && len(missingSubtitleTranslationItems(items, translated)) == 0 {
		return translated, nil
	}
	return translated, err
}

func ensureSubtitleTranslationMap(translated map[string]string) map[string]string {
	if translated == nil {
		return make(map[string]string)
	}
	return translated
}

func (t *TranslateSubtitle) translateSubtitleItemGroupSimple(items []subtitleTranslationItem) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	payload := make(map[string]string, len(items))
	for _, item := range items {
		payload[item.ID] = item.Text
	}
	userPromptBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化字幕翻译请求失败: %w", err)
	}
	systemPrompt := "将JSON对象中的英文视频字幕翻译成自然、简洁、准确的简体中文。保留语气、人名和专有名词，不合并、不遗漏、不解释。仅返回JSON对象；键必须与输入完全相同，值为对应译文。"
	response, err := t.callSubtitleTranslationAPI(systemPrompt, string(userPromptBytes))
	if err != nil {
		return nil, err
	}
	parsed, err := parseSubtitleTranslationJSON(response)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]string, len(items))
	allowed := make(map[string]struct{}, len(items))
	for _, item := range items {
		allowed[item.ID] = struct{}{}
	}
	for id, translation := range parsed {
		if _, ok := allowed[id]; ok && strings.TrimSpace(translation) != "" {
			valid[id] = strings.TrimSpace(translation)
		}
	}
	return valid, nil
}

func parseSubtitleTranslationJSON(response string) (map[string]string, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```JSON")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(strings.TrimSpace(response), "```")
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("翻译响应不是JSON对象")
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(response[start:end+1]), &result); err != nil {
		return nil, fmt.Errorf("解析翻译JSON失败: %w", err)
	}
	return result, nil
}

func missingSubtitleTranslationItems(items []subtitleTranslationItem, translated map[string]string) []subtitleTranslationItem {
	missing := make([]subtitleTranslationItem, 0)
	for _, item := range items {
		value, ok := translated[item.ID]
		if !ok || strings.TrimSpace(value) == "" {
			missing = append(missing, item)
		}
	}
	return missing
}

func isValidatedTranslation(translation string) bool {
	translation = strings.TrimSpace(translation)
	return translation != "" &&
		!strings.HasPrefix(translation, "[翻译失败]") &&
		!strings.Contains(strings.ToLower(translation), "rate limit") &&
		!strings.Contains(strings.ToLower(translation), "api error")
}

func (t *TranslateSubtitle) loadSubtitleTranslationCache(keys []string) (map[string]model.TranslationCache, error) {
	result := make(map[string]model.TranslationCache)
	if t.DB == nil || len(keys) == 0 {
		return result, nil
	}
	var entries []model.TranslationCache
	if err := t.DB.Where("cache_key IN ?", uniqueStrings(keys)).Find(&entries).Error; err != nil {
		return nil, err
	}
	for _, entry := range entries {
		result[entry.CacheKey] = entry
	}
	return result, nil
}

func (t *TranslateSubtitle) touchSubtitleTranslationCache(keys []string) {
	if t.DB == nil || len(keys) == 0 {
		return
	}
	if err := t.DB.Model(&model.TranslationCache{}).
		Where("cache_key IN ?", uniqueStrings(keys)).
		Updates(map[string]interface{}{
			"hit_count":    gorm.Expr("hit_count + 1"),
			"last_used_at": time.Now(),
		}).Error; err != nil {
		t.App.Logger.Warnf("⚠️ 更新字幕缓存命中统计失败: %v", err)
	}
}

func (t *TranslateSubtitle) saveSubtitleTranslationCache(entries []model.TranslationCache) error {
	if t.DB == nil || len(entries) == 0 {
		return nil
	}
	return t.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cache_key"}},
		DoNothing: true,
	}).CreateInBatches(entries, 100).Error
}

func acquireSubtitleCacheLocks(keys []string) func() {
	keys = uniqueStrings(keys)
	sort.Strings(keys)
	locks := make([]*subtitleCacheKeyLock, 0, len(keys))
	subtitleCacheLockRegistry.Lock()
	for _, key := range keys {
		lock := subtitleCacheLockRegistry.locks[key]
		if lock == nil {
			lock = &subtitleCacheKeyLock{}
			subtitleCacheLockRegistry.locks[key] = lock
		}
		lock.refs++
		locks = append(locks, lock)
	}
	subtitleCacheLockRegistry.Unlock()

	for _, lock := range locks {
		lock.mutex.Lock()
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].mutex.Unlock()
		}
		subtitleCacheLockRegistry.Lock()
		for i, key := range keys {
			locks[i].refs--
			if locks[i].refs == 0 {
				delete(subtitleCacheLockRegistry.locks, key)
			}
		}
		subtitleCacheLockRegistry.Unlock()
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func generateTranslatedSRTContentByCue(entries []SRTEntry, translated map[string]string) string {
	var builder strings.Builder
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("%d\n", entry.Index))
		builder.WriteString(entry.TimeCode)
		builder.WriteByte('\n')
		builder.WriteString(translated[entry.CueID])
		builder.WriteString("\n\n")
	}
	return builder.String()
}
