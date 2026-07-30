package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const oauthStateCookie = "open_resouce_oauth_state"

type oauthProviderConfig struct {
	ClientID, ClientSecret, AuthorizeURL, TokenURL string
}

func oauthStartHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	provider := request.PathValue("provider")
	config, ok := oauthConfig(provider)
	if !ok {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "oauth_not_configured", "该登录方式尚未配置")
		return
	}
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	http.SetCookie(writer, &http.Cookie{
		Name: oauthStateCookie, Value: provider + ":" + state, Path: "/api/v1/auth/oauth/",
		MaxAge: 600, HttpOnly: true, Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode,
	})
	parameters := url.Values{
		"client_id":    {config.ClientID},
		"redirect_uri": {oauthCallbackURL(request, provider)},
		"state":        {state},
	}
	if provider == "github" {
		parameters.Set("scope", "read:user")
	} else {
		parameters.Set("response_type", "code")
		parameters.Set("scope", "snsapi_login")
	}
	target := config.AuthorizeURL + "?" + parameters.Encode()
	if provider == "wechat" {
		target += "#wechat_redirect"
	}
	http.Redirect(writer, request, target, http.StatusFound)
}

func oauthCallbackHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	provider := request.PathValue("provider")
	config, ok := oauthConfig(provider)
	if !ok || authRepositoryStore == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "oauth_not_configured", "该登录方式尚未配置")
		return
	}
	cookie, err := request.Cookie(oauthStateCookie)
	expected := provider + ":" + request.URL.Query().Get("state")
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(expected)) != 1 ||
		request.URL.Query().Get("code") == "" {
		writeAPIError(writer, request, http.StatusBadRequest, "invalid_oauth_callback", "第三方登录回调无效")
		return
	}
	subject, displayName, err := fetchOAuthIdentity(request.Context(), provider, config, request.URL.Query().Get("code"), oauthCallbackURL(request, provider))
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	email := provider + "-" + shortHash(subject) + "@oauth.local"
	user, found, err := authRepositoryStore.FindUserByEmail(request.Context(), email)
	if err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	if !found {
		user = authUser{
			ID: "user-" + newRequestID(), Email: email,
			DisplayName: displayName, PasswordHash: "oauth-disabled",
		}
		if err := authRepositoryStore.CreateUser(request.Context(), user); err != nil {
			if !errors.Is(err, errEmailExists) {
				writeAuthInternalError(writer, request, err)
				return
			}
			user, _, err = authRepositoryStore.FindUserByEmail(request.Context(), email)
			if err != nil {
				writeAuthInternalError(writer, request, err)
				return
			}
		}
	}
	if err := createLoginSession(writer, request, user); err != nil {
		writeAuthInternalError(writer, request, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/api/v1/auth/oauth/", MaxAge: -1, HttpOnly: true, Secure: requestIsHTTPS(request), SameSite: http.SameSiteLaxMode})
	auditAuth(request, provider+"_oauth_succeeded", email, user.ID)
	http.Redirect(writer, request, "/", http.StatusFound)
}

func oauthConfig(provider string) (oauthProviderConfig, bool) {
	var config oauthProviderConfig
	switch provider {
	case "github":
		config = oauthProviderConfig{
			ClientID: os.Getenv("GITHUB_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
			AuthorizeURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token",
		}
	case "wechat":
		config = oauthProviderConfig{
			ClientID: os.Getenv("WECHAT_OAUTH_APP_ID"), ClientSecret: os.Getenv("WECHAT_OAUTH_APP_SECRET"),
			AuthorizeURL: "https://open.weixin.qq.com/connect/qrconnect", TokenURL: "https://api.weixin.qq.com/sns/oauth2/access_token",
		}
	default:
		return oauthProviderConfig{}, false
	}
	return config, config.ClientID != "" && config.ClientSecret != ""
}

func oauthCallbackURL(request *http.Request, provider string) string {
	base := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")
	if base == "" {
		scheme := "http"
		if requestIsHTTPS(request) {
			scheme = "https"
		}
		base = scheme + "://" + request.Host
	}
	return base + "/api/v1/auth/oauth/" + provider + "/callback"
}

func fetchOAuthIdentity(ctx context.Context, provider string, config oauthProviderConfig, code, redirectURI string) (string, string, error) {
	// GitHub can be reached through a high-latency international route from the
	// application server. Keep a bounded but tolerant timeout for the one-time
	// authorization-code exchange.
	client := &http.Client{Timeout: 30 * time.Second}
	if provider == "github" {
		form := url.Values{"client_id": {config.ClientID}, "client_secret": {config.ClientSecret}, "code": {code}, "redirect_uri": {redirectURI}}
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, config.TokenURL, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return "", "", fmt.Errorf("exchange GitHub OAuth code: %w", err)
		}
		defer response.Body.Close()
		var token struct {
			AccessToken string `json:"access_token"`
		}
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil || token.AccessToken == "" {
			return "", "", fmt.Errorf("GitHub OAuth token exchange failed")
		}
		userRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
		userRequest.Header.Set("Authorization", "Bearer "+token.AccessToken)
		userRequest.Header.Set("Accept", "application/vnd.github+json")
		userResponse, err := client.Do(userRequest)
		if err != nil {
			return "", "", fmt.Errorf("read GitHub user: %w", err)
		}
		defer userResponse.Body.Close()
		var user struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Name  string `json:"name"`
		}
		if userResponse.StatusCode != http.StatusOK || json.NewDecoder(userResponse.Body).Decode(&user) != nil || user.ID == 0 {
			return "", "", fmt.Errorf("GitHub user lookup failed")
		}
		if strings.TrimSpace(user.Name) == "" {
			user.Name = user.Login
		}
		return strconv.FormatInt(user.ID, 10), user.Name, nil
	}
	parameters := url.Values{"appid": {config.ClientID}, "secret": {config.ClientSecret}, "code": {code}, "grant_type": {"authorization_code"}}
	response, err := client.Get(config.TokenURL + "?" + parameters.Encode())
	if err != nil {
		return "", "", fmt.Errorf("exchange WeChat OAuth code: %w", err)
	}
	defer response.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&token) != nil || token.AccessToken == "" || token.OpenID == "" {
		return "", "", fmt.Errorf("WeChat OAuth token exchange failed")
	}
	userURL := "https://api.weixin.qq.com/sns/userinfo?" + url.Values{"access_token": {token.AccessToken}, "openid": {token.OpenID}, "lang": {"zh_CN"}}.Encode()
	userResponse, err := client.Get(userURL)
	if err != nil {
		return "", "", fmt.Errorf("read WeChat user: %w", err)
	}
	defer userResponse.Body.Close()
	var user struct {
		Nickname string `json:"nickname"`
	}
	if userResponse.StatusCode != http.StatusOK || json.NewDecoder(userResponse.Body).Decode(&user) != nil || strings.TrimSpace(user.Nickname) == "" {
		return "", "", fmt.Errorf("WeChat user lookup failed")
	}
	return token.OpenID, user.Nickname, nil
}
