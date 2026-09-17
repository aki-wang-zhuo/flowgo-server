package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowgo/flowgo-server/internal/store"
)

// Claims JWT 载荷。
type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Service 认证服务。
type Service struct {
	secret []byte
	ttl    time.Duration
	store  *store.Store
}

// NewService 创建认证服务。
func NewService(secret string, ttl time.Duration, st *store.Store) *Service {
	return &Service{secret: []byte(secret), ttl: ttl, store: st}
}

// Login 用户名密码登录，返回 JWT（编辑器会话，无 MCP audience）。
func (s *Service) Login(username, password string) (token string, user *store.User, err error) {
	user, err = s.store.FindUserByUsername(username)
	if err != nil {
		return "", nil, errors.New("invalid username or password")
	}
	if !store.CheckPassword(user.PasswordHash, password) {
		return "", nil, errors.New("invalid username or password")
	}
	token, err = s.issueToken(user, "", s.ttl)
	return token, user, err
}

// IssueAccessToken 签发带 audience 的访问令牌（MCP OAuth 用）。
func (s *Service) IssueAccessToken(user *store.User, audience string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = s.ttl
	}
	return s.issueToken(user, audience, ttl)
}

func (s *Service) issueToken(user *store.User, audience string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "flowgo",
		},
	}
	if audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(s.secret)
}

// ParseToken 解析并校验 JWT。
func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// AuthenticateRequest 从 Authorization 头解析身份：
// - Bearer <jwt>
// - ApiKey <key> 或 X-API-Key
func (s *Service) AuthenticateRequest(r *http.Request) (*store.User, error) {
	return s.AuthenticateRequestForAudience(r, "")
}

// AuthenticateRequestForAudience 鉴权；expectedAud 非空时：
// - 带 aud 的 JWT 必须包含 expectedAud（MCP OAuth access_token）
// - 无 aud 的旧版编辑器 JWT 仍允许（脚本/过渡）
// - API Key 始终允许
func (s *Service) AuthenticateRequestForAudience(r *http.Request, expectedAud string) (*store.User, error) {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return s.store.FindUserByAPIKey(key)
	}
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, errors.New("missing authorization")
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid authorization header")
	}
	switch strings.ToLower(parts[0]) {
	case "bearer":
		claims, err := s.ParseToken(parts[1])
		if err != nil {
			return nil, err
		}
		if expectedAud != "" && len(claims.Audience) > 0 {
			if !audienceContains(claims.Audience, expectedAud) {
				return nil, errors.New("invalid token audience")
			}
		}
		return s.store.FindUserByID(claims.UserID)
	case "apikey":
		return s.store.FindUserByAPIKey(parts[1])
	default:
		return nil, errors.New("unsupported authorization scheme")
	}
}

func audienceContains(aud jwt.ClaimStrings, want string) bool {
	want = strings.TrimRight(strings.TrimSpace(want), "/")
	for _, a := range aud {
		if strings.TrimRight(strings.TrimSpace(a), "/") == want {
			return true
		}
	}
	return false
}

// UserByID 按用户 ID 加载用户（供 WebSocket query token 鉴权后使用）。
func (s *Service) UserByID(id string) (*store.User, error) {
	return s.store.FindUserByID(id)
}
