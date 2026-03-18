package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ClaudePlayer はClaude APIを使ったAIプレイヤー
type ClaudePlayer struct {
	apiKey string
	model  string
}

// NewClaudePlayer は環境変数からAPIキーを取得してClaudePlayerを作成する
func NewClaudePlayer() (*ClaudePlayer, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("環境変数 ANTHROPIC_API_KEY が設定されていません")
	}
	return &ClaudePlayer{
		apiKey: apiKey,
		model:  "claude-sonnet-4-20250514",
	}, nil
}

// ChooseAction はClaudeにアクションを選択させる
func (c *ClaudePlayer) ChooseAction(state map[string]interface{}, actions []map[string]interface{}, playerID int, description string) (map[string]interface{}, error) {
	stateJSON, _ := json.MarshalIndent(state, "", "  ")
	actionsJSON, _ := json.Marshal(actions)

	var descSection string
	if description != "" {
		descSection = fmt.Sprintf("\nゲームの説明：\n%s\n", description)
	}

	prompt := fmt.Sprintf(`あなたはボードゲームのプレイヤー%dです。
勝利を目指して最善の行動を選んでください。
%s
現在の状態：
%s

取れる行動：
%s

選んだ行動をJSONで返してください。JSONのみを返し、他のテキストは含めないでください。
例: {"type": "take_card"}`, playerID, descSection, stateJSON, actionsJSON)

	// 最大3回リトライ
	for attempt := 0; attempt < 3; attempt++ {
		result, err := c.callAPI(prompt)
		if err != nil {
			if attempt == 2 {
				return nil, fmt.Errorf("Claude APIの呼び出しに失敗（3回リトライ後）: %w", err)
			}
			continue
		}

		// JSONをパース
		action, err := parseActionJSON(result)
		if err != nil {
			if attempt == 2 {
				return nil, fmt.Errorf("Claudeの応答をパースできません: %s", result)
			}
			continue
		}

		// 有効なアクションか検証
		actionType, _ := action["type"].(string)
		valid := false
		for _, a := range actions {
			if a["type"] == actionType {
				valid = true
				break
			}
		}
		if !valid {
			if attempt == 2 {
				return nil, fmt.Errorf("Claudeが無効なアクションを選択しました: %s", actionType)
			}
			continue
		}

		return action, nil
	}

	return nil, fmt.Errorf("Claudeからの応答を取得できませんでした")
}

type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *ClaudePlayer) callAPI(prompt string) (string, error) {
	reqBody := apiRequest{
		Model:     c.model,
		MaxTokens: 256,
		Messages: []apiMessage{
			{Role: "user", Content: prompt},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("APIリクエストに失敗: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("レスポンスの読み込みに失敗: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("APIエラー (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("レスポンスのパースに失敗: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("APIエラー: %s", apiResp.Error.Message)
	}

	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("レスポンスにコンテンツがありません")
	}

	return apiResp.Content[0].Text, nil
}

// parseActionJSON はClaudeの応答からJSONを抽出してパースする
func parseActionJSON(text string) (map[string]interface{}, error) {
	text = strings.TrimSpace(text)

	// ```json ... ``` ブロックを抽出
	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + 7
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			text = strings.TrimSpace(text[start : start+end])
		}
	} else if idx := strings.Index(text, "```"); idx >= 0 {
		start := idx + 3
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			text = strings.TrimSpace(text[start : start+end])
		}
	}

	// { から } までを抽出
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		text = text[start : end+1]
	}

	var action map[string]interface{}
	if err := json.Unmarshal([]byte(text), &action); err != nil {
		return nil, err
	}
	return action, nil
}
