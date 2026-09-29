package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// PatService P4-6 PAT（SelfService 语义——本人管理自己的凭据）。
// 明文 zpat_<43> 仅创建响应返回一次；落库 sha256（勿抄 gva signing key——独立 secret）。
type PatService struct {
	repo   *repository.PatRepo
	logger *slog.Logger
}

func NewPatService(repo *repository.PatRepo, logger *slog.Logger) *PatService {
	return &PatService{repo: repo, logger: logger}
}

const patAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const patPrefix = "zpat_"

// generateSecret 43 位随机串（~256bit 熵——crypto/rand）
func generateSecret() string {
	b := make([]byte, 43)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(patAlphabet))))
		if err != nil {
			b[i] = patAlphabet[i%len(patAlphabet)]
			continue
		}
		b[i] = patAlphabet[n.Int64()]
	}
	return patPrefix + string(b)
}

func HashSecret(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// Create 返回（落库行, 明文 secret——仅此一次出网）
func (s *PatService) Create(ctx context.Context, userID int64, name string, expiresDays int) (*model.PersonalAccessToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", errcode.New(errcode.ErrInvalidParams.Code, "name 必填")
	}
	if expiresDays < 0 || expiresDays > 3650 {
		return nil, "", errcode.New(errcode.ErrInvalidParams.Code, "expires_days 须为 0（永不过期）或 1–3650")
	}
	t := &model.PersonalAccessToken{
		UserID: userID, Name: name, Scope: "full",
	}
	if expiresDays > 0 {
		exp := time.Now().AddDate(0, 0, expiresDays)
		t.ExpiresAt = &exp
	}
	// 碰撞重试 ×3（43 位随机空间 2^256——理论不会，防御性）
	for i := 0; i < 3; i++ {
		plain := generateSecret()
		t.SecretHash = HashSecret(plain)
		if err := s.repo.Create(ctx, t); err != nil {
			if i == 2 {
				s.logger.Error("pat create", "err", err)
				return nil, "", errcode.ErrInternal
			}
			continue
		}
		return t, plain, nil
	}
	return nil, "", errcode.ErrInternal
}

// List 本人列表（剥离 hash）
func (s *PatService) List(ctx context.Context, userID int64) ([]*model.PersonalAccessToken, error) {
	list, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		s.logger.Error("pat list", "err", err)
		return nil, errcode.ErrInternal
	}
	for _, t := range list {
		t.SecretHash = ""
	}
	return list, nil
}

// Revoke 吊销（本人资源——id+userID 双条件防越权）
func (s *PatService) Revoke(ctx context.Context, userID, id int64) error {
	if err := s.repo.Revoke(ctx, id, userID); err != nil {
		if errors.Is(err, repository.ErrPatNotFound) {
			return errcode.ErrPatNotFound
		}
		s.logger.Error("pat revoke", "err", err)
		return errcode.ErrInternal
	}
	return nil
}
