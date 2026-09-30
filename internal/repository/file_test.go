package repository

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	syncInterval     = 0
	periodicInterval = time.Hour
)

func tempPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "metrics.json")
}

func newFileStorage(t *testing.T, path string, interval time.Duration, restore bool) *FileStorage {
	t.Helper()

	s, err := NewFileStorage(path, interval, restore, zap.NewNop())
	require.NoError(t, err)

	return s
}

func TestFileStorageRoundTrip(t *testing.T) {
	path := tempPath(t)

	s := newFileStorage(t, path, periodicInterval, false)
	s.UpdateGauge("Alloc", 12.5)
	s.UpdateGauge("Zero", 0)
	s.UpdateCounter("PollCount", 42)
	s.UpdateCounter("ZeroCount", 0)
	require.NoError(t, s.Save())

	restored := newFileStorage(t, path, periodicInterval, true)

	assert.Equal(t, map[string]float64{"Alloc": 12.5, "Zero": 0}, restored.Gauges())
	assert.Equal(t, map[string]int64{"PollCount": 42, "ZeroCount": 0}, restored.Counters())
}

func TestFileStorageReads(t *testing.T) {
	s := newFileStorage(t, tempPath(t), periodicInterval, false)
	s.UpdateGauge("Alloc", 1.5)
	assert.Equal(t, int64(3), s.UpdateCounter("PollCount", 3))

	value, ok := s.Gauge("Alloc")
	assert.True(t, ok)
	assert.Equal(t, 1.5, value)

	delta, ok := s.Counter("PollCount")
	assert.True(t, ok)
	assert.Equal(t, int64(3), delta)

	_, ok = s.Gauge("Missing")
	assert.False(t, ok)

	_, ok = s.Counter("Missing")
	assert.False(t, ok)
}

func TestFileStorageFileFormat(t *testing.T) {
	path := tempPath(t)

	s := newFileStorage(t, path, periodicInterval, false)
	s.UpdateGauge("LastGC", 1257894000000000000)
	s.UpdateCounter("NumGC", 42)
	require.NoError(t, s.Save())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.JSONEq(t, `[
		{"id":"NumGC","type":"counter","delta":42},
		{"id":"LastGC","type":"gauge","value":1257894000000000000}
	]`, string(data))
}

func TestFileStorageRestoreDisabledIgnoresFile(t *testing.T) {
	path := tempPath(t)
	require.NoError(t, os.WriteFile(path, []byte(`[{"id":"NumGC","type":"counter","delta":42}]`), 0o644))

	s := newFileStorage(t, path, periodicInterval, false)

	assert.Empty(t, s.Counters())
}

func TestFileStorageRestoreNothingToLoad(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{"файла нет", func(*testing.T, string) {}},
		{"пустой файл", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, nil, 0o644))
		}},
		{"только пробелы", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(" \n"), 0o644))
		}},
		{"пустой массив", func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("[]"), 0o644))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tempPath(t)
			tt.setup(t, path)

			s := newFileStorage(t, path, periodicInterval, true)

			assert.Empty(t, s.Gauges())
			assert.Empty(t, s.Counters())
		})
	}
}

func TestFileStorageRestoreErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"битый JSON", `[{"id":"NumGC"`},
		{"не массив", `{"id":"NumGC","type":"counter","delta":42}`},
		{"неизвестный тип", `[{"id":"X","type":"histogram","value":1}]`},
		{"gauge без value", `[{"id":"Alloc","type":"gauge"}]`},
		{"counter без delta", `[{"id":"NumGC","type":"counter"}]`},
		{"пустой id", `[{"id":"","type":"counter","delta":1}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tempPath(t)
			require.NoError(t, os.WriteFile(path, []byte(tt.data), 0o644))

			_, err := NewFileStorage(path, periodicInterval, true, zap.NewNop())

			assert.Error(t, err)
		})
	}
}

func TestNewFileStorageChecksPath(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		restore bool
	}{
		{"каталога нет", filepath.Join(dir, "missing", "metrics.json"), false},
		{"каталога нет, restore", filepath.Join(dir, "missing", "metrics.json"), true},
		{"родитель — файл", filepath.Join(dir, "file", "metrics.json"), false},
		{"путь — каталог", filepath.Join(dir, "subdir"), false},
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), nil, 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileStorage(tt.path, periodicInterval, tt.restore, zap.NewNop())

			assert.Error(t, err)
		})
	}
}

func TestNewFileStorageRejectsNegativeInterval(t *testing.T) {
	_, err := NewFileStorage(tempPath(t), -time.Second, false, zap.NewNop())

	assert.Error(t, err)
}

func TestFileStorageSyncWrite(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, syncInterval, false)

	s.UpdateGauge("Alloc", 1.5)
	assert.Equal(t, map[string]float64{"Alloc": 1.5}, newFileStorage(t, path, periodicInterval, true).Gauges())

	assert.Equal(t, int64(7), s.UpdateCounter("PollCount", 7))
	assert.Equal(t, map[string]int64{"PollCount": 7}, newFileStorage(t, path, periodicInterval, true).Counters())
}

func TestFileStorageWithoutSyncWriteDoesNotWrite(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, periodicInterval, false)

	s.UpdateGauge("Alloc", 1.5)

	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFileStorageSyncWriteConcurrent(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, syncInterval, false)

	const workers, updates = 8, 20

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range updates {
				s.UpdateCounter("PollCount", 1)
			}
		})
	}
	wg.Wait()

	restored := newFileStorage(t, path, periodicInterval, true)
	assert.Equal(t, map[string]int64{"PollCount": workers * updates}, restored.Counters())
}

func TestFileStorageSaveReplacesContent(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, periodicInterval, false)

	s.UpdateGauge("Alloc", 1.5)
	s.UpdateCounter("PollCount", 1)
	require.NoError(t, s.Save())

	require.NoError(t, s.restore(nil))
	s.UpdateGauge("Alloc", 2.5)
	require.NoError(t, s.Save())

	restored := newFileStorage(t, path, periodicInterval, true)
	assert.Equal(t, map[string]float64{"Alloc": 2.5}, restored.Gauges())
	assert.Empty(t, restored.Counters())
}

func TestFileStorageSaveError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	s := newFileStorage(t, path, periodicInterval, false)
	s.UpdateGauge("Alloc", 1.5)

	// Каталог на месте целевого файла: заменить его файлом нельзя.
	require.NoError(t, os.Mkdir(path, 0o755))

	assert.Error(t, s.Save())
	assert.True(t, s.dirty.Load(), "после неудачного сохранения метрики остаются несохранёнными")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "временный файл должен удаляться при ошибке")
	assert.Equal(t, "metrics.json", entries[0].Name())
}

func TestFileStorageSaveKeepsPermissions(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, periodicInterval, false)
	s.UpdateGauge("Alloc", 1.5)
	require.NoError(t, s.Save())

	info, err := os.Stat(path)
	require.NoError(t, err)

	// Права такие же, как дал бы os.WriteFile с 0o644 при текущем umask.
	want := filepath.Join(t.TempDir(), "want")
	require.NoError(t, os.WriteFile(want, nil, 0o644))
	wantInfo, err := os.Stat(want)
	require.NoError(t, err)

	assert.Equal(t, wantInfo.Mode().Perm(), info.Mode().Perm())
}

func TestFileStorageSaveOverwritesStaleTemp(t *testing.T) {
	path := tempPath(t)
	require.NoError(t, os.WriteFile(path+".tmp", []byte("garbage"), 0o644))

	s := newFileStorage(t, path, periodicInterval, false)
	s.UpdateCounter("PollCount", 1)
	require.NoError(t, s.Save())

	assert.Equal(t, map[string]int64{"PollCount": 1}, newFileStorage(t, path, periodicInterval, true).Counters())

	_, err := os.Stat(path + ".tmp")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFileStorageEmptyPathDisablesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	s := newFileStorage(t, "", syncInterval, true)
	s.UpdateGauge("Alloc", 1.5)
	require.NoError(t, s.Save())

	value, ok := s.Gauge("Alloc")
	assert.True(t, ok)
	assert.Equal(t, 1.5, value)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func runInBackground(t *testing.T, s *FileStorage) (cancel func(), done <-chan struct{}) {
	t.Helper()

	ctx, cancelCtx := context.WithCancel(context.Background())
	finished := make(chan struct{})

	go func() {
		s.Run(ctx)
		close(finished)
	}()

	t.Cleanup(cancelCtx)

	return cancelCtx, finished
}

func waitDone(t *testing.T, done <-chan struct{}, msg string) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(msg)
	}
}

func TestFileStorageRunSavesPeriodically(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, 10*time.Millisecond, false)
	s.UpdateCounter("PollCount", 3)

	cancel, done := runInBackground(t, s)

	assert.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, time.Second, 5*time.Millisecond)

	cancel()
	waitDone(t, done, "Run не завершился после отмены контекста")

	assert.Equal(t, map[string]int64{"PollCount": 3}, newFileStorage(t, path, periodicInterval, true).Counters())
}

func TestFileStorageRunSkipsUnchanged(t *testing.T) {
	path := tempPath(t)
	s := newFileStorage(t, path, 5*time.Millisecond, false)

	fileExists := func() bool {
		_, err := os.Stat(path)
		return err == nil
	}

	cancel, done := runInBackground(t, s)

	assert.Never(t, fileExists, 50*time.Millisecond, 5*time.Millisecond, "без изменений файл не создаётся")

	s.UpdateGauge("Alloc", 1)
	assert.Eventually(t, fileExists, time.Second, 5*time.Millisecond)

	// Подменяем файл меткой: если Run перезапишет его без новых изменений,
	// метка пропадёт. Так не зависим от точности ModTime файловой системы.
	const sentinel = "sentinel"
	require.NoError(t, os.WriteFile(path, []byte(sentinel), 0o644))

	assert.Never(t, func() bool {
		data, err := os.ReadFile(path)
		return err != nil || string(data) != sentinel
	}, 50*time.Millisecond, 5*time.Millisecond, "без новых изменений файл не перезаписывается")

	cancel()
	waitDone(t, done, "Run не завершился после отмены контекста")
}

func TestFileStorageRunReturnsWithoutPeriodicMode(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		interval time.Duration
	}{
		{"синхронная запись", tempPath(t), syncInterval},
		{"без файла", "", periodicInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newFileStorage(t, tt.path, tt.interval, false)

			_, done := runInBackground(t, s)
			waitDone(t, done, "Run должен сразу вернуться")
		})
	}
}
