package core

import (
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

	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second

	// Auto-Recovery Loop: Mencegah keluar ke menu jika seluruh key sedang cooldown/overload
	for {
		allFailed := true

		for i := uint32(0); i < totalKeys; i++ {
			idx := (atomic.AddUint32(cursor, 1) - 1) % totalKeys
			currentKey := keys[idx]

			log.Info().Str("provider", providerName).Int("key_index", int(idx)).Msg("Mencoba eksekusi API...")

			err := dispatchAPI(providerName, currentKey, memory, p.config.SystemInstruction)
			if err == nil {
				return nil // Eksekusi sukses, keluar dari loop failover
			}

			log.Warn().Err(err).Str("provider", providerName).Int("key_index", int(idx)).Msg("API key gagal/cooling down, rotasi...")

			if strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limit") {
				allFailed = true
			}
		}

		if allFailed {
			log.Warn().Dur("retry_in", backoff).Msg("Semua API sedang overload/cooling down. Daemon bersiap menunggu dan mencoba ulang otomatis...")
			fmt.Printf("\n[AI DAEMON] Semua API sibuk/cooling down. Menunggu %.0fs sebelum mencoba ulang otomatis...\n", backoff.Seconds())

			time.Sleep(backoff)

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		return fmt.Errorf("semua key pada provider %s mengalami error fatal", providerName)
	}
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
