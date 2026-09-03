# AI Multi-Provider CLI Engine

CLI tool berperforma tinggi berbasis bahasa Go untuk berinteraksi dengan berbagai model AI (Gemini, DeepSeek, Claude, GPT) secara paralel. Dilengkapi dengan sistem failover otomatis (*round-robin key rotation*), manajemen konfigurasi YAML, dan memori percakapan kontekstual lokal yang berkelanjutan.

## Struktur Direktori Proyek

```text
aicli/
├── .github/workflows/build.yml  # CI/CD Automation Build
├── internal/
│   ├── config/config.go         # Manajemen & parser config.yaml
│   ├── core/client.go           # HTTP dispatcher, failover pool & metrik
│   └── core/memory.go           # Sistem penyimpanan riwayat chat lokal
├── internal/ui/menu.go          # Interactive CLI & manajemen API key
├── go.mod                       # Go modules dependencies
├── main.go                      # Entry point aplikasi
└── README.md                    # Dokumentasi proyek
