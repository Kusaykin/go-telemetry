package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

type FileStorage struct {
	*MemStorage

	path     string
	interval time.Duration
	log      *zap.Logger

	dirty  atomic.Bool
	saveMu sync.Mutex
}

func NewFileStorage(path string, interval time.Duration, restore bool, log *zap.Logger) (*FileStorage, error) {
	if interval < 0 {
		return nil, fmt.Errorf("store interval must not be negative: %s", interval)
	}

	s := &FileStorage{
		MemStorage: NewMemStorage(),
		path:       path,
		interval:   interval,
		log:        log,
	}

	if path == "" {
		return s, nil
	}

	if err := checkPath(path); err != nil {
		return nil, fmt.Errorf("file storage path %s: %w", path, err)
	}

	if restore {
		if err := s.load(); err != nil {
			return nil, fmt.Errorf("restore metrics from %s: %w", path, err)
		}
	}

	return s, nil
}

// checkPath проверяет, что в путь можно будет записать файл: каталог
// существует, а сам путь либо отсутствует, либо является обычным файлом.
func checkPath(path string) error {
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return err
	}

	if !dir.IsDir() {
		return fmt.Errorf("%s is not a directory", filepath.Dir(path))
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if info.IsDir() {
		return errors.New("is a directory")
	}

	return nil
}

func (s *FileStorage) UpdateGauge(name string, value float64) {
	s.MemStorage.UpdateGauge(name, value)
	s.dirty.Store(true)
	s.saveIfSync()
}

func (s *FileStorage) UpdateCounter(name string, delta int64) int64 {
	value := s.MemStorage.UpdateCounter(name, delta)
	s.dirty.Store(true)
	s.saveIfSync()

	return value
}

func (s *FileStorage) syncWrite() bool {
	return s.interval == 0
}

func (s *FileStorage) saveIfSync() {
	if !s.syncWrite() {
		return
	}

	s.saveAndLog()
}

func (s *FileStorage) saveAndLog() {
	if err := s.Save(); err != nil {
		s.log.Error("save metrics", zap.String("path", s.path), zap.Error(err))
	}
}

func (s *FileStorage) Run(ctx context.Context) error {
	if s.syncWrite() || s.path == "" {
		return nil
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if s.dirty.Load() {
				if err := s.Save(); err != nil {
					return fmt.Errorf("save metrics to %s: %w", s.path, err)
				}
			}
		}
	}
}

func (s *FileStorage) Save() error {
	if s.path == "" {
		return nil
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.dirty.Store(false)

	data, err := json.MarshalIndent(s.Snapshot(), "", "  ")
	if err != nil {
		s.dirty.Store(true)

		return err
	}

	if err := writeFileAtomic(s.path, data, 0o644); err != nil {
		s.dirty.Store(true)

		return err
	}

	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	tmpPath := path + ".tmp"

	tmp, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return err
	}

	if err = tmp.Sync(); err != nil {
		return err
	}

	if err = tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func (s *FileStorage) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	var metrics []models.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return err
	}

	return s.restore(metrics)
}
