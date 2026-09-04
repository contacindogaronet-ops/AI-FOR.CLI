package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type FileTarget struct {
	FilePath string
	Content  string
}

// ExpandTermuxEnv menggantikan tilde (~) dengan HOME dan $PREFIX jika diperlukan
func ExpandTermuxEnv(path string) string {
	homeDir := os.Getenv("HOME")
	prefixDir := os.Getenv("PREFIX")

	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(homeDir, path[2:])
	} else if path == "~" {
		path = homeDir
	}

	if strings.Contains(path, "$PREFIX") && prefixDir != "" {
		path = strings.ReplaceAll(path, "$PREFIX", prefixDir)
	}

	return path
}

func RunSystemDiagnostics(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		log.Error().Str("cmd", name).Strs("args", args).Msg(errMsg)
		return errMsg
	}
	return strings.TrimSpace(out.String())
}

func ExtractCodeBlocks(aiResponse string) []FileTarget {
	var targets []FileTarget

	reHeaderPath := regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\\s+(?:path|filepath|file)=\"?([^\\s\"`]+)\"?\\s*\n(.*?)```")
	matches := reHeaderPath.FindAllStringSubmatch(aiResponse, -1)

	for _, m := range matches {
		if len(m) >= 3 {
			rawPath := strings.TrimSpace(m[1])
			expandedPath := ExpandTermuxEnv(rawPath)
			targets = append(targets, FileTarget{
				FilePath: expandedPath,
				Content:  strings.TrimPrefix(m[2], "\n"),
			})
		}
	}

	reGenericBlock := regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\\s*\n(.*?)```")
	genericMatches := reGenericBlock.FindAllStringSubmatch(aiResponse, -1)
	reCommentFile := regexp.MustCompile(`^(?://|#|/\*)\s*(?:File|Path):\s*([^\s\*]+)`)

	for _, m := range genericMatches {
		if len(m) >= 2 {
			blockContent := m[1]
			lines := strings.Split(blockContent, "\n")
			if len(lines) > 0 {
				firstLine := strings.TrimSpace(lines[0])
				if fileMatch := reCommentFile.FindStringSubmatch(firstLine); len(fileMatch) >= 2 {
					rawPath := strings.TrimSpace(fileMatch[1])
					filePath := ExpandTermuxEnv(rawPath)

					alreadyExtracted := false
					for _, t := range targets {
						if t.FilePath == filePath {
							alreadyExtracted = true
							break
						}
					}

					if !alreadyExtracted {
						targets = append(targets, FileTarget{
							FilePath: filePath,
							Content:  blockContent,
						})
					}
				}
			}
		}
	}

	return targets
}

// ParseIntentAndExecute mendeteksi awalan prompt untuk menentukan mode aksi daemon
func ParseIntentAndExecute(prompt string) (string, string) {
	trimmed := strings.TrimSpace(prompt)

	if strings.HasPrefix(strings.ToUpper(trimmed), "FIX IT") {
		cleanPrompt := strings.TrimSpace(trimmed[6:])
		return "FIX", cleanPrompt
	} else if strings.HasPrefix(strings.ToUpper(trimmed), "PEMBAHASAN") {
		cleanPrompt := strings.TrimSpace(trimmed[10:])
		return "DISCUSSION", cleanPrompt
	} else if strings.HasPrefix(strings.ToUpper(trimmed), "EXEC") {
		cleanPrompt := strings.TrimSpace(trimmed[4:])
		return "DIRECT_EXEC", cleanPrompt
	}

	return "DEFAULT", prompt
}

func AutoApplyFiles(aiResponse string) int {
	targets := ExtractCodeBlocks(aiResponse)
	if len(targets) == 0 {
		return 0
	}

	appliedCount := 0
	fmt.Println("\n[AUTO-EXECUTOR] Menganalisis direktori (~ / $PREFIX) & menulis file...")

	for _, t := range targets {
		dir := filepath.Dir(t.FilePath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Error().Err(err).Str("dir", dir).Msg("Gagal membuat folder direktori")
				continue
			}
		}

		err := os.WriteFile(t.FilePath, []byte(t.Content), 0644)
		if err != nil {
			log.Error().Err(err).Str("file", t.FilePath).Msg("Gagal menulis file")
			continue
		}

		fmt.Printf(" 🚀 [TERBUAT/TERPERBARUI] -> %s\n", t.FilePath)
		appliedCount++
	}

	if appliedCount > 0 {
		fmt.Println("\n[AUTO-GIT] Menjalankan sinkronisasi git otomatis & rilis tag...")

		if _, err := os.Stat(".git/rebase-merge"); err == nil {
			RunSystemDiagnostics("git", "rebase", "--abort")
		}
		if _, err := os.Stat(".git/MERGE_HEAD"); err == nil {
			RunSystemDiagnostics("git", "merge", "--abort")
		}

		currentBranch := RunSystemDiagnostics("git", "branch", "--show-current")
		if currentBranch == "" {
			currentBranch = "master"
		}

		RunSystemDiagnostics("git", "add", ".")
		RunSystemDiagnostics("git", "commit", "-m", "feat(core): autonomous AI patch & release trigger")
		
		tagName := fmt.Sprintf("v1.0.%d", time.Now().Unix()%1000)
		RunSystemDiagnostics("git", "tag", "-a", tagName, "-m", "Automated Release by Termux AI Daemon")

		pullOut := RunSystemDiagnostics("git", "pull", "--rebase", "origin", currentBranch)
		if strings.Contains(pullOut, "CONFLICT") {
			RunSystemDiagnostics("git", "add", ".")
			RunSystemDiagnostics("git", "rebase", "--continue")
		}

		pushOut := RunSystemDiagnostics("git", "push", "-u", "origin", currentBranch, "--follow-tags", "--force-with-lease")
		if strings.Contains(pushOut, "error") || strings.Contains(pushOut, "rejected") {
			RunSystemDiagnostics("git", "push", "-u", "origin", currentBranch, "--follow-tags")
		} else {
			fmt.Printf(" 🚀 [GIT PUSH & TAG %s BERHASIL DIPICU KE GITHUB]\n", tagName)
		}
	}

	return appliedCount
}
