package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	sharedauth "resume-backend/internal/shared/auth"
	"resume-backend/internal/shared/server/respond"
)

// GoogleService handles Google OAuth flows.
type GoogleService struct {
	oauthConfig *oauth2.Config
	uiRedirect  string
	stateTTL    time.Duration
	stateStore  *stateStore
}

// NewGoogleService builds a GoogleService.
func NewGoogleService(clientID, clientSecret, redirectURL, uiRedirect string) *GoogleService {
	return &GoogleService{
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		},
		uiRedirect: uiRedirect,
		stateTTL:   5 * time.Minute,
		stateStore: newStateStore(),
	}
}

// RegisterRoutes attaches Google auth routes.
func (s *GoogleService) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/auth/google/start", s.start)
	rg.GET("/auth/google/callback", s.callback)
}

func (s *GoogleService) start(c *gin.Context) {
	if s.oauthConfig.ClientID == "" || s.oauthConfig.ClientSecret == "" || s.oauthConfig.RedirectURL == "" {
		respond.Error(c, http.StatusInternalServerError, "auth_not_configured", "Google auth not configured", nil)
		return
	}

	state := uuid.NewString()
	s.stateStore.put(state, time.Now().Add(s.stateTTL))

	url := s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusFound, url)
}

func (s *GoogleService) callback(c *gin.Context) {
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		respond.Error(c, http.StatusBadRequest, "invalid_request", "missing state or code", nil)
		return
	}

	if !s.stateStore.consume(state) {
		respond.Error(c, http.StatusBadRequest, "invalid_request", "invalid or expired state", nil)
		return
	}

	ctx := c.Request.Context()
	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		respond.Error(c, http.StatusBadRequest, "invalid_request", "failed to exchange code", nil)
		return
	}

	userInfo, err := s.fetchUserInfo(ctx, token)
	if err != nil {
		respond.Error(c, http.StatusBadGateway, "auth_failed", "failed to fetch user profile", nil)
		return
	}

	if userInfo.Sub == "" {
		respond.Error(c, http.StatusBadGateway, "auth_failed", "invalid user profile", nil)
		return
	}

	jwt, err := sharedauth.SignJWT(sharedauth.Claims{
		Sub:     "google:" + userInfo.Sub,
		Email:   userInfo.Email,
		Name:    userInfo.Name,
		Picture: userInfo.Picture,
	})
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to issue token", nil)
		return
	}

	redirectURL, err := appendToken(s.uiRedirect, jwt)
	if err != nil {
		respond.Error(c, http.StatusInternalServerError, "internal_error", "failed to redirect", nil)
		return
	}

	c.Redirect(http.StatusFound, redirectURL)
}

type googleUserInfo struct {
	Sub     string `json:"sub"`
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (s *GoogleService) fetchUserInfo(ctx context.Context, token *oauth2.Token) (googleUserInfo, error) {
	client := s.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return googleUserInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return googleUserInfo{}, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return googleUserInfo{}, err
	}

	// Some responses use "id" instead of "sub".
	if info.Sub == "" {
		info.Sub = info.ID
	}
	return info, nil
}

type stateStore struct {
	items map[string]time.Time
	mu    sync.Mutex
}

func newStateStore() *stateStore {
	return &stateStore{items: make(map[string]time.Time)}
}

func (s *stateStore) put(state string, exp time.Time) {
	s.mu.Lock()
	s.items[state] = exp
	s.mu.Unlock()
}

func (s *stateStore) consume(state string) bool {
	s.mu.Lock()
	exp, ok := s.items[state]
	if ok {
		delete(s.items, state)
	}
	s.mu.Unlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		return false
	}
	return true
}

func appendToken(rawURL, token string) (string, error) {
	if rawURL == "" {
		return "", errors.New("redirect url required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
