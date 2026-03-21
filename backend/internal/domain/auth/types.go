package auth

import "time"

type User struct {
	ID         uint64
	EVMAddress string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Nonce struct {
	ID        string
	Address   string
	ChainID   int64
	Domain    string
	Value     string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type Session struct {
	ID                string
	UserID            uint64
	AccessJTI         string
	RefreshJTI        string
	DeviceFingerprint string
	IP                string
	UserAgent         string
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

type IssueNonceInput struct {
	Address string
	ChainID int64
}

type IssueNonceOutput struct {
	Nonce     string
	Domain    string
	ChainID   int64
	ExpiresAt time.Time
}

type LoginInput struct {
	Address           string
	ChainID           int64
	Nonce             string
	Signature         string
	DeviceFingerprint string
	IP                string
	UserAgent         string
}

type LoginResult struct {
	User         User
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}
