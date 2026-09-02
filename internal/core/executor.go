package core

import (
	"bytes"
	"os/exec"
	"strings"
)

// RunSystemDiagnostics mengeksekusi perintah terminal secara lokal untuk diagnosa sistem Termux.
func RunSystemDiagnostics(command string) string {
	cmd := exec.Command("bash", "-c", command)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(stderr.String())
	}
	return strings.TrimSpace(out.String())
}
