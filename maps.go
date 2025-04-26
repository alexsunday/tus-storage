package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// maps filename to tus upload id
type FileIdMap interface {
	GetFileId(fileName string) (string, error)
	SetFileIdMap(fileName, fileId string) error
}

type FileIdMapRedisImpl struct {
	r   *redis.Client
	ctx context.Context
}

const (
	fileIdMapKey = "TUS:FileIdMap"
)

// redis://<user>:<pass>@localhost:6379/<db>
func NewFileIdMapRedisImpl(redisUrl string) (*FileIdMapRedisImpl, error) {
	opt, err := redis.ParseURL(redisUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	rediClient := redis.NewClient(opt)
	return &FileIdMapRedisImpl{
		r:   rediClient,
		ctx: context.Background(),
	}, nil
}

func (m *FileIdMapRedisImpl) GetFileId(fileName string) (string, error) {
	return m.r.HGet(m.ctx, fileIdMapKey, fileName).Result()
}

func (m *FileIdMapRedisImpl) SetFileIdMap(fileName, fileId string) error {
	_, err := m.r.HSet(m.ctx, fileIdMapKey, fileName, fileId).Result()
	return err
}
