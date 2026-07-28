package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const encryptedAPISecretPrefix = "enc:v1:"

func (s *APIAutomationService) protectAPIConfiguration(raw json.RawMessage) (json.RawMessage, error) {
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	auth, _ := config["auth"].(map[string]any)
	for _, key := range []string{"token", "password", "value"} {
		value, _ := auth[key].(string)
		if value == "" || value == "******" || strings.HasPrefix(value, encryptedAPISecretPrefix) {
			continue
		}
		encrypted, err := encryptAPISecret(s.secretKey, value)
		if err != nil {
			return nil, err
		}
		auth[key] = encrypted
	}
	if auth != nil {
		config["auth"] = auth
	}
	return json.Marshal(config)
}

func (s *APIAutomationService) revealAPIAuth(auth map[string]any) error {
	for _, key := range []string{"token", "password", "value"} {
		value, _ := auth[key].(string)
		if !strings.HasPrefix(value, encryptedAPISecretPrefix) {
			continue
		}
		decrypted, err := decryptAPISecret(s.secretKey, value)
		if err != nil {
			return errors.New("认证密钥解密失败")
		}
		auth[key] = decrypted
	}
	return nil
}

func maskAPIConfiguration(raw json.RawMessage) json.RawMessage {
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil {
		return raw
	}
	auth, _ := config["auth"].(map[string]any)
	for _, key := range []string{"token", "password", "value"} {
		if value, exists := auth[key]; exists && value != "" {
			auth[key] = "******"
		}
	}
	masked, err := json.Marshal(config)
	if err != nil {
		return raw
	}
	return masked
}

func preserveMaskedAPIAuth(incoming, current json.RawMessage) json.RawMessage {
	var nextConfig, oldConfig map[string]any
	if json.Unmarshal(incoming, &nextConfig) != nil || json.Unmarshal(current, &oldConfig) != nil {
		return incoming
	}
	nextAuth, _ := nextConfig["auth"].(map[string]any)
	oldAuth, _ := oldConfig["auth"].(map[string]any)
	for _, key := range []string{"token", "password", "value"} {
		if nextAuth[key] == "******" {
			nextAuth[key] = oldAuth[key]
		}
	}
	nextConfig["auth"] = nextAuth
	merged, err := json.Marshal(nextConfig)
	if err != nil {
		return incoming
	}
	return merged
}

func encryptAPISecret(key []byte, value string) (string, error) {
	hash := sha256.Sum256(key)
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return encryptedAPISecretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptAPISecret(key []byte, value string) (string, error) {
	encoded := strings.TrimPrefix(value, encryptedAPISecretPrefix)
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(key)
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(payload) < gcm.NonceSize() {
		return "", errors.New("密文格式无效")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
	return string(plain), err
}
