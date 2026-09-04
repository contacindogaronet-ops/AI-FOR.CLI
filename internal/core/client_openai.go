package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func callOpenAICompatible(url, apiKey, model string, startTime time.Time, memory *Memory, systemInstruction string) error {
	messages := []any{}
	if systemInstruction != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemInstruction})
	}

	for _, msg := range memory.History {
		role := msg.Role
		if role == "model" {
			role = "assistant"
		}
		messages = append(messages, map[string]string{"role": role, "content": msg.Content})
	}

	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	resBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct{ Content string }
		}
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Error *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Choices) > 0 {
		responseText := result.Choices[0].Message.Content
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(responseText)
		fmt.Println("-------------------")

		_ = AutoApplyFiles(responseText)

		memory.Save("model", responseText)
		printMetrics(model, startTime, result.Usage.PromptTokens, result.Usage.CompletionTokens, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}
