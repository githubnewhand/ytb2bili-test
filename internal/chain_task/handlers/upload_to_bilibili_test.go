package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestShouldRetryUploadErrorRecognizesLocalizedTransientFailure(t *testing.T) {
	handler := &UploadToBilibili{}
	for _, message := range []string{
		"操作执行失败，请稍后重试",
		"系统繁忙，请稍后再试",
		"操作频繁",
	} {
		if !handler.shouldRetryUploadError(errors.New(message)) {
			t.Fatalf("localized transient error should be retryable: %q", message)
		}
	}
}

func TestShouldRetryUploadErrorRejectsPermanentPermissionFailure(t *testing.T) {
	handler := &UploadToBilibili{}
	for _, message := range []string{
		"访问权限不足",
		"账号未登录",
		"图片格式不支持",
	} {
		if handler.shouldRetryUploadError(errors.New(message)) {
			t.Fatalf("permanent error must fail fast: %q", message)
		}
	}
}

func TestUploadStatusDoesNotMatchURLDigits(t *testing.T) {
	handler := &UploadToBilibili{}
	for _, tt := range []struct{ err, want string }{
		{`failed to upload chunks: Put "https://upload/video?total=511034299&uploadId=401403500": http2: PROTOCOL_ERROR`, "视频分片上传中断"},
		{"upload chunk 1 failed with status 500 Internal Server Error", "B站服务器临时异常"},
		{"upload failed: status 429 Too Many Requests", "操作频率过快"},
		{"request failed: code=401", "登录状态已过期"},
	} {
		got := handler.getUserFriendlyError(fmt.Errorf("%s", tt.err), "上传视频")
		if !strings.Contains(got, tt.want) {
			t.Fatalf("%q => %q, want %q", tt.err, got, tt.want)
		}
	}
}
