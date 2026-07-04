// internal/admin/logic/openlist/token_manager.go
package openlist

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/crypto"
)

// 【新增】常量定义
const (
	TokenFileNamePrefix = "openlist."
	TokenFileExtension  = ".token"
	TokenFilePerms      = 0600
)

// TokenManager 管理 OpenList 站点的 token 会话
type TokenManager struct {
	tokens    map[string]SiteToken // siteId -> token info
	cfg       *config.Config       // 配置
	mutex     sync.RWMutex
	configDir string // 配置文件目录
}

type SiteToken struct {
	Token     string
	ExpiresAt time.Time
}

func NewTokenManager(ctx context.Context) *TokenManager {
	cfg := config.FromContext(ctx)

	// 【新增】获取配置目录
	configFile, err := config.FindConfigPath("")
	configDir := filepath.Dir(configFile)
	if err != nil {
		panic(err)
	}
	if configDir == "" {
		configDir = "." // 如果找不到配置目录，使用当前目录
	}

	tm := &TokenManager{
		tokens:    make(map[string]SiteToken),
		cfg:       cfg,
		configDir: configDir,
	}

	// 【新增】启动时加载所有已保存的 token
	tm.loadAllTokens()

	// 启动清理任务
	go tm.cleanupExpired()

	return tm
}

// SetToken 设置站点 token，30天有效期
func (tm *TokenManager) SetToken(siteId, token string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	siteToken := SiteToken{
		Token:     token,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}

	// 更新内存
	tm.tokens[siteId] = siteToken

	// 【新增】保存到加密文件
	if err := tm.saveTokenToFile(siteId, siteToken); err != nil {
		// 保存失败不影响内存中的 token，但记录错误
		// 这里可以使用日志记录
	}
}

// GetToken 获取站点 token
func (tm *TokenManager) GetToken(siteId string) (string, bool) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	token, exists := tm.tokens[siteId]
	if !exists || time.Now().After(token.ExpiresAt) {
		return "", false
	}
	return token.Token, true
}

// RemoveToken 移除站点 token
func (tm *TokenManager) RemoveToken(siteId string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 从内存中删除
	delete(tm.tokens, siteId)

	// 【新增】删除文件
	tokenFile := tm.getTokenFilePath(siteId)
	os.Remove(tokenFile) // 忽略错误
}

// 【新增】saveTokenToFile 加密保存 token 到文件
func (tm *TokenManager) saveTokenToFile(siteId string, siteToken SiteToken) error {
	// 序列化 token
	tokenData, err := json.Marshal(siteToken)
	if err != nil {
		return err
	}

	// 生成加密密钥（使用 cfg.Password）
	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		return err
	}
	key := crypto.GenerateKey(tm.cfg.Password, salt, crypto.KeySize_v2)

	// 生成 IV
	iv, err := crypto.GenerateIV_v2(crypto.IVSize_v2)
	if err != nil {
		return err
	}

	// 加密数据
	encryptedData, err := crypto.EncryptBytes_v2(tokenData, key, iv)
	if err != nil {
		return err
	}

	// 组合：salt + iv + encryptedData
	fileData := append(salt, iv...)
	fileData = append(fileData, encryptedData...)

	// 写入文件
	tokenFile := tm.getTokenFilePath(siteId)
	return os.WriteFile(tokenFile, fileData, TokenFilePerms)
}

// 【新增】loadTokenFromFile 从加密文件加载 token
func (tm *TokenManager) loadTokenFromFile(siteId string) (*SiteToken, error) {
	tokenFile := tm.getTokenFilePath(siteId)

	// 读取文件
	fileData, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}

	// 检查最小长度（salt + iv + 至少一些数据）
	minLength := crypto.SaltSize_v2 + crypto.IVSize_v2
	if len(fileData) < minLength {
		return nil, os.ErrInvalid
	}

	// 分解：salt + iv + encryptedData
	salt := fileData[:crypto.SaltSize_v2]
	iv := fileData[crypto.SaltSize_v2 : crypto.SaltSize_v2+crypto.IVSize_v2]
	encryptedData := fileData[crypto.SaltSize_v2+crypto.IVSize_v2:]

	// 生成密钥
	key := crypto.GenerateKey(tm.cfg.Password, salt, crypto.KeySize_v2)

	// 解密数据
	decryptedData, err := crypto.DecryptBytes_v2(encryptedData, key, iv)
	if err != nil {
		return nil, err
	}

	// 反序列化
	var siteToken SiteToken
	if err := json.Unmarshal(decryptedData, &siteToken); err != nil {
		return nil, err
	}

	return &siteToken, nil
}

// 【新增】loadAllTokens 启动时加载所有 token
func (tm *TokenManager) loadAllTokens() {
	// 扫描配置目录中的 token 文件
	pattern := filepath.Join(tm.configDir, TokenFileNamePrefix+"*"+TokenFileExtension)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	for _, tokenFile := range matches {
		// 提取 siteId
		filename := filepath.Base(tokenFile)
		siteId := filename[len(TokenFileNamePrefix) : len(filename)-len(TokenFileExtension)]

		// 加载 token
		if siteToken, err := tm.loadTokenFromFile(siteId); err == nil {
			// 检查是否过期
			if !time.Now().After(siteToken.ExpiresAt) {
				tm.tokens[siteId] = *siteToken
			} else {
				// 删除过期的 token 文件
				os.Remove(tokenFile)
			}
		}
	}
}

// 【新增】getTokenFilePath 获取 token 文件路径
func (tm *TokenManager) getTokenFilePath(siteId string) string {
	filename := TokenFileNamePrefix + siteId + TokenFileExtension
	return filepath.Join(tm.configDir, filename)
}

// cleanupExpired 清理过期 token
func (tm *TokenManager) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		tm.mutex.Lock()
		for siteId, token := range tm.tokens {
			if time.Now().After(token.ExpiresAt) {
				delete(tm.tokens, siteId)
				// 删除文件
				tokenFile := tm.getTokenFilePath(siteId)
				os.Remove(tokenFile)
			}
		}
		tm.mutex.Unlock()
	}
}

// GetSiteToken 获取完整的站点token信息（包括过期时间）
func (tm *TokenManager) GetSiteToken(siteId string) *SiteToken {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	token, exists := tm.tokens[siteId]
	if !exists || time.Now().After(token.ExpiresAt) {
		return nil
	}
	return &token
}

// SetTokenExpiry 设置token的有效期
func (tm *TokenManager) SetTokenExpiry(siteId string, days int) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	token, exists := tm.tokens[siteId]
	if !exists {
		return fmt.Errorf("token not found for site: %s", siteId)
	}

	// 更新过期时间
	token.ExpiresAt = time.Now().Add(time.Duration(days) * 24 * time.Hour)
	tm.tokens[siteId] = token

	// 保存到文件
	return tm.saveTokenToFile(siteId, token)
}
