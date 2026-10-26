package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const accessTokenIssuer = "campusclaw"

var errInvalidAccessToken = errors.New("invalid access token")

type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type accessTokenClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	TokenID   string `json:"jti"`
}

func (s *Server) issueAccessToken(userID int64, now time.Time) (string, error) {
	tokenID, err := randomToken(16)
	if err != nil {
		return "", err
	}
	headerJSON, err := json.Marshal(tokenHeader{Algorithm: "HS256", Type: "JWT"})
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(accessTokenClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    accessTokenIssuer,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.config.JWTTTL).Unix(),
		TokenID:   tokenID,
	})
	if err != nil {
		return "", err
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(s.config.JWTSecret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func (s *Server) verifyAccessToken(raw string, now time.Time) (int64, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return 0, errInvalidAccessToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, errInvalidAccessToken
	}
	var header tokenHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil ||
		header.Algorithm != "HS256" ||
		header.Type != "JWT" {
		return 0, errInvalidAccessToken
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, errInvalidAccessToken
	}
	mac := hmac.New(sha256.New, []byte(s.config.JWTSecret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return 0, errInvalidAccessToken
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errInvalidAccessToken
	}
	var claims accessTokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return 0, errInvalidAccessToken
	}
	if claims.Issuer != accessTokenIssuer ||
		claims.TokenID == "" ||
		claims.ExpiresAt <= now.Unix() ||
		claims.IssuedAt > now.Add(time.Minute).Unix() ||
		claims.IssuedAt >= claims.ExpiresAt {
		return 0, errInvalidAccessToken
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errInvalidAccessToken
	}
	return userID, nil
}
