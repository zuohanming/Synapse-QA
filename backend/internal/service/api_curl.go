package service

import (
	"errors"
	"net/url"
	"sort"
	"strings"

	"synapseqa/backend/internal/model"
)

func ParseAPICurl(command string) (model.APICurlParseResult, error) {
	tokens, err := splitCurlCommand(strings.TrimSpace(command))
	if err != nil || len(tokens) < 2 || strings.ToLower(tokens[0]) != "curl" {
		return model.APICurlParseResult{}, errors.New("请输入有效的 cURL 命令")
	}
	result := model.APICurlParseResult{Method: "GET", Headers: map[string]string{}}
	for index := 1; index < len(tokens); index++ {
		token := tokens[index]
		switch token {
		case "-X", "--request":
			index++
			if index >= len(tokens) {
				return result, errors.New("cURL 请求方法缺少值")
			}
			result.Method = strings.ToUpper(tokens[index])
		case "-H", "--header":
			index++
			if index >= len(tokens) {
				return result, errors.New("cURL 请求头缺少值")
			}
			parts := strings.SplitN(tokens[index], ":", 2)
			if len(parts) == 2 && !restrictedAPIHeader(parts[0]) {
				name := strings.TrimSpace(parts[0])
				result.Headers[name] = strings.TrimSpace(parts[1])
				if maskPreviewHeaders(map[string]string{name: result.Headers[name]})[name] == "******" {
					result.MaskedHeaders = append(result.MaskedHeaders, name)
				}
			}
		case "-d", "--data", "--data-raw", "--data-binary":
			index++
			if index >= len(tokens) {
				return result, errors.New("cURL 请求体缺少值")
			}
			result.Body = tokens[index]
			if result.Method == "GET" {
				result.Method = "POST"
			}
		default:
			if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
				result.URL = token
			}
		}
	}
	if result.URL == "" {
		return result, errors.New("cURL 命令缺少 URL")
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Host == "" {
		return result, errors.New("cURL URL 无效")
	}
	result.Protocol = strings.ToUpper(parsed.Scheme)
	sort.Strings(result.MaskedHeaders)
	return result, nil
}

func BuildMaskedCurl(preview model.APIRequestPreview) string {
	parts := []string{"curl", "-X", shellQuote(preview.Method)}
	names := make([]string, 0, len(preview.Headers))
	for name := range preview.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parts = append(parts, "-H", shellQuote(name+": "+preview.Headers[name]))
	}
	if preview.Body != "" {
		parts = append(parts, "--data-raw", shellQuote(preview.Body))
	}
	parts = append(parts, shellQuote(preview.URL))
	return strings.Join(parts, " ")
}

func splitCurlCommand(command string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range command {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
		} else if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
			flush()
		} else {
			current.WriteRune(char)
		}
	}
	if quote != 0 || escaped {
		return nil, errors.New("cURL 命令引号未闭合")
	}
	flush()
	return tokens, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
