package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

// stubBackupCheckpointer 是 BackupCheckpointer 的测试替身。
// 在 destDir 中写入标记文件以确认调用已发生。
type stubBackupCheckpointer struct {
	called    atomic.Int32
	markerErr error // 如果非 nil，Checkpoint 返回此错误
}

func (s *stubBackupCheckpointer) Checkpoint(destDir string) error {
	s.called.Add(1)
	if s.markerErr != nil {
		return s.markerErr
	}
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, "checkpoint.marker"), []byte("ok"), 0600)
}

// TestNewBackupScheduler_DisabledWhenNoInterval 验证当 Interval 为零时 NewBackupScheduler 返回 nil。
func TestNewBackupScheduler_DisabledWhenNoInterval(t *testing.T) {
	stub := &stubBackupCheckpointer{}
	sched := NewBackupScheduler(BackupConfig{Interval: 0, BackupDir: "/tmp/backups", Retain: 5}, stub)
	if sched != nil {
		t.Fatalf("预期当 Interval 为零时返回 nil，但得到非 nil")
	}
}

// TestBackupRunOnce_CreatesCheckpoint 验证 backupRunOnce 创建带有预期时间戳名称的备份目录，
// 并在其中调用 Checkpoint。
func TestBackupRunOnce_CreatesCheckpoint(t *testing.T) {
	dir := t.TempDir()
	stub := &stubBackupCheckpointer{}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    5,
		DataDir:   "",
	}, stub)
	if sched == nil {
		t.Fatal("预期非 nil 调度器")
	}

	before := time.Now().UTC()
	sched.backupRunOnce()
	after := time.Now().UTC()

	if stub.called.Load() != 1 {
		t.Fatalf("预期 Checkpoint 被调用一次，实际调用 %d 次", stub.called.Load())
	}

	// 验证备份目录已创建
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取备份目录: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("预期 1 个备份目录，实际 %d 个", len(entries))
	}

	name := entries[0].Name()

	// 名称必须以 "backup-" 开头并包含有效时间戳
	const prefix = "backup-"
	if len(name) <= len(prefix) {
		t.Fatalf("意外的备份目录名: %q", name)
	}
	tsStr := name[len(prefix):]
	ts, err := time.Parse("20060102-150405", tsStr)
	if err != nil {
		t.Fatalf("备份目录时间戳解析错误 %q: %v", tsStr, err)
	}

	// 解析的时间戳应在运行窗口内（允许 1 秒偏差）
	if ts.Before(before.Add(-time.Second)) || ts.After(after.Add(time.Second)) {
		t.Fatalf("时间戳 %v 不在预期范围 [%v, %v] 内", ts, before, after)
	}

	// 验证 pebble 子目录和标记文件存在
	markerPath := filepath.Join(dir, name, "pebble", "checkpoint.marker")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("标记文件缺失 %s: %v", markerPath, err)
	}
}

// TestBackupPruneOldBackups_KeepsRetainCount 创建 7 个备份目录，设置 retain=3，
// 运行 backupRunOnce（添加第 8 个），验证只保留 3 个。
func TestBackupPruneOldBackups_KeepsRetainCount(t *testing.T) {
	dir := t.TempDir()
	stub := &stubBackupCheckpointer{}

	const retain = 3

	// 预创建 7 个带有递增时间戳的备份目录，
	// 确保清理时最旧的先被删除
	for i := 0; i < 7; i++ {
		ts := time.Date(2024, 1, i+1, 12, 0, 0, 0, time.UTC)
		name := fmt.Sprintf("backup-%s", ts.Format("20060102-150405"))
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    retain,
		DataDir:   "",
	}, stub)

	sched.backupRunOnce()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取备份目录: %v", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	if len(names) != retain {
		t.Fatalf("清理后预期 %d 个备份目录，实际 %d 个: %v", retain, len(names), names)
	}
}

// TestBackupGetStatus_ReflectsLastRun 验证 backupRunOnce 完成后状态字段被正确填充。
func TestBackupGetStatus_ReflectsLastRun(t *testing.T) {
	dir := t.TempDir()
	stub := &stubBackupCheckpointer{}

	cfg := BackupConfig{
		Interval:  5 * time.Minute,
		BackupDir: dir,
		Retain:    5,
		DataDir:   "",
	}
	sched := NewBackupScheduler(cfg, stub)

	// 首次运行前的状态：已启用但无上次运行信息
	before := sched.GetBackupStatus()
	if !before.Enabled {
		t.Fatal("首次运行前预期 Enabled=true")
	}
	if !before.LastRunAt.IsZero() {
		t.Fatalf("首次运行前预期零值 LastRunAt，实际 %v", before.LastRunAt)
	}

	runStart := time.Now()
	sched.backupRunOnce()
	runEnd := time.Now()

	st := sched.GetBackupStatus()

	if !st.Enabled {
		t.Error("预期 Enabled=true")
	}
	if st.Interval != cfg.Interval.String() {
		t.Errorf("预期 Interval=%q，实际 %q", cfg.Interval.String(), st.Interval)
	}
	if st.BackupDir != cfg.BackupDir {
		t.Errorf("预期 BackupDir=%q，实际 %q", cfg.BackupDir, st.BackupDir)
	}
	if st.Retain != cfg.Retain {
		t.Errorf("预期 Retain=%d，实际 %d", cfg.Retain, st.Retain)
	}
	if !st.LastRunOK {
		t.Errorf("预期 LastRunOK=true，实际 false（错误: %s）", st.LastError)
	}
	if st.LastRunAt.Before(runStart) || st.LastRunAt.After(runEnd) {
		t.Errorf("LastRunAt %v 超出运行窗口 [%v, %v]", st.LastRunAt, runStart, runEnd)
	}
	if st.LastElapsed == "" {
		t.Error("预期非空的 LastElapsed")
	}
	if st.LastError != "" {
		t.Errorf("预期空的 LastError，实际 %q", st.LastError)
	}
}

// TestBackupScheduler_Start 验证调度器通过极短的间隔在协程中触发，
// 且引擎 Checkpoint 至少被调用一次。
func TestBackupScheduler_Start(t *testing.T) {
	dir := t.TempDir()
	stub := &stubBackupCheckpointer{}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  50 * time.Millisecond,
		BackupDir: dir,
		Retain:    10,
		DataDir:   "",
	}, stub)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := sched.Start(ctx)
	<-ctx.Done()
	<-done // 等待协程完成正在进行的 backupRunOnce，然后再清理临时目录

	if stub.called.Load() == 0 {
		t.Fatal("预期调度器协程至少调用一次 Checkpoint")
	}
}

// TestBackupRunOnce_CheckpointError 验证当 Checkpoint 返回错误时，
// 调度器记录 LastRunOK=false 并填充 LastError。
func TestBackupRunOnce_CheckpointError(t *testing.T) {
	dir := t.TempDir()
	stub := &stubBackupCheckpointer{
		markerErr: fmt.Errorf("磁盘已满"),
	}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    5,
		DataDir:   "",
	}, stub)

	sched.backupRunOnce()

	st := sched.GetBackupStatus()
	if st.LastRunOK {
		t.Error("检查点错误后预期 LastRunOK=false")
	}
	if st.LastError == "" {
		t.Error("检查点错误后预期 LastError 已设置")
	}
	if st.LastRunAt.IsZero() {
		t.Error("检查点错误后预期 LastRunAt 已设置")
	}
}

// TestBackupRunOnce_MkdirAllFailure 验证当备份目录无法创建时
// （例如 BackupDir 是已有文件而非目录），调度器记录错误且不会 panic。
func TestBackupRunOnce_MkdirAllFailure(t *testing.T) {
	tmp := t.TempDir()
	blockingFile := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0600); err != nil {
		t.Fatalf("设置: %v", err)
	}

	stub := &stubBackupCheckpointer{}
	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: filepath.Join(blockingFile, "subdir"), // 路径穿过一个文件
		Retain:    5,
		DataDir:   "",
	}, stub)

	sched.backupRunOnce()

	st := sched.GetBackupStatus()
	if st.LastRunOK {
		t.Error("MkdirAll 失败时预期 LastRunOK=false")
	}
	if st.LastError == "" {
		t.Error("MkdirAll 失败时预期 LastError 非空")
	}
	// Checkpoint 不应被尝试调用
	if stub.called.Load() != 0 {
		t.Errorf("预期 Checkpoint 未被调用，实际调用 %d 次", stub.called.Load())
	}
}

// TestBackupCopyFile_Roundtrip 创建源文件，复制后验证目标文件内容一致且权限正确。
func TestBackupCopyFile_Roundtrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.txt")
	dst := filepath.Join(t.TempDir(), "dest.txt")

	content := []byte("hello muninndb backup")
	if err := os.WriteFile(src, content, 0640); err != nil {
		t.Fatalf("写入源文件: %v", err)
	}

	if err := backupCopyFile(src, dst); err != nil {
		t.Fatalf("backupCopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("读取目标文件: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("内容不匹配: 实际 %q, 预期 %q", got, content)
	}

	// 验证文件权限被保留
	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(dst)
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("权限不匹配: 源 %v, 目标 %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

// TestBackupCopyFile_SrcMissing 验证当源文件不存在时 backupCopyFile 返回错误。
func TestBackupCopyFile_SrcMissing(t *testing.T) {
	src := filepath.Join(t.TempDir(), "nonexistent.txt")
	dst := filepath.Join(t.TempDir(), "dest.txt")

	if err := backupCopyFile(src, dst); err == nil {
		t.Error("源文件缺失时预期返回错误，实际返回 nil")
	}
}

// TestBackupCopyDir_Roundtrip 创建多级目录树及文件，复制后验证所有文件存在且内容正确。
func TestBackupCopyDir_Roundtrip(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "copy")

	files := map[string]string{
		"a.txt":          "alpha",
		"sub/b.txt":      "beta",
		"sub/deep/c.txt": "gamma",
	}

	for relPath, content := range files {
		full := filepath.Join(srcDir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	if err := backupCopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("backupCopyDir: %v", err)
	}

	for relPath, wantContent := range files {
		dstFile := filepath.Join(dstDir, relPath)
		got, err := os.ReadFile(dstFile)
		if err != nil {
			t.Errorf("缺失文件 %s: %v", dstFile, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("文件 %s: 实际 %q, 预期 %q", relPath, got, wantContent)
		}
	}
}

// TestBackupDirSize 验证 backupDirSize 正确计算目录树中所有文件的总字节大小。
func TestBackupDirSize(t *testing.T) {
	dir := t.TempDir()

	files := map[string][]byte{
		"file1.txt":     []byte("hello"),   // 5 字节
		"sub/file2.txt": []byte("world!!"), // 7 字节
	}
	var expectedTotal int64
	for relPath, content := range files {
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, content, 0600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
		expectedTotal += int64(len(content))
	}

	got := backupDirSize(dir)
	if got != expectedTotal {
		t.Errorf("backupDirSize: 实际 %d, 预期 %d", got, expectedTotal)
	}
}

// TestBackupDirSize_EmptyDir 验证 backupDirSize 对空目录返回 0。
func TestBackupDirSize_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if got := backupDirSize(dir); got != 0 {
		t.Errorf("空目录预期返回 0，实际 %d", got)
	}
}

// TestBackupPruneOldBackups_EmptyDir 验证备份目录为空时 backupPruneOldBackups 返回 0。
func TestBackupPruneOldBackups_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    3,
	}, &stubBackupCheckpointer{})

	pruned := sched.backupPruneOldBackups()
	if pruned != 0 {
		t.Errorf("空目录预期清理 0 个，实际 %d 个", pruned)
	}
}

// TestBackupPruneOldBackups_UnderRetain 验证当备份目录数量少于等于 cfg.Retain 时不会删除任何内容。
func TestBackupPruneOldBackups_UnderRetain(t *testing.T) {
	dir := t.TempDir()
	const retain = 5

	// 创建 3 个备份目录 — 少于 retain
	for i := 0; i < 3; i++ {
		ts := time.Date(2025, 1, i+1, 10, 0, 0, 0, time.UTC)
		name := fmt.Sprintf("backup-%s", ts.Format("20060102-150405"))
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    retain,
	}, &stubBackupCheckpointer{})

	pruned := sched.backupPruneOldBackups()
	if pruned != 0 {
		t.Errorf("低于保留阈值时预期清理 0 个，实际 %d 个", pruned)
	}

	// 所有 3 个目录必须仍然存在
	entries, _ := os.ReadDir(dir)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 3 {
		t.Errorf("预期保留 3 个目录，实际 %d 个: %v", len(dirs), dirs)
	}
}

// TestBackupPruneOldBackups_ZeroRetain 验证当 Retain 为 0（已禁用）时，
// backupPruneOldBackups 完全跳过清理，保留所有目录。
func TestBackupPruneOldBackups_ZeroRetain(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 4; i++ {
		ts := time.Date(2025, 3, i+1, 8, 0, 0, 0, time.UTC)
		name := fmt.Sprintf("backup-%s", ts.Format("20060102-150405"))
		if err := os.MkdirAll(filepath.Join(dir, name), 0700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: dir,
		Retain:    0,
	}, &stubBackupCheckpointer{})

	pruned := sched.backupPruneOldBackups()
	if pruned != 0 {
		t.Errorf("Retain=0 时预期清理 0 个，实际 %d 个", pruned)
	}
}

// TestBackupRunOnce_CopiesAuxFiles 验证当 DataDir 包含 wal/ 子目录和 auth_secret 文件时，
// backupRunOnce 将两者都复制到备份中。
func TestBackupRunOnce_CopiesAuxFiles(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := t.TempDir()

	// 创建 wal/ 目录及日志文件
	walDir := filepath.Join(dataDir, "wal")
	if err := os.MkdirAll(walDir, 0700); err != nil {
		t.Fatalf("mkdir wal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(walDir, "000001.log"), []byte("wal data"), 0600); err != nil {
		t.Fatalf("write wal file: %v", err)
	}

	// 创建 auth_secret 文件
	secretContent := []byte("supersecret")
	if err := os.WriteFile(filepath.Join(dataDir, "auth_secret"), secretContent, 0600); err != nil {
		t.Fatalf("write auth_secret: %v", err)
	}

	stub := &stubBackupCheckpointer{}
	sched := NewBackupScheduler(BackupConfig{
		Interval:  time.Hour,
		BackupDir: backupDir,
		Retain:    5,
		DataDir:   dataDir,
	}, stub)

	sched.backupRunOnce()

	// 找到创建的备份目录
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("读取备份目录: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("预期 1 个备份目录，实际 %d 个", len(entries))
	}
	backupName := entries[0].Name()
	backupPath := filepath.Join(backupDir, backupName)

	// 验证 wal/ 已复制及其内容
	copiedWalFile := filepath.Join(backupPath, "wal", "000001.log")
	gotWal, err := os.ReadFile(copiedWalFile)
	if err != nil {
		t.Errorf("wal 文件未复制: %v", err)
	} else if string(gotWal) != "wal data" {
		t.Errorf("wal 文件内容不匹配: 实际 %q", gotWal)
	}

	// 验证 auth_secret 已复制且内容正确
	copiedSecret := filepath.Join(backupPath, "auth_secret")
	gotSecret, err := os.ReadFile(copiedSecret)
	if err != nil {
		t.Errorf("auth_secret 未复制: %v", err)
	} else if string(gotSecret) != string(secretContent) {
		t.Errorf("auth_secret 内容不匹配: 实际 %q, 预期 %q", gotSecret, secretContent)
	}

	// 验证整体运行标记为成功
	st := sched.GetBackupStatus()
	if !st.LastRunOK {
		t.Errorf("预期 LastRunOK=true，实际 false: %s", st.LastError)
	}
}
