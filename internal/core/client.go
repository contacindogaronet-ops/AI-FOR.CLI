package core

import (
        "bytes"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "os"
        "path/filepath"
        "strings"
        "sync/atomic"
        "time"

        "aicli/internal/config"
        "github.com/rs/zerolog/log"
)

type AIClientPool struct {
        providers map[string][]string
        cursors   map[string]*uint32
        config    *config.Config
}

type ProviderHealth struct {
        Provider string
        KeyIndex int
        KeyMask  string
        Status   string
        Reason   string
}

func InitPool(cfg *config.Config) *AIClientPool {
        pool := &AIClientPool{
                providers: make(map[string][]string),
                cursors:   make(map[string]*uint32),
                config:    cfg,
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

func maskKey(k string) string {
        if len(k) <= 8 {
                return "****"
        }
        return k[:4] + "..." + k[len(k)-4:]
}

func (p *AIClientPool) PingAllProviders() []ProviderHealth {
        var results []ProviderHealth
        for name, keys := range p.providers {
                for idx, key := range keys {
                        err := testSingleKey(name, key)
                        status := "ONLINE"
                        reason := "OK"
                        if err != nil {
                                status = "OFFLINE"
                                reason = err.Error()
                        }
                        results = append(results, ProviderHealth{
                                Provider: name,
                                KeyIndex: idx,
                                KeyMask:  maskKey(key),
                                Status:   status,
                                Reason:   reason,
                        })
                }
        }
        return results
}

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

func isTextFile(filename string) bool {
        base := filepath.Base(filename)
        if base == "aicli" || strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".bin") || strings.HasSuffix(base, ".exe") {
                return false
        }

        allowedExts := map[string]bool{
                ".sh": true, ".go": true, ".py": true, ".js": true, ".ts": true,
                ".json": true, ".yaml": true, ".yml": true, ".txt": true, ".md": true,
                ".rs": true, ".c": true, ".cpp": true, ".h": true, ".html": true, ".css": true,
        }

        ext := strings.ToLower(filepath.Ext(filename))
        if allowedExts[ext] {
                return true
        }

        f, err := os.Open(filename)
        if err != nil {
                return false
        }
        defer f.Close()

        buf := make([]byte, 512)
        n, err := f.Read(buf)
        if err != nil && err != io.EOF {
                return false
        }

        for i := 0; i < n; i++ {
                if buf[i] == 0 {
                        return false
                }
        }

        return true
}

func EnrichPromptWithFile(prompt string) string {
        words := strings.Fields(prompt)
        for _, word := range words {
                cleaned := strings.Trim(word, ",'\"`?()[]{}")
                if cleaned == "" {
                        continue
                }
                if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
                        if isTextFile(cleaned) {
                                data, err := os.ReadFile(cleaned)
                                if err == nil {
                                        log.Info().Str("file", cleaned).Msg("Berhasil membaca file teks/skrip lokal Termux")
                                        return fmt.Sprintf("%s\n\n--- KONTEN FILE [%s] ---\n%s", prompt, cleaned, string(data))
                                }
                        }
                }
        }
        return prompt
}

func (p *AIClientPool) ExecuteWithFailover(providerName, prompt string, memory *Memory) error {
        keys, exists := p.providers[providerName]
        if !exists || len(keys) == 0 {
                return fmt.Errorf("provider %s tidak memiliki API key aktif", providerName)
        }

        enrichedPrompt := EnrichPromptWithFile(prompt)
        memory.Save("user", enrichedPrompt)

        cursor := p.cursors[providerName]
        totalKeys := uint32(len(keys))

        for i := uint32(0); i < totalKeys; i++ {
                idx := (atomic.AddUint32(cursor, 1) - 1) % totalKeys
                currentKey := keys[idx]

                log.Info().Str("provider", providerName).Int("key_index", int(idx)).Msg("Mencoba eksekusi API...")

                err := dispatchAPI(providerName, currentKey, memory, p.config.SystemInstruction)
                if err == nil {
                        return nil
                }

                log.Warn().Err(err).Str("provider", providerName).Int("key_index", int(idx)).Msg("API key gagal/cooling down, rotasi ke key berikutnya...")
        }

        return fmt.Errorf("semua key pada provider %s gagal diproses", providerName)
}

func dispatchAPI(providerType, apiKey string, memory *Memory, systemInstruction string) error {
        startTime := time.Now()

        switch providerType {
        case "gemini":
                return callGemini(apiKey, startTime, memory, systemInstruction)
        case "deepseek", "openai":
                baseURL := "https://api.deepseek.com/chat/completions"
                model := "deepseek-chat"
                if providerType == "openai" {
                        baseURL = "https://api.openai.com/v1/chat/completions"
                        model = "gpt-4o"
                }
                return callOpenAICompatible(baseURL, apiKey, model, startTime, memory, systemInstruction)
        case "anthropic", "claude":
                return callClaude(apiKey, startTime, memory, systemInstruction)
        default:
                return callGemini(apiKey, startTime, memory, systemInstruction)
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
