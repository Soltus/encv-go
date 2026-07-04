// internal/auth/auth.go
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/routes"
)

// JWT Claims 结构
type Claims struct {
	SessionID string `json:"sid"` // 会话ID，用于撤销特定会话
	IssuedAt  int64  `json:"iat"` // 签发时间
	ExpiresAt int64  `json:"exp"` // 过期时间
}

// JWTManager JWT认证管理器
type JWTManager struct {
	secretKey string // HMAC密钥
	duration  time.Duration
}

// NewJWTManager 创建JWT管理器
func NewJWTManager(password string, duration time.Duration) *JWTManager {
	// 使用密码的SHA256作为密钥
	h := hmac.New(sha256.New, []byte(password))
	h.Write([]byte("encv-jwt-secret"))
	secretKey := base64.URLEncoding.EncodeToString(h.Sum(nil))

	return &JWTManager{
		secretKey: secretKey,
		duration:  duration,
	}
}

func (m *JWTManager) IsLoggedIn(authHeader string) bool {
	isLoggedIn := false
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		tokenString := authHeader[7:] // 去掉 "Bearer " 前缀
		if _, err := m.ValidateToken(tokenString); err == nil {
			isLoggedIn = true
		}
	}
	return isLoggedIn
}

// CreateToken 创建JWT token
func (m *JWTManager) CreateToken() (string, error) {
	now := time.Now()
	sessionID := generateSessionID()

	claims := Claims{
		SessionID: sessionID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.duration).Unix(),
	}

	// 创建header
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}

	// 编码header
	headerBytes, _ := json.Marshal(header)
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerBytes)

	// 编码payload
	payloadBytes, _ := json.Marshal(claims)
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// 创建签名
	message := headerEncoded + "." + payloadEncoded
	signature := m.sign(message)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	// 组合JWT
	return message + "." + signatureEncoded, nil
}

// ValidateToken 验证JWT token
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// 验证签名
	message := parts[0] + "." + parts[1]
	expectedSignature := m.sign(message)
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid signature")
	}

	// 解析payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload format")
	}

	// 检查过期时间
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// sign 创建HMAC-SHA256签名
func (m *JWTManager) sign(message string) []byte {
	h := hmac.New(sha256.New, []byte(m.secretKey))
	h.Write([]byte(message))
	return h.Sum(nil)
}

// generateSessionID 生成随机会话ID
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random session ID: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// SetAuthCookie 设置认证Cookie
func SetAuthCookie(w http.ResponseWriter, token string, duration time.Duration) {
	maxAge := int(duration.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     "encv_auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge, // 设置过期时间，浏览器关闭后仍然有效
		Secure:   false,  // 开发环境设为false，生产环境应为true（需要HTTPS）
	})
}

// ClearAuthCookie 清除认证Cookie
func ClearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "encv_auth_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// GetTokenFromCookie 从Cookie获取token
func GetTokenFromCookie(r *http.Request) string {
	if cookie, err := r.Cookie("encv_auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

// SetRedirectCookie 设置重定向Cookie
func SetRedirectCookie(w http.ResponseWriter, redirectURL string) {
	if strings.Contains(redirectURL, routes.Login) ||
		strings.Contains(redirectURL, routes.Logout) {
		return
	}

	if redirectURL == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "encv_redirect_url",
		Value:    redirectURL,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300,
	})
}

// GetRedirectCookie 获取重定向URL
func GetRedirectCookie(r *http.Request) string {
	if cookie, err := r.Cookie("encv_redirect_url"); err == nil {
		redirectURL := cookie.Value
		if strings.Contains(redirectURL, routes.Login) ||
			strings.Contains(redirectURL, routes.Logout) {
			return ""
		}
		return redirectURL
	}
	return ""
}

// ClearRedirectCookie 清除重定向Cookie
func ClearRedirectCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "encv_redirect_url",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
