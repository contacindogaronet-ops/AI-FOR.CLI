package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func callGemini(apiKey string, startTime time.Time, memory *Memory, systemInstruction string) error {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.7-flash:generateContent?key=%s", apiKey)

	contents := []any{}
	for _, msg := range memory.History {
		role := "user"
		if msg.Role == "model" || msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role": role,
			"parts": []any{map[string]any{"text": msg.Content}},
		})
	}

	payload := map[string]any{
		"contents": contents,
	}
	if systemInstruction != "" {
		payload["system_instruction"] = map[string]any{
			"parts": []any{map[string]any{"text": systemInstruction}},
		}
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	resBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string }
			}
		}
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
		Error *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Candidates) > 0 {
		responseText := result.Candidates[0].Content.Parts[0].Text
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(responseText)
		fmt.Println("-------------------")

		_ = AutoApplyFiles(responseText)

		memory.Save("model", responseText)
		printMetrics("gemini-3.7-flash", startTime, result.UsageMetadata.PromptTokenCount, result.UsageMetadata.CandidatesTokenCount, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}
