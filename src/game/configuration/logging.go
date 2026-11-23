package configuration

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileLimitMB is decimal MB, independently of the binary MiB replay budget.
type Logging struct {
	TerminalTarget string `json:"terminal_target"`
	ShowLog        bool   `json:"show_log"`
	Level          string `json:"level"`
	WriteLog       bool   `json:"write_log"`
	FileLimitMB    int    `json:"file_limit_mb"`
	FileName       string `json:"file_name"`
}

func DefaultLogging() Logging {
	return Logging{TerminalTarget: "custom", Level: "WARN", FileLimitMB: 5, FileName: "error_log.txt"}
}
func (l Logging) Validate() error {
	if l.TerminalTarget != "system" && l.TerminalTarget != "custom" {
		return fmt.Errorf("logging.terminal_target: must be system or custom")
	}
	if l.Level != "DEBUG" && l.Level != "INFO" && l.Level != "WARN" && l.Level != "ERROR" {
		return fmt.Errorf("logging.level: must be DEBUG, INFO, WARN or ERROR")
	}
	if l.FileLimitMB < 1 || l.FileLimitMB > 100 {
		return fmt.Errorf("logging.file_limit_mb: must be an integer1..100 decimal MB (1000000 bytes each)")
	}
	n := l.FileName
	if len(n) < 1 || len(n) > 64 || n == "." || n == ".." || strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".") || strings.HasSuffix(n, " ") || strings.HasSuffix(strings.ToLower(n), ".previous") {
		return fmt.Errorf("logging.file_name: use an ordinary1..64-character filename, not a path, hidden name or .previous name")
	}
	for _, c := range n {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return fmt.Errorf("logging.file_name: only ASCII letters, digits, underscore, hyphen and dot are supported")
		}
	}
	base := strings.ToUpper(strings.SplitN(n, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
		return fmt.Errorf("logging.file_name: reserved Windows device name")
	}
	return nil
}
func LogPath(directory string, l Logging) string { return filepath.Join(directory, "logs", l.FileName) }
