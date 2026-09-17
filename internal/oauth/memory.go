package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"
)

// Client 动态或静态注册的 OAuth 客户端（公钥客户端为主）。
type Client struct {
	ID                      string
	Secret                  string // 公钥客户端为空
	Name                    string
	RedirectURIs            []string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
	CreatedAt               time.Time
}

// authCode 一次性授权码。
type authCode struct {
	Code                string
	ClientID            string
	UserID              string
	RedirectURI         string
	Scope               string
	Resource            string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}

// refreshToken 可刷新会话。
type refreshToken struct {
	Token     string
	ClientID  string
	UserID    string
	Scope     string
	Resource  string
	ExpiresAt time.Time
}

type memoryStore struct {
	mu       sync.Mutex
	clients  map[string]*Client
	codes    map[string]*authCode
	refresh  map[string]*refreshToken
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		clients: make(map[string]*Client),
		codes:   make(map[string]*authCode),
		refresh: make(map[string]*refreshToken),
	}
}

func (m *memoryStore) putClient(c *Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *c
	cp.RedirectURIs = append([]string(nil), c.RedirectURIs...)
	m.clients[c.ID] = &cp
	return nil
}

func (m *memoryStore) getClient(id string) (*Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clients[id]
	if !ok {
		return nil, false
	}
	cp := *c
	cp.RedirectURIs = append([]string(nil), c.RedirectURIs...)
	return &cp, true
}

func (m *memoryStore) clientAllowsRedirect(id, redirectURI string) bool {
	c, ok := m.getClient(id)
	if !ok {
		return false
	}
	for _, u := range c.RedirectURIs {
		if u == redirectURI {
			return true
		}
	}
	return false
}

func (m *memoryStore) saveCode(c *authCode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked(time.Now())
	cp := *c
	m.codes[c.Code] = &cp
}

func (m *memoryStore) takeCode(code string) (*authCode, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked(time.Now())
	c, ok := m.codes[code]
	if !ok {
		return nil, false
	}
	delete(m.codes, code)
	if time.Now().After(c.ExpiresAt) {
		return nil, false
	}
	cp := *c
	return &cp, true
}

func (m *memoryStore) saveRefresh(t *refreshToken) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *t
	m.refresh[t.Token] = &cp
}

func (m *memoryStore) takeRefresh(token string) (*refreshToken, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked(time.Now())
	t, ok := m.refresh[token]
	if !ok {
		return nil, false
	}
	delete(m.refresh, token) // 轮换：用后作废
	if time.Now().After(t.ExpiresAt) {
		return nil, false
	}
	cp := *t
	return &cp, true
}

func (m *memoryStore) purgeLocked(now time.Time) {
	for k, c := range m.codes {
		if now.After(c.ExpiresAt) {
			delete(m.codes, k)
		}
	}
	for k, t := range m.refresh {
		if now.After(t.ExpiresAt) {
			delete(m.refresh, k)
		}
	}
}

func randomToken(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomClientID() (string, error) {
	return randomToken(16)
}

// verifyPKCE 校验 S256 / plain code_verifier。
func verifyPKCE(verifier, challenge, method string) bool {
	method = normalizeChallengeMethod(method)
	switch method {
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		encoded := base64.RawURLEncoding.EncodeToString(sum[:])
		return encoded == challenge
	case "plain":
		return verifier == challenge
	default:
		return false
	}
}

func normalizeChallengeMethod(m string) string {
	if m == "" {
		return "plain"
	}
	return m
}
