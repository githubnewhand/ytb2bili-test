package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

// DecryptCookies 解密 cookies 中间件
// 从请求体的 meta 字段提取加密的 cookies，解密后存入 context
func DecryptCookies(decryptKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("🔐 开始解密 cookies...\n")
		
		// 只处理 POST 请求
		if c.Request.Method != "POST" && c.Request.Method != "PUT" {
			c.Next()
			return
		}
		
		// 读取请求体
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			fmt.Printf("⚠️ 读取请求体失败: %v\n", err)
			c.Next()
			return
		}
		
		// 如果请求体为空，直接跳过
		if len(bodyBytes) == 0 {
			c.Next()
			return
		}
		
		fmt.Printf("📦 请求体长度: %d bytes\n", len(bodyBytes))
		
		// 恢复请求体供后续使用
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		
		// 解析请求体，查找 meta 字段
		var data map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &data); err != nil {
			fmt.Printf("⚠️ 解析请求体 JSON 失败: %v\n", err)
			c.Next()
			return
		}

		// 如果存在 meta 字段，尝试解密
		if encryptedCookies, ok := data["meta"].(string); ok && encryptedCookies != "" {
			fmt.Printf("🔍 发现加密的 cookies 字段 (meta)，长度: %d\n", len(encryptedCookies))
			decryptedCookies, err := decryptData(encryptedCookies, decryptKey)
			if err != nil {
				fmt.Printf("❌ 解密 cookies 失败: %v\n", err)
			} else {
				// 将解密后的 cookies 存储到 context 中
				c.Set("decryptedCookies", decryptedCookies)
				fmt.Printf("✅ Cookies 已成功解密，长度: %d 字符\n", len(decryptedCookies))
			}
		} else {
			fmt.Printf("⚠️ 未找到 meta 字段，data keys: %v\n", func() []string {
				keys := make([]string, 0, len(data))
				for k := range data {
					keys = append(keys, k)
				}
				return keys
			}())
		}
		
		c.Next()
	}
}

// decryptData 使用 AES-GCM 解密数据
func decryptData(encryptedBase64 string, keyStr string) (string, error) {
	// 解析 Base64
	combined, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}

	// 准备密钥（确保32字节）
	key := []byte(strings.Repeat(keyStr+"0", 32)[:32])

	// 创建 cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 cipher 失败: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 失败: %w", err)
	}

	// 提取 IV 和密文
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return "", fmt.Errorf("密文长度不足")
	}

	nonce := combined[:nonceSize]
	ciphertext := combined[nonceSize:]

	// 解密
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}

	return string(plaintext), nil
}
