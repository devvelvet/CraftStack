package agent

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pb "craftstack/gen/proto/craftstack"
	"craftstack/internal/agent/process"
)

// instanceNameRe validates instance names for Docker compatibility.
var instanceNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// extractServerZip extracts a ZIP file into the target directory.
// It handles nested root directories (e.g., "server/" prefix) by detecting a single
// root folder and stripping it. Returns the detected JAR filename (if any).
func (a *Agent) extractServerZip(zipData []byte, targetDir string) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("ZIP file open failed: %w", err)
	}

	// 1. detect single root directory — strip common prefix folder if present
	stripPrefix := ""
	if len(r.File) > 0 {
		roots := make(map[string]bool)
		for _, f := range r.File {
			name := filepath.ToSlash(f.Name)
			parts := strings.SplitN(name, "/", 2)
			if len(parts) >= 2 {
				roots[parts[0]] = true
			} else if !f.FileInfo().IsDir() {
				// root file directly if present strip  no
				roots["__ROOT_FILE__"] = true
			}
		}
		if len(roots) == 1 && !roots["__ROOT_FILE__"] {
			for k := range roots {
				stripPrefix = k + "/"
			}
		}
	}

	// 2. file extract
	var detectedJar string
	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)

		// strip prefix
		if stripPrefix != "" {
			if !strings.HasPrefix(name, stripPrefix) {
				continue
			}
			name = strings.TrimPrefix(name, stripPrefix)
		}

		if name == "" {
			continue
		}

		destPath := filepath.Join(targetDir, filepath.FromSlash(name))

		// prevent path traversal attacks
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(targetDir)) {
			a.log.Warn("ZIP path traversal detect, skipped", "path", name)
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return "", fmt.Errorf("directory create failed %s: %w", name, err)
			}
			continue
		}

		// parent directory create
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return "", fmt.Errorf("directory create failed %s: %w", filepath.Dir(name), err)
		}

		// file extract
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("file open failed %s: %w", name, err)
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return "", fmt.Errorf("create file failed %s: %w", name, err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return "", fmt.Errorf("file write failed %s: %w", name, err)
		}

		// JAR file auto detect (root level .jar file)
		if strings.HasSuffix(strings.ToLower(name), ".jar") && !strings.Contains(name, "/") {
			detectedJar = name
			a.log.Info("JAR file detect", "jar", name)
		}
	}

	return detectedJar, nil
}

// computeDockerMemory calculates Docker --memory as ~1.5x of JVM memoryMax.
// Supports "M" (megabytes) and "G" (gigabytes) suffixes.
// Returns a string like "1536M" or "3G".
func computeDockerMemory(memoryMax string) string {
	memoryMax = strings.TrimSpace(memoryMax)
	if memoryMax == "" {
		return ""
	}

	upper := strings.ToUpper(memoryMax)
	var valuePart string
	var unit string
	if strings.HasSuffix(upper, "G") {
		valuePart = upper[:len(upper)-1]
		unit = "G"
	} else if strings.HasSuffix(upper, "M") {
		valuePart = upper[:len(upper)-1]
		unit = "M"
	} else {
		// numberonly if present MB as 
		valuePart = upper
		unit = "M"
	}

	var val int
	if _, err := fmt.Sscanf(valuePart, "%d", &val); err != nil || val <= 0 {
		return ""
	}

	// calculate 1.5x
	if unit == "G" {
		// G unit: convert to MB then 1.5x, then back to proper unit
		totalMB := val * 1024 * 3 / 2
		if totalMB%1024 == 0 {
			return fmt.Sprintf("%dG", totalMB/1024)
		}
		return fmt.Sprintf("%dM", totalMB)
	}
	// M unit: 1.5x
	totalMB := val * 3 / 2
	if totalMB >= 1024 && totalMB%1024 == 0 {
		return fmt.Sprintf("%dG", totalMB/1024)
	}
	return fmt.Sprintf("%dM", totalMB)
}

func processStateToProto(s process.State) pb.InstanceState {
	switch s {
	case process.StateStopped:
		return pb.InstanceState_INSTANCE_STATE_STOPPED
	case process.StateStarting:
		return pb.InstanceState_INSTANCE_STATE_STARTING
	case process.StateRunning:
		return pb.InstanceState_INSTANCE_STATE_RUNNING
	case process.StateStopping:
		return pb.InstanceState_INSTANCE_STATE_STOPPING
	case process.StateCrashed:
		return pb.InstanceState_INSTANCE_STATE_CRASHED
	default:
		return pb.InstanceState_INSTANCE_STATE_UNKNOWN
	}
}
