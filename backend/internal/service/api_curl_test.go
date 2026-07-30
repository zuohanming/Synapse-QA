package service

import (
	"strings"
	"testing"

	"synapseqa/backend/internal/model"
)

func TestParseAPICurl(t *testing.T) {
	result, err := ParseAPICurl(`curl -X POST 'https://example.com/users' -H 'Authorization: Bearer secret' -H 'Content-Type: application/json' --data-raw '{"name":"张三"}'`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "POST" || result.URL != "https://example.com/users" || result.Body == "" {
		t.Fatalf("cURL 解析结果不正确: %#v", result)
	}
	if len(result.MaskedHeaders) != 1 || result.MaskedHeaders[0] != "Authorization" {
		t.Fatalf("敏感请求头识别不正确: %#v", result.MaskedHeaders)
	}
}

func TestBuildMaskedCurl(t *testing.T) {
	command := BuildMaskedCurl(model.APIRequestPreview{
		Method: "GET",
		URL:    "https://example.com/users",
		Headers: map[string]string{
			"Authorization": "******",
		},
	})
	if !strings.Contains(command, "Authorization: ******") || strings.Contains(command, "secret") {
		t.Fatalf("脱敏 cURL 结果不正确: %s", command)
	}
}
