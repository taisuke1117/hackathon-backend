package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var ErrNoAPIKey = errors.New("GEMINI_API_KEY is not set")

func model() string {
	if v := os.Getenv("GEMINI_MODEL"); v != "" {
		return v
	}
	return "gemini-flash-latest"
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type generateRequest struct {
	Contents []struct {
		Parts []part `json:"parts"`
	} `json:"contents"`
	GenerationConfig *struct {
		ResponseMimeType string `json:"response_mime_type,omitempty"`
	} `json:"generationConfig,omitempty"`
}

// generate Gemini APIを呼んで最初の候補テキストを返す
func generate(parts []part, jsonMode bool) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", ErrNoAPIKey
	}

	var req generateRequest
	req.Contents = make([]struct {
		Parts []part `json:"parts"`
	}, 1)
	req.Contents[0].Parts = parts
	if jsonMode {
		req.GenerationConfig = &struct {
			ResponseMimeType string `json:"response_mime_type,omitempty"`
		}{ResponseMimeType: "application/json"}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	// 新形式のAPIキー(AQ.プレフィックス)はX-goog-api-keyヘッダーで渡す
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		model())
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-goog-api-key", apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini api error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini returned no candidates")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// formatPrice カンマ区切りの価格文字列
func formatPrice(p int) string {
	s := fmt.Sprintf("%d", p)
	n := len(s)
	if n <= 3 {
		return s
	}
	result := ""
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
