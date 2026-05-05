package resp

// * +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
// * Copyright 2023 The Geek-AI Authors. All rights reserved.
// * Use of this source code is governed by a Apache-2.0 license
// * that can be found in the LICENSE file.
// * @Author yangjian102621@163.com
// * +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

import (
	"math"

	"github.com/gin-gonic/gin"
	"net/http"
)

// Response 通用响应结构
type Response struct {
	Code    int         `json:"code" example:"200"`        // 状态码
	Message string      `json:"message" example:"success"` // 消息
	Data    interface{} `json:"data,omitempty"`            // 数据
}

// PaginationResponse 通用分页响应结构体
type PaginationResponse[T any] struct {
	Message    string `json:"message,omitempty"`
	Code       int    `json:"code,omitempty"`
	Data       []T    `json:"data,omitempty"`
	Total      int64  `json:"total,omitempty"`
	PageNum    int    `json:"pageNum,omitempty"`
	PageSize   int    `json:"pageSize,omitempty"`
	TotalPages int64  `json:"totalPages,omitempty"`
}

// NewPaginationResponse 创建分页响应
func NewPaginationResponse[T any](data []T, total int64, pageNum, pageSize int) PaginationResponse[T] {
	totalPages := int64(math.Ceil(float64(total) / float64(pageSize)))
	return PaginationResponse[T]{
		Message:    "success",
		Code:       200,
		Data:       data,
		Total:      total,
		PageNum:    pageNum,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

func SuccessWithPagination[T any](c *gin.Context, data []T, total int64, pageNum, pageSize int) {

	if data == nil || len(data) == 0 {
		data = []T{} // 如果 data 为空或长度为 0，则返回默认空数组
		total = 0    // 默认 total 为 0

		c.JSON(http.StatusOK, gin.H{
			"message":    "success",
			"code":       200,
			"data":       []T{},
			"total":      0,
			"pageNum":    pageNum,
			"pageSize":   pageSize,
			"totalPages": 1, // 如果没有数据，totalPages 也设置为 1
		})
	} else {
		totalPages := int64(math.Ceil(float64(total) / float64(pageSize)))

		c.JSON(http.StatusOK, PaginationResponse[T]{
			Message:    "success",
			Code:       200,
			Data:       data,
			Total:      total,
			PageNum:    pageNum,
			PageSize:   pageSize,
			TotalPages: totalPages,
		})
	}

}

// Success 成功响应
func Success(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: message,
		Data:    data,
	})
}

// Fail 失败响应
func Fail(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, Response{
		Code:    statusCode,
		Message: message,
		Data:    data,
	})
}
