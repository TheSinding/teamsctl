package teamsauth

import "time"

type Options struct {
	Email      string
	Password   string
	OTP        string
	ChromePath string
	Timeout    time.Duration
}

type authTokenKind string

type authTenant struct {
	TenantID string `json:"tenantId"`
}

type authClaims struct {
	Audience  string `json:"aud"`
	TenantID  string `json:"tid"`
	ExpiresAt int64  `json:"exp"`
}
