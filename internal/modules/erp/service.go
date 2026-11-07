package erp

import (
	"crypto/hmac"
	"crypto/sha256"
	"log/slog"
)

type Config struct {
	Pepper                     []byte
	Tag                        string
	DefaultRate, MaxBatchItems int
	Provider                   string
}
type Service struct {
	repo       Repository
	ports      Ports
	cfg        Config
	log        *slog.Logger
	hashSecret func(string) []byte
}

func NewService(repo Repository, ports Ports, cfg Config, log *slog.Logger) *Service {
	return &Service{repo: repo, ports: ports, cfg: cfg, log: log, hashSecret: func(secret string) []byte {
		mac := hmac.New(sha256.New, cfg.Pepper)
		_, _ = mac.Write([]byte(secret))
		return mac.Sum(nil)
	}}
}
