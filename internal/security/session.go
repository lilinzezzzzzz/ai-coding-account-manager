package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

const (
	// SessionCookieName 是本地管理会话 Cookie 名称。
	SessionCookieName = "aicam_session"

	defaultSessionTTL = 12 * time.Hour
	tokenByteLength   = 32
)

type sessionContextKey struct{}

// Config 保存本地 Web 安全管理器配置。
type Config struct {
	BindAddr       string
	BootstrapToken string
	SessionTTL     time.Duration
}

// Session 保存已认证浏览器会话的最小状态。
type Session struct {
	ID        string
	CSRFToken string
	ExpiresAt time.Time
}

// Manager 管理一次性 bootstrap token、内存 session 和 CSRF token。
type Manager struct {
	mu sync.Mutex

	bootstrapToken string
	bootstrapUsed  bool
	sessionTTL     time.Duration
	sessions       map[string]Session
	allowedHosts   map[string]struct{}
	allowedOrigins map[string]struct{}
}

// NewManager 创建本地 Web 安全管理器。
func NewManager(cfg Config) (*Manager, error) {
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("bind address is required")
	}
	host, port, err := net.SplitHostPort(cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("parse bind address: %w", err)
	}
	if port == "" {
		return nil, fmt.Errorf("bind address port is required")
	}

	bootstrapToken := cfg.BootstrapToken
	if bootstrapToken == "" {
		bootstrapToken, err = newToken()
		if err != nil {
			return nil, fmt.Errorf("create bootstrap token: %w", err)
		}
	}

	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}

	allowedHosts := map[string]struct{}{}
	addHost := func(name string) {
		if name != "" {
			allowedHosts[net.JoinHostPort(name, port)] = struct{}{}
		}
	}
	addHost(host)
	addHost("127.0.0.1")
	addHost("localhost")

	allowedOrigins := make(map[string]struct{}, len(allowedHosts))
	for allowedHost := range allowedHosts {
		allowedOrigins["http://"+allowedHost] = struct{}{}
	}

	return &Manager{
		bootstrapToken: bootstrapToken,
		sessionTTL:     sessionTTL,
		sessions:       make(map[string]Session),
		allowedHosts:   allowedHosts,
		allowedOrigins: allowedOrigins,
	}, nil
}

// ExchangeBootstrap 使用一次性 bootstrap token 创建浏览器会话。
func (manager *Manager) ExchangeBootstrap(token string, now time.Time) (Session, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.bootstrapUsed || !constantTimeEqual(token, manager.bootstrapToken) {
		return Session{}, entity.NewAppError(entity.ErrorCodeForbidden)
	}

	sessionID, err := newToken()
	if err != nil {
		return Session{}, fmt.Errorf("create session id: %w", err)
	}
	csrfToken, err := newToken()
	if err != nil {
		return Session{}, fmt.Errorf("create csrf token: %w", err)
	}

	manager.bootstrapUsed = true
	session := Session{
		ID:        sessionID,
		CSRFToken: csrfToken,
		ExpiresAt: now.Add(manager.sessionTTL),
	}
	manager.sessions[sessionID] = session
	return session, nil
}

// SessionFromRequest 从 Cookie 中解析并校验会话。
func (manager *Manager) SessionFromRequest(r *http.Request, now time.Time) (Session, bool) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return Session{}, false
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	session, ok := manager.sessions[cookie.Value]
	if !ok {
		return Session{}, false
	}
	if !now.Before(session.ExpiresAt) {
		delete(manager.sessions, cookie.Value)
		return Session{}, false
	}
	return session, true
}

// ValidateHost 校验请求 Host 是否属于当前本地服务。
func (manager *Manager) ValidateHost(host string) bool {
	normalized, ok := normalizeHost(host)
	if !ok {
		return false
	}
	_, ok = manager.allowedHosts[normalized]
	return ok
}

// ValidateOrigin 校验写请求 Origin 是否为当前服务 origin。
func (manager *Manager) ValidateOrigin(origin string) bool {
	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	_, ok = manager.allowedOrigins[normalizedOrigin]
	return ok
}

// ValidateOriginForHost 校验 Origin 与本次请求 Host 精确匹配。
func (manager *Manager) ValidateOriginForHost(origin string, host string) bool {
	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	normalizedHost, ok := normalizeHost(host)
	if !ok {
		return false
	}
	return normalizedOrigin == "http://"+normalizedHost
}

// ValidateCSRF 校验请求头中的 CSRF token 是否匹配当前 session。
func (manager *Manager) ValidateCSRF(session Session, token string) bool {
	return constantTimeEqual(token, session.CSRFToken)
}

// CookieForSession 生成 HttpOnly session Cookie。
func CookieForSession(session Session) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

// WithSession 将已校验的会话写入 request context。
func WithSession(ctx context.Context, session Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}

// SessionFromContext 从 context 读取已校验会话。
func SessionFromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(Session)
	return session, ok
}

func newToken() (string, error) {
	var raw [tokenByteLength]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func constantTimeEqual(left string, right string) bool {
	if left == "" || right == "" || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func normalizeHost(host string) (string, bool) {
	if strings.TrimSpace(host) != host || host == "" {
		return "", false
	}
	normalizedHost, port, err := net.SplitHostPort(host)
	if err != nil || normalizedHost == "" || port == "" {
		return "", false
	}
	return net.JoinHostPort(normalizedHost, port), true
}

func normalizeOrigin(origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", false
	}
	normalizedHost, ok := normalizeHost(parsed.Host)
	if !ok {
		return "", false
	}
	return "http://" + normalizedHost, true
}
