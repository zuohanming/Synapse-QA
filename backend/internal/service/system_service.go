package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return nil, errors.New("账号已锁定，请稍后再试")
	}
	if passwordHash != hashPassword(req.Password) {
		failureLimit, lockMinutes := s.loginSecurityPolicy(ctx)
		_ = s.repo.RecordLoginFailure(ctx, req.Username, failureLimit, lockMinutes)
		return nil, errors.New("用户名或密码错误")
	}
	if u.Status != "active" {
		return nil, errors.New("用户名或密码错误")
	}
	if err := s.repo.RecordLogin(ctx, u.ID, ip); err != nil {
		return nil, errors.New("记录登录状态失败")
	}
	_, u.RoleCode, u.AuthVersion, u.Permissions, err = s.repo.UserAccess(ctx, u.ID)
	if err != nil {
		return nil, errors.New("读取用户权限失败")
	}
	token, err := s.signToken(model.Claims{UserID: u.ID, Username: u.Username, AuthVersion: u.AuthVersion, ExpiresAt: time.Now().Add(s.sessionDuration(ctx)).Unix()})
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
	if err := s.validatePassword(ctx, req.Password); err != nil {
		return err
	}
	if err := s.repo.CreateUser(ctx, req, hashPassword(req.Password), generateAPIKey(req.Username)); err != nil {
		return errors.New("注册失败，用户名可能已存在")
	}
	_ = s.repo.LogOperation(ctx, req.Username, "注册账号", req.DisplayName)
	return nil
}

func (s *SystemService) ChangePassword(ctx context.Context, claims model.Claims, req model.ChangePasswordRequest) (map[string]any, error) {
	user, passwordHash, err := s.repo.FindUserByUsername(ctx, claims.Username)
	if err != nil || passwordHash != hashPassword(req.CurrentPassword) {
		return nil, errors.New("当前密码不正确")
	}
	if err := s.validatePassword(ctx, req.NewPassword); err != nil {
		return nil, err
	}
	if hashPassword(req.NewPassword) == passwordHash {
		return nil, errors.New("新密码不能与当前密码相同")
	}
	authVersion, err := s.repo.ChangePassword(ctx, claims.UserID, hashPassword(req.NewPassword))
	if err != nil {
		return nil, errors.New("修改密码失败")
	}
	user.MustChangePassword = false
	_, user.RoleCode, _, user.Permissions, err = s.repo.UserAccess(ctx, user.ID)
	if err != nil {
		return nil, errors.New("读取用户权限失败")
	}
	token, err := s.signToken(model.Claims{UserID: user.ID, Username: user.Username, AuthVersion: authVersion, ExpiresAt: time.Now().Add(s.sessionDuration(ctx)).Unix()})
	if err != nil {
		return nil, errors.New("更新登录凭证失败")
	}
	_ = s.repo.LogOperation(ctx, user.Username, "修改登录密码", user.Username)
	return map[string]any{"token": token, "user": user}, nil
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
	if err == nil {
		_, u.RoleCode, _, u.Permissions, err = s.repo.UserAccess(ctx, u.ID)
	}
	return u, err
}

func (s *SystemService) ResolveAccess(ctx context.Context, claims model.Claims) (model.Claims, error) {
	status, roleCode, authVersion, permissions, err := s.repo.UserAccess(ctx, claims.UserID)
	if err != nil || status != "active" {
		return model.Claims{}, errors.New("用户已停用或不存在")
	}
	if claims.AuthVersion != authVersion {
		return model.Claims{}, errors.New("用户登录状态已更新")
	}
	claims.RoleCode = roleCode
	claims.Permissions = permissions
	return claims, nil
}

func (s *SystemService) CreateManagedUser(ctx context.Context, actor string, req model.UserCreateRequest) (model.UserCreateResult, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Email = strings.TrimSpace(req.Email)
	if req.Username == "" || req.DisplayName == "" || req.RoleID <= 0 {
		return model.UserCreateResult{}, errors.New("账号、昵称和角色不能为空")
	}
	password, err := temporaryPassword()
	if err != nil {
		return model.UserCreateResult{}, errors.New("生成临时密码失败")
	}
	id, err := s.repo.CreateManagedUser(ctx, req, hashPassword(password), generateAPIKey(req.Username))
	if err != nil {
		return model.UserCreateResult{}, errors.New("新增用户失败，账号可能已存在或角色不可用")
	}
	_ = s.repo.LogOperation(ctx, actor, "新增用户", req.Username)
	return model.UserCreateResult{ID: id, TemporaryPassword: password}, nil
}

func (s *SystemService) ResetUserPassword(ctx context.Context, actor string, id int64) (string, error) {
	password, err := temporaryPassword()
	if err != nil {
		return "", errors.New("生成临时密码失败")
	}
	username, rows, err := s.repo.ResetUserPassword(ctx, id, hashPassword(password))
	if err != nil || rows == 0 {
		return "", errors.New("用户不存在")
	}
	_ = s.repo.LogOperation(ctx, actor, "重置用户密码", username)
	return password, nil
}

func (s *SystemService) Overview(ctx context.Context) (model.SystemOverview, error) {
	offlineSeconds := 45
	if item, err := s.repo.GetSystemSettingGroup(ctx, "execution"); err == nil {
		var value map[string]any
		if json.Unmarshal(item.Value, &value) == nil && settingInt(value, "executorOfflineSeconds") > 0 {
			offlineSeconds = settingInt(value, "executorOfflineSeconds")
		}
	}
	return s.repo.SystemOverview(ctx, offlineSeconds)
}

func (s *SystemService) ListOperationLogs(ctx context.Context, filter model.OperationLogFilter) (model.PageResult, error) {
	filter.Page = positiveInt(strconv.Itoa(filter.Page), 1)
	filter.PageSize = positiveInt(strconv.Itoa(filter.PageSize), 20)
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	items, total, err := s.repo.ListOperationLogs(ctx, filter)
	return model.PageResult{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, err
}

func (s *SystemService) ExportOperationLogs(ctx context.Context, filter model.OperationLogFilter) ([]byte, error) {
	filter.Page, filter.PageSize = 1, 5000
	items, _, err := s.repo.ListOperationLogs(ctx, filter)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"ID", "操作人", "动作", "目标", "IP", "时间"})
	for _, item := range items {
		_ = writer.Write([]string{strconv.FormatInt(item.ID, 10), item.Actor, item.Action, item.Target, item.IP, item.CreatedAt.Format("2006-01-02 15:04:05")})
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func (s *SystemService) ListUsers(ctx context.Context, id, nickname, account string, page, pageSize int) (model.PageResult, error) {
	page = positiveInt(strconv.Itoa(page), 1)
	pageSize = positiveInt(strconv.Itoa(pageSize), 20)
	items, total, err := s.repo.ListUsers(ctx, strings.TrimSpace(id), strings.TrimSpace(nickname), strings.TrimSpace(account), page, pageSize)
	return model.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (s *SystemService) UnlockUser(ctx context.Context, actor string, id int64) error {
	username, err := s.repo.UnlockUser(ctx, id)
	if err != nil {
		return errors.New("解锁用户失败")
	}
	_ = s.repo.LogOperation(ctx, actor, "解锁用户", username)
	return nil
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
	roleChanged := (oldRole.Valid && roleID != oldRole.Int64) || (!oldRole.Valid && roleID != nil)
	if s.repo.IsOnlyActiveAdmin(ctx, id) && (status != "active" || roleChanged) {
		return errors.New("系统必须保留至少一个启用的超级管理员")
	}
	if username == actor && status != "active" {
		return errors.New("不能停用当前登录用户")
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
	req.Status = normalizeRoleStatus(req.Status)
	var err error
	req.Permissions, err = s.normalizePermissions(ctx, req.Permissions)
	if err != nil {
		return err
	}
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
	req.Status = normalizeRoleStatus(req.Status)
	var permissionErr error
	req.Permissions, permissionErr = s.normalizePermissions(ctx, req.Permissions)
	if permissionErr != nil {
		return permissionErr
	}
	if req.Name == "" || req.Description == "" {
		return errors.New("角色名称和角色描述不能为空")
	}
	code, _, err := s.repo.FindRoleCodeName(ctx, id)
	if err != nil {
		return errors.New("角色不存在")
	}
	if code == "admin" {
		return errors.New("超级管理员角色不能修改")
	}
	if err := s.repo.UpdateRole(ctx, id, req); err != nil {
		return errors.New("更新角色失败，角色名称可能已存在")
	}
	_ = s.repo.LogOperation(ctx, actor, "编辑角色", req.Name)
	return nil
}

func (s *SystemService) ListPermissions(ctx context.Context) ([]model.Permission, error) {
	return s.repo.ListPermissions(ctx)
}

func (s *SystemService) ListSystemSettings(ctx context.Context) ([]model.SystemSettingGroup, error) {
	return s.repo.ListSystemSettingGroups(ctx)
}

func (s *SystemService) UpdateSystemSettings(ctx context.Context, actor, groupKey string, req model.SystemSettingUpdateRequest) (int64, error) {
	value, err := validateSystemSetting(groupKey, req.Value)
	if err != nil {
		return 0, err
	}
	if req.Revision <= 0 {
		return 0, errors.New("配置版本无效")
	}
	summary := strings.TrimSpace(req.ChangeSummary)
	if summary == "" {
		summary = "更新" + systemSettingLabel(groupKey)
	}
	revision, err := s.repo.UpdateSystemSettingGroup(ctx, groupKey, value, req.Revision, actor, summary)
	if err != nil {
		return 0, errors.New("保存失败，配置可能已被其他管理员修改，请刷新后重试")
	}
	_ = s.repo.LogOperation(ctx, actor, "更新系统参数", groupKey)
	return revision, nil
}

func (s *SystemService) ListSystemSettingHistory(ctx context.Context, groupKey string) ([]model.SystemSettingHistory, error) {
	if !validSystemSettingGroup(groupKey) {
		return nil, errors.New("配置分组不存在")
	}
	return s.repo.ListSystemSettingHistory(ctx, groupKey)
}

func (s *SystemService) RollbackSystemSettings(ctx context.Context, actor, groupKey string, req model.SystemSettingRollbackRequest) (int64, error) {
	if !validSystemSettingGroup(groupKey) || req.TargetRevision <= 0 || req.Revision <= 0 {
		return 0, errors.New("回滚参数无效")
	}
	value, err := s.repo.SystemSettingHistory(ctx, groupKey, req.TargetRevision)
	if err != nil {
		return 0, errors.New("目标历史版本不存在")
	}
	value, err = validateSystemSetting(groupKey, value)
	if err != nil {
		return 0, err
	}
	revision, err := s.repo.UpdateSystemSettingGroup(ctx, groupKey, value, req.Revision, actor, fmt.Sprintf("回滚到 V%d", req.TargetRevision))
	if err != nil {
		return 0, errors.New("回滚失败，配置可能已被其他管理员修改")
	}
	_ = s.repo.LogOperation(ctx, actor, "回滚系统参数", fmt.Sprintf("%s:V%d", groupKey, req.TargetRevision))
	return revision, nil
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

func temporaryPassword() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for index := range random {
		random[index] = alphabet[int(random[index])%len(alphabet)]
	}
	return string(random[:6]) + "7a" + string(random[6:]), nil
}

func (s *SystemService) validatePassword(ctx context.Context, password string) error {
	minLength := 8
	if item, err := s.repo.GetSystemSettingGroup(ctx, "security"); err == nil {
		var value map[string]any
		if json.Unmarshal(item.Value, &value) == nil && settingInt(value, "passwordMinLength") >= 8 {
			minLength = settingInt(value, "passwordMinLength")
		}
	}
	if len(password) < minLength {
		return fmt.Errorf("密码长度至少 %d 位", minLength)
	}
	hasLetter, hasNumber := false, false
	for _, value := range password {
		if value >= '0' && value <= '9' {
			hasNumber = true
		}
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' {
			hasLetter = true
		}
	}
	if !hasLetter || !hasNumber {
		return errors.New("密码必须同时包含字母和数字")
	}
	return nil
}

func normalizeRoleStatus(status string) string {
	if status == "disabled" {
		return status
	}
	return "active"
}

func (s *SystemService) sessionDuration(ctx context.Context) time.Duration {
	item, err := s.repo.GetSystemSettingGroup(ctx, "security")
	if err != nil {
		return 24 * time.Hour
	}
	var value map[string]any
	if json.Unmarshal(item.Value, &value) != nil {
		return 24 * time.Hour
	}
	hours := settingInt(value, "sessionHours")
	if hours < 1 || hours > 720 {
		hours = 24
	}
	return time.Duration(hours) * time.Hour
}

func (s *SystemService) loginSecurityPolicy(ctx context.Context) (int, int) {
	failureLimit, lockMinutes := 5, 15
	if item, err := s.repo.GetSystemSettingGroup(ctx, "security"); err == nil {
		var value map[string]any
		if json.Unmarshal(item.Value, &value) == nil {
			if configured := settingInt(value, "loginFailureLimit"); configured > 0 {
				failureLimit = configured
			}
			if configured := settingInt(value, "lockMinutes"); configured > 0 {
				lockMinutes = configured
			}
		}
	}
	return failureLimit, lockMinutes
}

func validateSystemSetting(groupKey string, raw json.RawMessage) (json.RawMessage, error) {
	if !validSystemSettingGroup(groupKey) {
		return nil, errors.New("配置分组不存在")
	}
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil, errors.New("配置内容必须是 JSON 对象")
	}
	switch groupKey {
	case "execution":
		defaultConcurrency := settingInt(value, "defaultConcurrency")
		maxConcurrency := settingInt(value, "maxConcurrency")
		batchSize := settingInt(value, "batchSize")
		offlineSeconds := settingInt(value, "executorOfflineSeconds")
		if defaultConcurrency < 1 || maxConcurrency < defaultConcurrency || maxConcurrency > 500 {
			return nil, errors.New("默认并发必须为 1–最大并发，最大并发不能超过 500")
		}
		if batchSize < 1 || batchSize > 5000 {
			return nil, errors.New("单批用例数必须为 1–5000")
		}
		if offlineSeconds < 15 || offlineSeconds > 600 {
			return nil, errors.New("执行器离线阈值必须为 15–600 秒")
		}
		value = map[string]any{"defaultConcurrency": defaultConcurrency, "maxConcurrency": maxConcurrency, "batchSize": batchSize, "executorOfflineSeconds": offlineSeconds}
	case "security":
		sessionHours := settingInt(value, "sessionHours")
		passwordMinLength := settingInt(value, "passwordMinLength")
		failureLimit := settingInt(value, "loginFailureLimit")
		lockMinutes := settingInt(value, "lockMinutes")
		if sessionHours < 1 || sessionHours > 720 {
			return nil, errors.New("会话有效期必须为 1–720 小时")
		}
		if passwordMinLength < 8 || passwordMinLength > 32 {
			return nil, errors.New("密码最小长度必须为 8–32 位")
		}
		if failureLimit < 3 || failureLimit > 20 || lockMinutes < 1 || lockMinutes > 1440 {
			return nil, errors.New("登录锁定策略超出允许范围")
		}
		value = map[string]any{"sessionHours": sessionHours, "passwordMinLength": passwordMinLength, "loginFailureLimit": failureLimit, "lockMinutes": lockMinutes}
	case "notification":
		value = map[string]any{
			"executionSuccess": settingBool(value, "executionSuccess"),
			"executionFailure": settingBool(value, "executionFailure"),
			"executorOffline":  settingBool(value, "executorOffline"),
			"systemAlert":      true,
			"securityAlert":    true,
		}
	}
	return json.Marshal(value)
}

func validSystemSettingGroup(groupKey string) bool {
	return groupKey == "execution" || groupKey == "security" || groupKey == "notification"
}

func systemSettingLabel(groupKey string) string {
	return map[string]string{"execution": "执行策略", "security": "安全策略", "notification": "通知策略"}[groupKey]
}

func settingInt(value map[string]any, key string) int {
	switch item := value[key].(type) {
	case float64:
		return int(item)
	case int:
		return item
	}
	return 0
}

func settingBool(value map[string]any, key string) bool {
	item, _ := value[key].(bool)
	return item
}

func (s *SystemService) normalizePermissions(ctx context.Context, codes []string) ([]string, error) {
	items, err := s.repo.ListPermissions(ctx)
	if err != nil {
		return nil, errors.New("读取权限目录失败")
	}
	known := map[string]bool{}
	for _, item := range items {
		known[item.Code] = true
	}
	result := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if !known[code] {
			return nil, fmt.Errorf("权限编码不存在：%s", code)
		}
		result[code] = true
		if strings.HasPrefix(code, "system.") {
			result["menu.system.read"] = true
		}
		if strings.HasPrefix(code, "api.") {
			result["menu.api_automation.read"] = true
		}
	}
	normalized := make([]string, 0, len(result))
	for code := range result {
		normalized = append(normalized, code)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func positiveInt(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
