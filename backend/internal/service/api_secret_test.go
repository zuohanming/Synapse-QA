package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIConfigurationSecretEncryptionAndMasking(t *testing.T) {
	service := NewAPIAutomationService(nil, nil, []byte("test-secret"))
	protected, err := service.protectAPIConfiguration(json.RawMessage(`{"auth":{"type":"bearer","token":"plain-token"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(protected), "plain-token") || !strings.Contains(string(protected), encryptedAPISecretPrefix) {
		t.Fatalf("认证密钥未正确加密: %s", protected)
	}
	masked := maskAPIConfiguration(protected)
	if strings.Contains(string(masked), encryptedAPISecretPrefix) || !strings.Contains(string(masked), "******") {
		t.Fatalf("普通 DTO 未正确脱敏: %s", masked)
	}
	var config apiRequestConfiguration
	if err := json.Unmarshal(protected, &config); err != nil {
		t.Fatal(err)
	}
	if err := service.revealAPIAuth(config.Auth); err != nil || config.Auth["token"] != "plain-token" {
		t.Fatalf("执行边界解密失败: %#v, %v", config.Auth, err)
	}
}

func TestPreserveMaskedAPIAuth(t *testing.T) {
	incoming := json.RawMessage(`{"auth":{"type":"bearer","token":"******"}}`)
	current := json.RawMessage(`{"auth":{"type":"bearer","token":"enc:v1:stored"}}`)
	merged := preserveMaskedAPIAuth(incoming, current)
	if !strings.Contains(string(merged), "enc:v1:stored") {
		t.Fatalf("掩码保存时未保留原密文: %s", merged)
	}
}
