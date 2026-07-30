package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

const maxAPITempFileSize = 10 << 20

func (s *APIAutomationService) UploadTempFile(ctx context.Context, userID, projectID int64, name, mimeType string, size int64, source io.Reader) (model.APITempFile, error) {
	if projectID <= 0 || !s.repo.CanAccessProject(ctx, userID, projectID) {
		return model.APITempFile{}, errors.New("项目不存在或无权访问")
	}
	if size < 0 || size > maxAPITempFileSize {
		return model.APITempFile{}, errors.New("临时文件不能超过 10 MB")
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." {
		return model.APITempFile{}, errors.New("文件名不能为空")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	root := filepath.Join(os.TempDir(), "synapse-qa", "api-temp-files")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return model.APITempFile{}, errors.New("创建临时文件目录失败")
	}
	id, err := randomAPITempFileID()
	if err != nil {
		return model.APITempFile{}, errors.New("生成临时文件标识失败")
	}
	storedPath := filepath.Join(root, id)
	output, err := os.OpenFile(storedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return model.APITempFile{}, errors.New("保存临时文件失败")
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(source, maxAPITempFileSize+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || written > maxAPITempFileSize || written != size {
		_ = os.Remove(storedPath)
		return model.APITempFile{}, errors.New("保存临时文件失败或文件大小不一致")
	}
	now := time.Now()
	item := model.APITempFile{
		ID: id, ProjectID: projectID, OriginalName: name, MIMEType: mimeType,
		SizeBytes: written, SHA256: hex.EncodeToString(hash.Sum(nil)),
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := s.repo.CreateTempFile(ctx, item, userID, storedPath); err != nil {
		_ = os.Remove(storedPath)
		return model.APITempFile{}, errors.New("记录临时文件失败")
	}
	s.cleanupExpiredTempFiles(ctx)
	return item, nil
}

func (s *APIAutomationService) DeleteTempFile(ctx context.Context, userID int64, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("临时文件标识不能为空")
	}
	path, projectID, err := s.repo.DeleteTempFile(ctx, id, userID)
	if err != nil || !s.repo.CanAccessProject(ctx, userID, projectID) {
		return errors.New("临时文件不存在或无权访问")
	}
	if safeAPITempPath(path) {
		_ = os.Remove(path)
	}
	return nil
}

func (s *APIAutomationService) cleanupExpiredTempFiles(ctx context.Context) {
	paths, err := s.repo.ExpiredTempFiles(ctx)
	if err != nil {
		return
	}
	for _, path := range paths {
		if safeAPITempPath(path) {
			_ = os.Remove(path)
		}
	}
}

func safeAPITempPath(path string) bool {
	root := filepath.Join(os.TempDir(), "synapse-qa", "api-temp-files")
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}

func randomAPITempFileID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
