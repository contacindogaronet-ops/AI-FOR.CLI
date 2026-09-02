package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultModel string           `yaml:"default_model"`
	Providers    []ProviderConfig `yaml:"providers"`
}

type ProviderConfig struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	APIKeys []string `yaml:"api_keys"`
}

type AIClientPool struct {
	providers map[string][]string
	cursors   map[string]*uint32
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})

	promptFlag := flag.String("p", "", "Prompt untuk AI")
	fileFlag := flag.String("f", "", "File konteks opsional")
	modelFlag := flag.String("m", "", "Pilih provider spesifik")
	flag.Parse()

	config, err := loadConfig()
	if err != nil {
		log.Error().Err(err).Msg("Gagal memuat config.yaml")
		os.Exit(1)
	}

	pool := initPool(config)

	if *promptFlag == "" {
		runInteractiveMenu(config, pool)
		return
	}

	runPrompt(config, pool, *modelFlag, *promptFlag, *fileFlag)
}

func runInteractiveMenu(cfg *Config, pool *AIClientPool) {
	reader := bufio.NewReader(os.Stdin)
	currentModel := cfg.DefaultModel

	for {
		fmt.Println("\n=========================================")
		fmt.Println("       AI MULTI-PROVIDER CLI ENGINE      ")
		fmt.Println("=========================================")
		fmt.Printf("Model/Provider Aktif : [%s]\n", currentModel)
		fmt.Println("-----------------------------------------")
		fmt.Println("[1] Kirim Prompt / Chat")
		fmt.Println("[2] Kelola API Keys (Tambah / Hapus)")
		fmt.Println("[3] Ganti Provider Aktif")
		fmt.Println("[q] Keluar")
		fmt.Println("-----------------------------------------")
		fmt.Print("Pilih menu [1-3/q]: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Print("\nMasukkan prompt kamu: ")
			pInput, _ := reader.ReadString('\n')
			prompt := strings.TrimSpace(pInput)
			if prompt == "" {
				fmt.Println("Prompt tidak boleh kosong!")
				continue
			}

			fmt.Printf("\n[Mengirim dengan provider: %s]...\n", currentModel)
			err = pool.ExecuteWithFailover(currentModel, prompt)
			if err != nil {
				log.Error().Err(err).Msg("Gagal mendapatkan respons dari semua failover API keys")
			}

		case "2":
			manageAPIKeysMenu(cfg, pool, reader)

		case "3":
			fmt.Println("\nDaftar Provider Tersedia:")
			for idx, p := range cfg.Providers {
				fmt.Printf("[%d] %s (%d API Keys terdaftar)\n", idx+1, p.Name, len(p.APIKeys))
			}
			fmt.Print("Pilih nama provider atau nomor: ")
			mInput, _ := reader.ReadString('\n')
			target := strings.TrimSpace(mInput)
			
			found := false
			for _, p := range cfg.Providers {
				if p.Name == target {
					currentModel = p.Name
					found = true
					fmt.Printf("-> Berhasil beralih ke provider: %s\n", currentModel)
					break
				}
			}
			if !found {
				fmt.Println("-> Provider tidak valid!")
			}

		case "q", "exit":
			fmt.Println("Keluar dari program...")
			return

		default:
			fmt.Println("Pilihan tidak valid.")
		}
	}
}

func manageAPIKeysMenu(cfg *Config, pool *AIClientPool, reader *bufio.Reader) {
	for {
		fmt.Println("\n--- KELOLA API KEYS ---")
		for idx, p := range cfg.Providers {
			fmt.Printf("[%d] Provider: %s (Total Key: %d)\n", idx+1, p.Name, len(p.APIKeys))
			for kidx, k := range p.APIKeys {
				masked := k
				if len(k) > 10 {
					masked = k[:6] + "..." + k[len(k)-4:]
				}
				fmt.Printf("    - [%d.%d] %s\n", idx+1, kidx+1, masked)
			}
		}
		fmt.Println("-----------------------------------------")
		fmt.Println("[1] Tambah API Key")
		fmt.Println("[2] Hapus API Key")
		fmt.Println("[b] Kembali ke Menu Utama")
		fmt.Print("Pilih [1/2/b]: ")

		input, _ := reader.ReadString('\n')
		opt := strings.TrimSpace(input)

		if opt == "b" || opt == "B" {
			break
		}

		if opt == "1" {
			fmt.Print("Masukkan nama provider (gemini/deepseek/claude/gpt): ")
			pName, _ := reader.ReadString('\n')
			pName = strings.TrimSpace(pName)

			fmt.Print("Masukkan string API Key baru: ")
			newKey, _ := reader.ReadString('\n')
			newKey = strings.TrimSpace(newKey)

			if newKey == "" {
				fmt.Println("API Key kosong!")
				continue
			}

			updated := false
			for i := range cfg.Providers {
				if cfg.Providers[i].Name == pName {
					cfg.Providers[i].APIKeys = append(cfg.Providers[i].APIKeys, newKey)
					updated = true
					break
				}
			}

			if !updated {
				// Buat provider baru jika belum ada
				cfg.Providers = append(cfg.Providers, ProviderConfig{
					Name:    pName,
					Type:    pName,
					APIKeys: []string{newKey},
				})
			}

			_ = saveConfig(cfg)
			*pool = *initPool(cfg)
			fmt.Println("-> API Key berhasil ditambahkan dan disimpan!")

		} else if opt == "2" {
			fmt.Print("Masukkan nama provider yang ingin dihapus key-nya: ")
			pName, _ := reader.ReadString('\n')
			pName = strings.TrimSpace(pName)

			for i := range cfg.Providers {
				if cfg.Providers[i].Name == pName {
					if len(cfg.Providers[i].APIKeys) == 0 {
						fmt.Println("Tidak ada key di provider ini.")
						break
					}
					for kidx, k := range cfg.Providers[i].APIKeys {
						fmt.Printf("[%d] %s\n", kidx+1, k)
					}
					fmt.Print("Pilih nomor index key yang akan dihapus: ")
					var kidx int
					_, err := fmt.Scanf("%d", &kidx)
					// bersihkan buffer newline
					_, _ = reader.ReadString('\n')

					if err == nil && kidx >= 1 && kidx <= len(cfg.Providers[i].APIKeys) {
						idxToRemove := kidx - 1
						cfg.Providers[i].APIKeys = append(cfg.Providers[i].APIKeys[:idxToRemove], cfg.Providers[i].APIKeys[idxToRemove+1:]...)
						_ = saveConfig(cfg)
						*pool = *initPool(cfg)
						fmt.Println("-> API Key berhasil dihapus!")
					} else {
						fmt.Println("Index key tidak valid!")
					}
					break
				}
			}
		}
	}
}

func runPrompt(cfg *Config, pool *AIClientPool, modelFlag, promptFlag, fileFlag string) {
	finalPrompt := promptFlag
	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			log.Error().Err(err).Msg("Gagal membaca file konteks")
			os.Exit(1)
		}
		finalPrompt = fmt.Sprintf("%s\n\n--- KONTEN FILE (%s) ---\n%s", promptFlag, fileFlag, string(data))
	}

	targetProvider := cfg.DefaultModel
	if modelFlag != "" {
		targetProvider = modelFlag
	}

	err := pool.ExecuteWithFailover(targetProvider, finalPrompt)
	if err != nil {
		log.Error().Err(err).Msg("Semua API Key pada pool mengalami cooling down / gagal")
		os.Exit(1)
	}
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aicli", "config.yaml")
}

func loadConfig() (*Config, error) {
	configPath := getConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultCfg := Config{
			DefaultModel: "gemini",
			Providers: []ProviderConfig{
				{
					Name:    "gemini",
					Type:    "gemini",
					APIKeys: []string{},
				},
			},
		}
		_ = saveConfig(&defaultCfg)
		return &defaultCfg, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	configPath := getConfigPath()
	_ = os.MkdirAll(filepath.Dir(configPath), 0755)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

func initPool(cfg *Config) *AIClientPool {
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

func (p *AIClientPool) ExecuteWithFailover(providerName, prompt string) error {
	keys, exists := p.providers[providerName]
	if !exists || len(keys) == 0 {
		return fmt.Errorf("provider %s tidak memiliki API key aktif. Tambahkan key terlebih dahulu via menu!", providerName)
	}

	cursor := p.cursors[providerName]
	totalKeys := uint32(len(keys))

	for i := uint32(0); i < totalKeys; i++ {
		idx := (atomic.AddUint32(cursor, 1) - 1) % totalKeys
		currentKey := keys[idx]

		log.Info().Str("provider", providerName).Int("key_index", int(idx)).Msg("Mencoba eksekusi API...")

		err := dispatchAPI(providerName, currentKey, prompt)
		if err == nil {
			return nil
		}

		log.Warn().Err(err).Str("provider", providerName).Int("key_index", int(idx)).Msg("API key gagal/cooling down, memindahkan ke key berikutnya...")
	}

	return fmt.Errorf("semua key pada provider %s gagal diproses", providerName)
}

func dispatchAPI(providerType, apiKey, prompt string) error {
	switch providerType {
	case "gemini":
		return callGemini(apiKey, prompt)
	case "deepseek", "openai":
		baseURL := "https://api.deepseek.com/chat/completions"
		model := "deepseek-chat"
		if providerType == "openai" {
			baseURL = "https://api.openai.com/v1/chat/completions"
			model = "gpt-4o"
		}
		return callOpenAICompatible(baseURL, apiKey, model, prompt)
	case "anthropic", "claude":
		return callClaude(apiKey, prompt)
	default:
		return callGemini(apiKey, prompt)
	}
}

func callGemini(apiKey, prompt string) error {
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
		Error *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Candidates) > 0 {
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(result.Candidates[0].Content.Parts[0].Text)
		fmt.Println("-------------------")
		return nil
	}
	return fmt.Errorf("respon kosong")
}

func callOpenAICompatible(url, apiKey, model, prompt string) error {
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
		Error *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Choices) > 0 {
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(result.Choices[0].Message.Content)
		fmt.Println("-------------------")
		return nil
	}
	return fmt.Errorf("respon kosong")
}

func callClaude(apiKey, prompt string) error {
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
		Error   *struct{ Message string }
	}
	_ = json.Unmarshal(resBytes, &result)

	if result.Error != nil {
		return fmt.Errorf(result.Error.Message)
	}
	if len(result.Content) > 0 {
		fmt.Println("\n--- AI RESPONSE ---")
		fmt.Println(result.Content[0].Text)
		fmt.Println("-------------------")
		return nil
	}
	return fmt.Errorf("respon kosong")
}
