package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/difyz9/ytb2bili/internal/bilibiliutil"
	"github.com/difyz9/ytb2bili/internal/chain_task/base"
	"github.com/difyz9/ytb2bili/internal/chain_task/manager"
	"github.com/difyz9/ytb2bili/internal/core"
	"github.com/difyz9/ytb2bili/internal/core/services"
	"github.com/difyz9/ytb2bili/internal/core/types"
	"github.com/difyz9/ytb2bili/internal/storage"
	"github.com/difyz9/ytb2bili/pkg/cos"
)

const (
	bilibiliCommentTypeVideo = "1"
	bilibiliCommentPlatform  = "1"
	bilibiliPinMaxAttempts   = 5
)

type PublishCommentHandler struct {
	base.BaseTask
	App               *core.AppServer
	SavedVideoService *services.SavedVideoService
}

type bilibiliCommentAPIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	TTL     int             `json:"ttl,omitempty"`
}

type addCommentResponseData struct {
	RPID  int64 `json:"rpid"`
	Reply struct {
		RPID int64 `json:"rpid"`
	} `json:"reply"`
}

type bilibiliAPIError struct {
	Code    int
	Message string
}

func (e *bilibiliAPIError) Error() string {
	return fmt.Sprintf("Bilibili API error: code=%d, message=%s", e.Code, e.Message)
}

func NewPublishCommentHandler(name string, app *core.AppServer, stateManager *manager.StateManager, client *cos.CosClient, savedVideoService *services.SavedVideoService) *PublishCommentHandler {
	return &PublishCommentHandler{
		BaseTask: base.BaseTask{
			Name:         name,
			StateManager: stateManager,
			Client:       client,
		},
		App:               app,
		SavedVideoService: savedVideoService,
	}
}

func (t *PublishCommentHandler) Execute(context map[string]interface{}) bool {
	t.App.Logger.Info("========================================")
	t.App.Logger.Info("开始发布 Bilibili 章节置顶评论")
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 5, "检查章节评论内容")

	var savedVideoChapters string
	var savedVideoBVID string
	var savedVideoAID int64
	if t.SavedVideoService != nil {
		savedVideo, err := t.SavedVideoService.GetVideoByVideoID(t.StateManager.VideoID)
		if err != nil {
			t.App.Logger.Warnf("⚠️ 无法从数据库获取视频信息，将仅使用 context: %v", err)
		} else if savedVideo != nil {
			savedVideoChapters = strings.TrimSpace(savedVideo.Chapters)
			savedVideoBVID = strings.TrimSpace(savedVideo.BiliBVID)
			savedVideoAID = savedVideo.BiliAID
		}
	}

	chapters := getContextString(context, "bili_chapters")
	if chapters == "" {
		chapters = savedVideoChapters
		if chapters != "" {
			context["bili_chapters"] = chapters
		}
	}
	if strings.TrimSpace(chapters) == "" {
		t.App.Logger.Info("ℹ️ 未找到章节内容，跳过发布置顶评论")
		return true
	}

	bvid := getContextString(context, "bili_bvid")
	if bvid == "" {
		bvid = savedVideoBVID
		if bvid != "" {
			context["bili_bvid"] = bvid
		}
	}
	if strings.TrimSpace(bvid) == "" {
		errMsg := "没有找到 BVID，无法发布章节评论"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	aid, ok := getContextInt64(context, "bili_aid")
	if (!ok || aid <= 0) && savedVideoAID > 0 {
		aid = savedVideoAID
		ok = true
		context["bili_aid"] = aid
	}
	if !ok || aid <= 0 {
		errMsg := "没有找到有效的 AID，无法发布章节评论"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	t.App.Logger.Infof("📺 准备发布章节评论: BVID=%s, AID=%d, 内容长度=%d 字符", bvid, aid, len([]rune(chapters)))
	types.ReportTaskProgress(context, 20, "检查Bilibili登录")

	loginStore := storage.GetDefaultStore()
	if !loginStore.IsValid() {
		errMsg := "未登录 Bilibili，无法发布章节评论"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	loginInfo, err := loginStore.Load()
	if err != nil {
		errMsg := fmt.Sprintf("加载 Bilibili 登录信息失败: %v", err)
		t.App.Logger.Errorf("❌ %s", errMsg)
		context["error"] = errMsg
		return false
	}

	if cookieStr := loginInfo.GetCookieString(); strings.TrimSpace(cookieStr) == "" {
		errMsg := "Bilibili 登录 Cookie 为空，无法发布章节评论"
		t.App.Logger.Error("❌ " + errMsg)
		context["error"] = errMsg
		return false
	}

	if _, err := loginInfo.GetCSRFToken(); err != nil {
		errMsg := fmt.Sprintf("获取 Bilibili CSRF token 失败: %v", err)
		t.App.Logger.Errorf("❌ %s", errMsg)
		context["error"] = errMsg
		return false
	}

	client := bilibili.NewClient(bilibiliutil.BuildOptions(t.App.Config, 2*time.Minute)...)

	types.ReportTaskProgress(context, 45, "发布章节评论")
	rpid, err := t.addComment(client, loginInfo, aid, bvid, chapters)
	if err != nil {
		context["bili_chapters_comment_published"] = false
		context["bili_chapters_comment_error"] = err.Error()
		t.App.Logger.Warnf("⚠️ 发布章节评论失败，不阻断字幕上传任务: %v", err)
		t.App.Logger.Info("========================================")
		types.ReportTaskProgress(context, 100, "字幕已完成，章节评论发布失败")
		return true
	}

	context["bili_chapters_rpid"] = rpid
	context["bili_chapters_comment_published"] = true
	t.App.Logger.Infof("✅ 章节评论发布成功: RPID=%d", rpid)

	types.ReportTaskProgress(context, 75, "置顶章节评论")
	if err := t.pinCommentWithRetry(client, loginInfo, aid, bvid, rpid); err != nil {
		context["bili_chapters_pinned"] = false
		context["bili_chapters_pin_error"] = err.Error()
		t.App.Logger.Warnf("⚠️ 章节评论已发布，但置顶失败，不阻断后续任务: %v", err)
		t.App.Logger.Info("========================================")
		types.ReportTaskProgress(context, 100, "章节评论已发布，置顶失败")
		return true
	}

	context["bili_chapters_pinned"] = true
	t.App.Logger.Infof("📌 章节评论已置顶: RPID=%d", rpid)
	t.App.Logger.Info("========================================")
	types.ReportTaskProgress(context, 100, "章节评论已置顶")

	return true
}

func (t *PublishCommentHandler) addComment(client *bilibili.Client, loginInfo *bilibili.LoginInfo, aid int64, bvid, message string) (int64, error) {
	csrf, err := loginInfo.GetCSRFToken()
	if err != nil {
		return 0, fmt.Errorf("get CSRF token failed: %w", err)
	}

	form := url.Values{}
	form.Set("oid", strconv.FormatInt(aid, 10))
	form.Set("type", bilibiliCommentTypeVideo)
	form.Set("message", message)
	form.Set("plat", bilibiliCommentPlatform)
	form.Set("csrf", csrf)
	form.Set("csrf_token", csrf)

	var response addCommentResponseData
	if err := t.postBilibiliForm(client, loginInfo, "https://api.bilibili.com/x/v2/reply/add", form, bvid, &response); err != nil {
		return 0, err
	}

	if response.RPID > 0 {
		return response.RPID, nil
	}
	if response.Reply.RPID > 0 {
		return response.Reply.RPID, nil
	}

	return 0, fmt.Errorf("comment response missing rpid")
}

func (t *PublishCommentHandler) pinCommentWithRetry(client *bilibili.Client, loginInfo *bilibili.LoginInfo, aid int64, bvid string, rpid int64) error {
	var lastErr error

	for attempt := 1; attempt <= bilibiliPinMaxAttempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt*2) * time.Second
			t.App.Logger.Infof("⏳ 等待 %s 后重试置顶章节评论: attempt=%d/%d", delay, attempt, bilibiliPinMaxAttempts)
			time.Sleep(delay)
		}

		if err := t.pinComment(client, loginInfo, aid, bvid, rpid); err != nil {
			lastErr = err
			t.App.Logger.Warnf("⚠️ 置顶章节评论失败: attempt=%d/%d, err=%v", attempt, bilibiliPinMaxAttempts, err)
			if !isRetryablePinCommentError(err) {
				return err
			}
			continue
		}

		return nil
	}

	return lastErr
}

func (t *PublishCommentHandler) pinComment(client *bilibili.Client, loginInfo *bilibili.LoginInfo, aid int64, bvid string, rpid int64) error {
	csrf, err := loginInfo.GetCSRFToken()
	if err != nil {
		return fmt.Errorf("get CSRF token failed: %w", err)
	}

	form := url.Values{}
	form.Set("oid", strconv.FormatInt(aid, 10))
	form.Set("type", bilibiliCommentTypeVideo)
	form.Set("rpid", strconv.FormatInt(rpid, 10))
	form.Set("action", "1")
	form.Set("csrf", csrf)
	form.Set("csrf_token", csrf)

	return t.postBilibiliForm(client, loginInfo, "https://api.bilibili.com/x/v2/reply/top", form, bvid, nil)
}

func isRetryablePinCommentError(err error) bool {
	var apiErr *bilibiliAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case -404, -509, 12006:
			return true
		default:
			return false
		}
	}

	errText := strings.ToLower(err.Error())
	return strings.Contains(errText, "timeout") ||
		strings.Contains(errText, "connection reset") ||
		strings.Contains(errText, "eof")
}

func (t *PublishCommentHandler) postBilibiliForm(client *bilibili.Client, loginInfo *bilibili.LoginInfo, endpoint string, form url.Values, bvid string, data interface{}) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", loginInfo.GetCookieString())
	req.Header.Set("Origin", "https://www.bilibili.com")
	req.Header.Set("Referer", fmt.Sprintf("https://www.bilibili.com/video/%s/", bvid))
	req.Header.Set("User-Agent", bilibili.DefaultConfig().UserAgent)

	resp, err := client.GetHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, trimResponseBody(body))
	}

	var apiResp bilibiliCommentAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("unmarshal response failed: %w", err)
	}

	if apiResp.Code != 0 {
		return &bilibiliAPIError{
			Code:    apiResp.Code,
			Message: apiResp.Message,
		}
	}

	if data == nil || len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil
	}

	if err := json.Unmarshal(apiResp.Data, data); err != nil {
		return fmt.Errorf("unmarshal response data failed: %w", err)
	}

	return nil
}

func getContextString(context map[string]interface{}, key string) string {
	if context == nil {
		return ""
	}

	value, ok := context[key]
	if !ok || value == nil {
		return ""
	}

	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}

	return strings.TrimSpace(fmt.Sprint(value))
}

func getContextInt64(context map[string]interface{}, key string) (int64, bool) {
	if context == nil {
		return 0, false
	}

	value, ok := context[key]
	if !ok || value == nil {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed, err == nil
	default:
		parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(v)), 10, 64)
		return parsed, err == nil
	}
}

func trimResponseBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len([]rune(text)) <= 300 {
		return text
	}

	runes := []rune(text)
	return string(runes[:300]) + "..."
}
