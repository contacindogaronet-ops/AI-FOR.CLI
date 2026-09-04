package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func callClaude(apiKey string, startTime time.Time, memory *Memory, systemInstruction string) error {
	url := "https://api.anthropic.com/v1/messages"

	messages := []any{}
	for _, msg := range memory.History {
		role := msg.Role
		if role == "model" {
			role = "assistant"
		}
		messages = append(messages, map[string]string{"role": role, "content": msg.Content})
	}

	payload := map[string]any{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 8192,
		"messages":   messages,
	}
	if systemInstruction != "" {
		payload["system"] = systemInstruction
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	resBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Content []struct{ Text string }
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Content) > 0 {
		responseText := result.Content[0].Text
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(responseText)
		fmt.Println("-------------------")

		_ = AutoApplyFiles(responseText)

		memory.Save("model", responseText)
		printMetrics("claude-3-5-sonnet", startTime, result.Usage.InputTokens, result.Usage.OutputTokens, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}

// Tambahan fungsi testSingleKey untuk keperluan PingAllProviders
func testSingleKey(providerType, apiKey string) error {
	switch providerType {
	case "gemini":
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.7-flash:generateContent?key=%s", apiKey)
		payload := map[string]any{
			"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "ping"}}}},
		}
		body, _ := json.Marshal(payload)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil
	case "deepseek", "openai":
		baseURL := "https://api.deepseek.com/chat/completions"
		model := "deepseek-chat"
		if providerType == "openai" {
			baseURL = "https://api.openai.com/v1/chat/completions"
			model = "gpt-4o"
		}
		payload := map[string]any{
			"model":      model,
			"messages":   []any{map[string]string{"role": "user", "content": "ping"}},
			"max_tokens": 5,
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil
	case "anthropic", "claude":
		url := "https://api.anthropic.com/v1/messages"
		payload := map[string]any{
			"model":      "claude-3-5-sonnet-20241022",
			"max_tokens": 5,
			"messages":   []any{map[string]string{"role": "user", "content": "ping"}},
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil
	default:
		return fmt.Errorf("unknown provider type")
	}
}
