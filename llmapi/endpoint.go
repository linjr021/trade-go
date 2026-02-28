package llmapi

import (
	"fmt"
	"net/url"
	"strings"
)

const defaultChatPath = "/chat/completions"
const defaultModelsPath = "/models"

// ResolveChatEndpoint converts provider base URL into a Chat Completions endpoint.
// It keeps already-complete endpoint paths untouched, while allowing base URLs like
// https://api.openai.com/v1.
func ResolveChatEndpoint(base string) (string, error) {
	raw := strings.TrimSpace(base)
	if raw == "" {
		return "", fmt.Errorf("base_url 为空")
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("base_url 非法")
	}

	p := strings.TrimSpace(u.Path)
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		// already full endpoint
	case p == "" || p == "/":
		u.Path = defaultChatPath
	case p == "/v1" || p == "/v1/":
		u.Path = "/v1" + defaultChatPath
	default:
		u.Path = strings.TrimSuffix(p, "/") + defaultChatPath
	}
	return u.String(), nil
}

// ResolveModelsEndpoint converts provider base URL into a models-list endpoint.
// For example:
// - https://api.openai.com/v1 => https://api.openai.com/v1/models
// - https://api.openai.com/v1/chat/completions => https://api.openai.com/v1/models
func ResolveModelsEndpoint(base string) (string, error) {
	raw := strings.TrimSpace(base)
	if raw == "" {
		return "", fmt.Errorf("base_url 为空")
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("base_url 非法")
	}

	p := strings.TrimSpace(u.Path)
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		u.Path = strings.TrimSuffix(p, "/chat/completions") + defaultModelsPath
	case p == "" || p == "/":
		u.Path = defaultModelsPath
	case p == "/v1" || p == "/v1/":
		u.Path = "/v1" + defaultModelsPath
	default:
		u.Path = strings.TrimSuffix(p, "/") + defaultModelsPath
	}
	return u.String(), nil
}
