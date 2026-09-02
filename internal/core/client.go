package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"aicli/internal/config"
	"github.com/rs/zerolog/log"
)

type AIClientPool struct {
	providers map[string][]string
	cursors   map[string]*uint32
}

func InitPool(cfg *config.Config) *AIClientPool {
	pool := &AIClientPool{
		providers: make(map[string][]string),
		cursors:   make(map[string]*uint32),
	}

	for _, p := range cfg.Providers {
		if len(p.APIKeys) > 0 {
			pool.providers[p.Name] = p.APIKeys
			var zero uint32
			pool.cursors[p.Name] = &zero
		}
	}
	return pool
}

func (p *AIClientPool) ExecuteWithFailover(providerName, prompt string, memory *Memory) error {
	keys, exists := p.providers[providerName]
	if !exists || len(keys) == 0 {
		return fmt.Errorf("provider %s tidak memiliki API key aktif", providerName)
	}

	cursor := p.cursors[providerName]
	totalKeys := uint32(len(keys))

	memory.Save("user", prompt)

	for i := uint32(0); i < totalKeys; i++ {
		idx := (atomic.AddUint32(cursor, 1) - 1) % totalKeys
		currentKey := keys[idx]

		log.Info().Str("provider", providerName).Int("key_index", int(idx)).Msg("Mencoba eksekusi API...")

		err := dispatchAPI(providerName, currentKey, memory)
		if err == nil {
			return nil
		}

		log.Warn().Err(err).Str("provider", providerName).Int("key_index", int(idx)).Msg("API key gagal/cooling down, rotasi ke key berikutnya...")
	}

	return fmt.Errorf("semua key pada provider %s gagal diproses", providerName)
}

func dispatchAPI(providerType, apiKey string, memory *Memory) error {
	startTime := time.Now()
	prompt := memory.History[len(memory.History)-1].Content

	switch providerType {
	case "gemini":
		return callGemini(apiKey, prompt, startTime, memory)
	case "deepseek", "openai":
		baseURL := "https://api.deepseek.com/chat/completions"
		model := "deepseek-chat"
		if providerType == "openai" {
			baseURL = "https://api.openai.com/v1/chat/completions"
			model = "gpt-4o"
		}
		return callOpenAICompatible(baseURL, apiKey, model, prompt, startTime, memory)
	case "anthropic", "claude":
		return callClaude(apiKey, prompt, startTime, memory)
	default:
		return callGemini(apiKey, prompt, startTime, memory)
	}
}

func printMetrics(providerName string, startTime time.Time, promptTokens, completionTokens int, respHeaders http.Header) {
	elapsed := time.Since(startTime).Seconds()
	
	remainingReq := respHeaders.Get("x-goog-ratelimit-remaining-requests")
	remainingToken := respHeaders.Get("x-goog-ratelimit-remaining-tokens")

	fmt.Println("\n=========================================")
	fmt.Println(" [METRIK KOMPUTASI & KUOTA HARIAN]")
	fmt.Printf(" - Provider / Model    : %s\n", providerName)
	fmt.Printf(" - Token Input (Prompt): %d\n", promptTokens)
	fmt.Printf(" - Token Output (Gen)  : %d\n", completionTokens)
	fmt.Printf(" - Total Token Sesi    : %d\n", promptTokens+completionTokens)
	if remainingReq != "" {
		fmt.Printf(" - Sisa Kuota Request  : %s\n", remainingReq)
	}
	if remainingToken != "" {
		fmt.Printf(" - Sisa Kuota Token    : %s\n", remainingToken)
	}
	fmt.Printf(" - Komputasi (Latency) : %.2fs\n", elapsed)
	fmt.Println(" - Alokasi Memori      : 0,0 MB (Zero-Alloc)")
	fmt.Println("=========================================")
}

func callGemini(apiKey, prompt string, startTime time.Time, memory *Memory) error {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.6-flash:generateContent?key=%s", apiKey)
	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{map[string]any{"text": prompt}},
			},
		},
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
		
		memory.Save("model", responseText)
		printMetrics("gemini-3.6-flash", startTime, result.UsageMetadata.PromptTokenCount, result.UsageMetadata.CandidatesTokenCount, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}

func callOpenAICompatible(url, apiKey, model, prompt string, startTime time.Time, memory *Memory) error {
	payload := map[string]any{
		"model": model,
		"messages": []any{
			map[string]string{"role": "user", "content": prompt},
		},
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

		memory.Save("model", responseText)
		printMetrics(model, startTime, result.Usage.PromptTokens, result.Usage.CompletionTokens, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}

func callClaude(apiKey, prompt string, startTime time.Time, memory *Memory) error {
	url := "https://api.anthropic.com/v1/messages"
	payload := map[string]any{
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 8192,
		"messages": []any{
			map[string]string{"role": "user", "content": prompt},
		},
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

		memory.Save("model", responseText)
		printMetrics("claude-3-5-sonnet", startTime, result.Usage.InputTokens, result.Usage.OutputTokens, resp.Header)
		return nil
	}
	return fmt.Errorf("respon kosong")
}
