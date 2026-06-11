package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
	"synapseqa/backend/internal/repository"
)

// SystemService 承载系统管理业务规则，Controller 不直接访问数据库。
type SystemService struct {
	repo      *repository.SystemRepository
	jwtSecret []byte
}

func NewSystemService(repo *repository.SystemRepository, jwtSecret []byte) *SystemService {
	return &SystemService{repo: repo, jwtSecret: jwtSecret}
}

func (s *SystemService) Login(ctx context.Context, req model.LoginRequest, ip string) (map[string]any, error) {
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	u, passwordHash, err := s.repo.FindUserByUsername(ctx, req.Username)
	if err != nil || passwordHash != hashPassword(req.Password) || u.Status != "active" {
		return nil, errors.New("用户名或密码错误")
	}
	if err := s.repo.RecordLogin(ctx, u.ID, ip); err != nil {
		return nil, errors.New("记录登录状态失败")
	}
	token, err := s.signToken(model.Claims{UserID: u.ID, Username: u.Username, ExpiresAt: time.Now().Add(24 * time.Hour).Unix()})
	if err != nil {
		return nil, errors.New("生成登录凭证失败")
	}
	_ = s.repo.LogOperation(ctx, u.Username, "登录系统", "Synapse QA")
	return map[string]any{"token": token, "user": u}, nil
}

func (s *SystemService) Register(ctx context.Context, req model.RegisterRequest) error {
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Email = strings.TrimSpace(req.Email)
	if req.Username == "" || req.Password == "" || req.DisplayName == "" {
		return errors.New("用户名、密码和昵称不能为空")
	}
	if len(req.Password) < 6 {
		return errors.New("密码长度至少 6 位")
	}
	if err := s.repo.CreateUser(ctx, req, hashPassword(req.Password), generateAPIKey(req.Username)); err != nil {
		return errors.New("注册失败，用户名可能已存在")
	}
	_ = s.repo.LogOperation(ctx, req.Username, "注册账号", req.DisplayName)
	return nil
}

func (s *SystemService) ParseToken(token string) (model.Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return model.Claims{}, errors.New("token 格式无效")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return model.Claims{}, err
	}
	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return model.Claims{}, errors.New("token 签名无效")
	}
	var c model.Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return model.Claims{}, err
	}
	if c.ExpiresAt < time.Now().Unix() {
		return model.Claims{}, errors.New("token 已过期")
	}
	return c, nil
}

func (s *SystemService) CurrentUser(ctx context.Context, username string) (model.User, error) {
	u, _, err := s.repo.FindUserByUsername(ctx, username)
	return u, err
}

func (s *SystemService) Overview(ctx context.Context) map[string]int64 {
	return map[string]int64{
		"users":        s.repo.Count(ctx, "users"),
		"roles":        s.repo.Count(ctx, "roles"),
		"uiAssets":     s.repo.Count(ctx, "ui_assets"),
		"projects":     s.repo.Count(ctx, "projects"),
		"products":     s.repo.Count(ctx, "products"),
		"testObjects":  s.repo.Count(ctx, "test_objects"),
		"pageElements": s.repo.Count(ctx, "page_elements"),
	}
}

func (s *SystemService) ListUsers(ctx context.Context, id, nickname, account string, page, pageSize int) (model.PageResult, error) {
	page = positiveInt(strconv.Itoa(page), 1)
	pageSize = positiveInt(strconv.Itoa(pageSize), 20)
	items, total, err := s.repo.ListUsers(ctx, strings.TrimSpace(id), strings.TrimSpace(nickname), strings.TrimSpace(account), page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *SystemService) UpdateUser(ctx context.Context, actor string, id int64, req model.UserUpdateRequest) error {
	oldStatus, oldName, oldEmail, oldRole, username, err := s.repo.GetUserEditState(ctx, id)
	if err != nil {
		return errors.New("用户不存在")
	}
	status, displayName, email := oldStatus, oldName, oldEmail
	if req.Status != nil {
		status = *req.Status
	}
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Email != nil {
		email = strings.TrimSpace(*req.Email)
	}
	var roleID any
	if oldRole.Valid {
		roleID = oldRole.Int64
	}
	if req.RoleID != nil {
		if *req.RoleID == 0 {
			roleID = nil
		} else {
			roleID = *req.RoleID
		}
	}
	rows, err := s.repo.UpdateUser(ctx, id, status, displayName, email, roleID)
	if err != nil || rows == 0 {
		return errors.New("更新用户失败")
	}
	_ = s.repo.LogOperation(ctx, actor, "编辑用户", username)
	return nil
}

func (s *SystemService) DeleteUser(ctx context.Context, actor string, id int64) error {
	username, rows, err := s.repo.DeleteUser(ctx, id)
	if err != nil || rows == 0 {
		return errors.New("用户不存在")
	}
	_ = s.repo.LogOperation(ctx, actor, "删除用户", username)
	return nil
}

func (s *SystemService) ListRoles(ctx context.Context) ([]model.Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *SystemService) CreateRole(ctx context.Context, actor string, req model.RoleRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || req.Description == "" {
		return errors.New("角色名称和角色描述不能为空")
	}
	if err := s.repo.CreateRole(ctx, req); err != nil {
		return errors.New("新增角色失败，角色名称可能已存在")
	}
	_ = s.repo.LogOperation(ctx, actor, "新增角色", req.Name)
	return nil
}

func (s *SystemService) UpdateRole(ctx context.Context, actor string, id int64, req model.RoleRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || req.Description == "" {
		return errors.New("角色名称和角色描述不能为空")
	}
	if err := s.repo.UpdateRole(ctx, id, req); err != nil {
		return errors.New("更新角色失败，角色名称可能已存在")
	}
	_ = s.repo.LogOperation(ctx, actor, "编辑角色", req.Name)
	return nil
}

func (s *SystemService) DeleteRole(ctx context.Context, actor string, id int64) error {
	code, name, err := s.repo.FindRoleCodeName(ctx, id)
	if err != nil {
		return errors.New("角色不存在")
	}
	if code == "admin" {
		return errors.New("内置管理员角色不能删除")
	}
	if s.repo.CountRoleUsers(ctx, id) > 0 {
		return errors.New("该角色仍有关联用户，不能删除")
	}
	if err := s.repo.DeleteRole(ctx, id); err != nil {
		return errors.New("删除角色失败")
	}
	_ = s.repo.LogOperation(ctx, actor, "删除角色", name)
	return nil
}

func (s *SystemService) signToken(c model.Claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte("synapse:" + password))
	return hex.EncodeToString(sum[:])
}

func generateAPIKey(seed string) string {
	sum := sha256.Sum256([]byte(seed + ":" + strconv.FormatInt(time.Now().UnixNano(), 10)))
	return "mango_" + hex.EncodeToString(sum[:])[:24]
}

func positiveInt(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
