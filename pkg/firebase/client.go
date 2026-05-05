package firebase

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client Firebase Backend SDK 客户端
type Client struct {
	BaseURL    string
	AppID      string
	AppSecret  string
	HTTPClient *http.Client
}

// NewClient 创建 SDK 客户端
func NewClient(baseURL, appID, appSecret string) *Client {
	return &Client{
		BaseURL:   baseURL,
		AppID:     appID,
		AppSecret: appSecret,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UserProfile 用户信息
type UserProfile struct {
	UID         string      `json:"uid"`
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	VIPStatus   *VIPStatus  `json:"vip_status"`
	Power       int         `json:"power"`
	CreatedAt   time.Time   `json:"created_at"`
}

// VIPStatus VIP状态
type VIPStatus struct {
	IsVIP      bool       `json:"is_vip"`
	Tier       string     `json:"tier"`
	ExpireTime *time.Time `json:"expire_time,omitempty"`
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	UserID      string `json:"user_id"`
	ProductID   string `json:"product_id"`
	PayWay      string `json:"pay_way"`      // alipay, wechat, paypal, mock
	PayType     string `json:"pay_type"`     // h5, pc, native
	ReturnURL   string `json:"return_url"`   // 支付成功跳转地址
	CallbackURL string `json:"callback_url"` // 第三方回调地址
	Extra       string `json:"extra"`        // 自定义数据
}

// OrderResponse 订单响应
type OrderResponse struct {
	OrderNo   string    `json:"order_no"`
	Amount    float64   `json:"amount"`
	Product   string    `json:"product"`
	CreatedAt time.Time `json:"created_at"`
}

// OrderStatus 订单状态
type OrderStatus struct {
	OrderNo   string     `json:"order_no"`
	Status    string     `json:"status"`
	Amount    float64    `json:"amount"`
	UserID    string     `json:"user_id"`
	PayWay    string     `json:"pay_way"`
	PaidAt    *time.Time `json:"paid_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APIResponse 统一API响应格式
type APIResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// GetUserProfile 获取用户信息
func (c *Client) GetUserProfile(uid string) (*UserProfile, error) {
	path := fmt.Sprintf("/api/third-party/users/%s", uid)

	var result UserProfile
	if err := c.request("GET", path, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetVIPStatus 获取VIP状态
func (c *Client) GetVIPStatus(uid string) (*VIPStatus, error) {
	path := fmt.Sprintf("/api/third-party/users/%s/vip-status", uid)

	var response struct {
		IsVIP     bool       `json:"is_vip"`
		VIPStatus *VIPStatus `json:"vip_status"`
	}
	if err := c.request("GET", path, nil, &response); err != nil {
		return nil, err
	}

	return response.VIPStatus, nil
}

// CreateOrder 创建订单
func (c *Client) CreateOrder(req *CreateOrderRequest) (*OrderResponse, error) {
	path := "/api/third-party/orders"

	var result OrderResponse
	if err := c.request("POST", path, req, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetOrderStatus 查询订单状态
func (c *Client) GetOrderStatus(orderNo string) (*OrderStatus, error) {
	path := fmt.Sprintf("/api/third-party/orders/%s", orderNo)

	var result OrderStatus
	if err := c.request("GET", path, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetUserOrders 获取用户订单列表
func (c *Client) GetUserOrders(uid string, limit int) ([]OrderStatus, error) {
	path := fmt.Sprintf("/api/third-party/users/%s/orders?limit=%d", uid, limit)

	var result []OrderStatus
	if err := c.request("GET", path, nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// request 发送请求
func (c *Client) request(method, path string, body interface{}, result interface{}) error {
	var bodyData []byte
	var bodyReader io.Reader

	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewBuffer(bodyData)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// 添加认证头（符合 go-auth 标准）
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := generateNonce()
	signature := c.generateSignature(timestamp, nonce, path, bodyData)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Id", c.AppID)     // go-auth 使用 X-App-Id
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Sign", signature)     // go-auth 使用 X-Sign

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// 解析统一响应格式
	var apiResp APIResponse
	if err := json.Unmarshal(responseBody, &apiResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Code != 0 {
		return fmt.Errorf("API error (code=%d): %s", apiResp.Code, apiResp.Msg)
	}

	// 解析实际数据
	if result != nil {
		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}

// generateSignature 生成签名
// 与 go-auth 库保持一致的签名算法
// 签名格式: HMAC-SHA256(timestamp + nonce + path + body, app_secret)
func (c *Client) generateSignature(timestamp, nonce, path string, body []byte) string {
	// go-auth 的签名格式
	data := timestamp + nonce + path
	if len(body) > 0 {
		data += string(body)
	}
	h := hmac.New(sha256.New, []byte(c.AppSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// generateNonce 生成随机数
func generateNonce() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
