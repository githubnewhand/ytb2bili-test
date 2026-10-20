package handlers

import (
	"errors"
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
