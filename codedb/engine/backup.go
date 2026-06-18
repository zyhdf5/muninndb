package engine

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// BackupCheckpointer 由存储引擎实现，用于在指定目标目录创建数据检查点。
type BackupCheckpointer interface {
	Checkpoint(destDir string) error
}

// BackupConfig 保存备份调度器的配置信息。
type BackupConfig struct {
	Interval  time.Duration // 备份间隔时间
	BackupDir string        // 备份存储目录
	Retain    int           // 保留备份数量
	DataDir   string        // 数据目录，用于辅助文件复制（如 wal、auth_secret）
}

// BackupStatus 是调度器当前状态的快照。
type BackupStatus struct {
	Enabled       bool      `json:"enabled"`                   // 是否启用
	Interval      string    `json:"interval,omitempty"`        // 备份间隔
	BackupDir     string    `json:"backup_dir,omitempty"`      // 备份目录
	Retain        int       `json:"retain"`                    // 保留数量
	LastRunAt     time.Time `json:"last_run_at,omitempty"`     // 上次运行时间
	LastRunOK     bool      `json:"last_run_ok"`               // 上次运行是否成功
	LastError     string    `json:"last_error,omitempty"`      // 上次错误信息
	LastSizeBytes int64     `json:"last_size_bytes,omitempty"` // 上次备份大小（字节）
	LastElapsed   string    `json:"last_elapsed,omitempty"`    // 上次备份耗时
	NextRunAt     time.Time `json:"next_run_at,omitempty"`     // 下次运行时间
	PrunedCount   int       `json:"pruned_count,omitempty"`    // 清理的旧备份数量
}

// BackupScheduler 运行周期性数据库备份任务。
type BackupScheduler struct {
	cfg BackupConfig
	eng BackupCheckpointer

	mu          sync.RWMutex
	lastRunAt   time.Time
	lastRunOK   bool
	lastError   string
	lastSize    int64
	lastElapsed string
	nextRunAt   time.Time
	prunedCount int
}

// NewBackupScheduler 创建新的备份调度器。如果 cfg.Interval 为零则返回 nil（表示已禁用）。
func NewBackupScheduler(cfg BackupConfig, eng BackupCheckpointer) *BackupScheduler {
	if cfg.Interval == 0 {
		return nil
	}
	return &BackupScheduler{cfg: cfg, eng: eng}
}

// Start 启动备份协程，接受上下文取消控制。
// 返回的通道在协程完全停止后关闭。
func (s *BackupScheduler) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.backupRun(ctx)
	}()
	return done
}

// GetBackupStatus 返回调度器当前状态的线程安全快照。
func (s *BackupScheduler) GetBackupStatus() BackupStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := BackupStatus{
		Enabled:       true,
		Interval:      s.cfg.Interval.String(),
		BackupDir:     s.cfg.BackupDir,
		Retain:        s.cfg.Retain,
		LastRunAt:     s.lastRunAt,
		LastRunOK:     s.lastRunOK,
		LastError:     s.lastError,
		LastSizeBytes: s.lastSize,
		LastElapsed:   s.lastElapsed,
		NextRunAt:     s.nextRunAt,
		PrunedCount:   s.prunedCount,
	}
	return st
}

// backupRun 是调度器的主循环。
func (s *BackupScheduler) backupRun(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	s.mu.Lock()
	s.nextRunAt = time.Now().Add(s.cfg.Interval)
	s.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			_ = t
			s.backupRunOnce()

			s.mu.Lock()
			s.nextRunAt = time.Now().Add(s.cfg.Interval)
			s.mu.Unlock()
		}
	}
}

// backupRunOnce 执行一次完整的备份周期：检查点创建、辅助文件复制、旧备份清理。
func (s *BackupScheduler) backupRunOnce() {
	start := time.Now()
	ts := start.UTC().Format("20060102-150405")
	destDir := filepath.Join(s.cfg.BackupDir, "backup-"+ts)

	slog.Info("backup: 计划备份开始", "dest", destDir)

	if err := os.MkdirAll(destDir, 0700); err != nil {
		s.backupRecordError(start, fmt.Errorf("创建备份目录: %w", err))
		return
	}

	checkpointDir := filepath.Join(destDir, "pebble")
	if err := s.eng.Checkpoint(checkpointDir); err != nil {
		os.RemoveAll(destDir)
		s.backupRecordError(start, fmt.Errorf("pebble 检查点: %w", err))
		return
	}
	slog.Info("backup: pebble 检查点完成", "dir", checkpointDir)

	if s.cfg.DataDir != "" {
		walSrc := filepath.Join(s.cfg.DataDir, "wal")
		walDst := filepath.Join(destDir, "wal")
		if info, err := os.Stat(walSrc); err == nil && info.IsDir() {
			if err := backupCopyDir(walSrc, walDst); err != nil {
				slog.Warn("backup: 复制 wal 目录失败", "err", err)
			}
		}

		secretSrc := filepath.Join(s.cfg.DataDir, "auth_secret")
		secretDst := filepath.Join(destDir, "auth_secret")
		if _, err := os.Stat(secretSrc); err == nil {
			if err := backupCopyFile(secretSrc, secretDst); err != nil {
				slog.Warn("backup: 复制 auth_secret 失败", "err", err)
			}
		}
	}

	elapsed := time.Since(start)
	size := backupDirSize(destDir)

	pruned := s.backupPruneOldBackups()

	slog.Info("backup: 完成",
		"dest", destDir,
		"size_bytes", size,
		"elapsed", elapsed.Round(time.Millisecond),
		"pruned", pruned,
	)

	s.mu.Lock()
	s.lastRunAt = start
	s.lastRunOK = true
	s.lastError = ""
	s.lastSize = size
	s.lastElapsed = elapsed.Round(time.Millisecond).String()
	s.prunedCount = pruned
	s.mu.Unlock()
}

// backupRecordError 在备份失败后更新状态字段。
func (s *BackupScheduler) backupRecordError(start time.Time, err error) {
	slog.Error("backup: 计划备份失败", "err", err)
	s.mu.Lock()
	s.lastRunAt = start
	s.lastRunOK = false
	s.lastError = err.Error()
	s.mu.Unlock()
}

// backupPruneOldBackups 移除超过 cfg.Retain 数量的最旧备份目录。
// 返回删除的目录数量。
func (s *BackupScheduler) backupPruneOldBackups() int {
	if s.cfg.Retain <= 0 {
		return 0
	}

	entries, err := os.ReadDir(s.cfg.BackupDir)
	if err != nil {
		slog.Warn("backup: 读取备份目录失败，无法清理", "err", err)
		return 0
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	// 按名称升序排列 — "backup-YYYYMMDD-HHMMSS" 格式在字典序下正确排列，
	// 最旧的排在前面。
	sort.Strings(dirs)

	pruned := 0
	excess := len(dirs) - s.cfg.Retain
	for i := 0; i < excess; i++ {
		target := filepath.Join(s.cfg.BackupDir, dirs[i])
		if err := os.RemoveAll(target); err != nil {
			slog.Warn("backup: 清理旧备份失败", "dir", target, "err", err)
		} else {
			slog.Info("backup: 已清理旧备份", "dir", target)
			pruned++
		}
	}
	return pruned
}

// backupCopyFile 复制单个文件，保留其权限。
func backupCopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// backupCopyDir 递归复制目录树，从 src 到 dst。
func backupCopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		return backupCopyFile(path, target)
	})
}

// backupDirSize 返回指定目录下所有文件的总字节大小。
func backupDirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
