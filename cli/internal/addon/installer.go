package addon

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	PluginResourcePath = "res://addons/gdbg/plugin.cfg"
	defaultRepository  = "Dluck-Games/godot-debug-bridge"
	maxArchiveBytes    = 100 << 20
)

var requiredAutoloads = []struct {
	Name string
	Path string
}{
	{"ScreenshotManager", "res://addons/gdbg/screenshot_manager.gd"},
	{"AIDebugBridge", "res://addons/gdbg/ai_debug_bridge.gd"},
	{"DebugBridgeTestRuntime", "res://addons/gdbg/testing/test_runtime.gd"},
}

type Options struct {
	ProjectDir string
	SourceDir  string
	Version    string
	Force      bool
	Client     *http.Client
}

type Result struct {
	AddonDir       string
	Version        string
	DownloadURL    string
	ProjectChanged bool
}

func Install(ctx context.Context, opts Options) (Result, error) {
	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return Result{}, err
	}
	projectFile := filepath.Join(projectDir, "project.godot")
	if info, statErr := os.Stat(projectFile); statErr != nil || !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("no project.godot at %s", projectDir)
	}

	staging, err := os.MkdirTemp(projectDir, ".gdbg-install-")
	if err != nil {
		return Result{}, fmt.Errorf("create install staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	result := Result{}
	var sourceRoot string
	if strings.TrimSpace(opts.SourceDir) != "" {
		sourceRoot, err = filepath.Abs(opts.SourceDir)
		if err != nil {
			return Result{}, err
		}
		staged := filepath.Join(staging, "addons", "gdbg")
		if err := copyTree(sourceRoot, staged); err != nil {
			return Result{}, fmt.Errorf("stage addon source: %w", err)
		}
		sourceRoot = staged
		result.Version = "source"
	} else {
		client := opts.Client
		if client == nil {
			client = &http.Client{Timeout: 60 * time.Second}
		}
		version, err := resolveVersion(ctx, client, opts.Version)
		if err != nil {
			return Result{}, err
		}
		result.Version = version
		result.DownloadURL = releaseAssetURL(version)
		archivePath := filepath.Join(staging, "addon.zip")
		if err := download(ctx, client, result.DownloadURL, archivePath); err != nil {
			return Result{}, err
		}
		extracted := filepath.Join(staging, "extracted")
		if err := extractZip(archivePath, extracted); err != nil {
			return Result{}, fmt.Errorf("extract addon archive: %w", err)
		}
		sourceRoot, err = locateAddonRoot(extracted)
		if err != nil {
			return Result{}, err
		}
	}

	if err := validateAddon(sourceRoot); err != nil {
		return Result{}, err
	}
	destination := filepath.Join(projectDir, "addons", "gdbg")
	if _, err := os.Stat(destination); err == nil && !opts.Force {
		return Result{}, fmt.Errorf("%s already exists; use --force to replace it", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err := replaceDirectory(sourceRoot, destination, opts.Force); err != nil {
		return Result{}, err
	}

	pluginChanged, err := EnablePlugin(projectFile)
	if err != nil {
		return Result{}, err
	}
	autoloadsChanged, err := EnsureAutoloads(projectFile)
	if err != nil {
		return Result{}, err
	}
	result.AddonDir = destination
	result.ProjectChanged = pluginChanged || autoloadsChanged
	return result, nil
}

func resolveVersion(ctx context.Context, client *http.Client, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "dev" && requested != "latest" {
		return strings.TrimPrefix(requested, "v"), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+defaultRepository+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve latest GDBG release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve latest GDBG release: GitHub returned %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode latest GDBG release: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if version == "" {
		return "", errors.New("latest GDBG release has no tag_name")
	}
	return version, nil
}

func releaseAssetURL(version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/v%s/gdbg-addon-v%s.zip",
		defaultRepository, version, version)
}

func download(ctx context.Context, client *http.Client, url, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: server returned %s", url, resp.Status)
	}
	if resp.ContentLength > maxArchiveBytes {
		return fmt.Errorf("download %s: archive exceeds %d bytes", url, maxArchiveBytes)
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return err
	}
	if written > maxArchiveBytes {
		return fmt.Errorf("download %s: archive exceeds %d bytes", url, maxArchiveBytes)
	}
	return nil
}

func extractZip(archivePath, destination string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	cleanDestination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, file := range r.File {
		name := filepath.FromSlash(file.Name)
		if filepath.IsAbs(name) || strings.HasPrefix(filepath.Clean(name), ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe archive path %q", file.Name)
		}
		target := filepath.Join(cleanDestination, name)
		if target != cleanDestination && !strings.HasPrefix(target, cleanDestination+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe archive path %q", file.Name)
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive symlink is not allowed: %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		mode := file.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(out, source)
		closeErr := out.Close()
		source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func locateAddonRoot(root string) (string, error) {
	for _, candidate := range []string{
		filepath.Join(root, "addons", "gdbg"),
		filepath.Join(root, "gdbg"),
	} {
		if err := validateAddon(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("release archive does not contain addons/gdbg/plugin.cfg")
}

func validateAddon(root string) error {
	info, err := os.Stat(filepath.Join(root, "plugin.cfg"))
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("GDBG addon source is missing plugin.cfg: %s", root)
	}
	return nil
}

func copyTree(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", source)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink is not allowed: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, entryInfo.Mode().Perm())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entryInfo.Mode().Perm())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		inCloseErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return inCloseErr
	})
}

func replaceDirectory(source, destination string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	backup := ""
	if _, err := os.Stat(destination); err == nil {
		if !force {
			return fmt.Errorf("%s already exists", destination)
		}
		backup = fmt.Sprintf("%s.backup-%d", destination, time.Now().UnixNano())
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("backup existing addon: %w", err)
		}
	}
	if err := os.Rename(source, destination); err != nil {
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("install addon: %w", err)
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("addon installed but old backup could not be removed: %w", err)
		}
	}
	return nil
}

// EnablePlugin adds the GDBG plugin to project.godot while preserving existing
// editor plugins and unrelated formatting. It supports Godot's single-line
// PackedStringArray representation and fails closed on unknown representations.
func EnablePlugin(projectFile string) (bool, error) {
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return false, err
	}
	content := string(data)
	if strings.Contains(content, `"`+PluginResourcePath+`"`) {
		return false, nil
	}
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	section := -1
	sectionEnd := len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == "[editor_plugins]" {
			section = i
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}
	pluginLiteral := `"` + PluginResourcePath + `"`
	if section < 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[editor_plugins]", "", "enabled=PackedStringArray("+pluginLiteral+")", "")
	} else {
		enabledLine := -1
		for i := section + 1; i < sectionEnd; i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "enabled=") {
				enabledLine = i
				break
			}
		}
		if enabledLine < 0 {
			lines = append(lines[:section+1], append([]string{"", "enabled=PackedStringArray(" + pluginLiteral + ")"}, lines[section+1:]...)...)
		} else {
			line := lines[enabledLine]
			prefix := "enabled=PackedStringArray("
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, ")") {
				return false, errors.New("unsupported editor_plugins enabled representation in project.godot")
			}
			inside := strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), ")")
			if strings.TrimSpace(inside) == "" {
				inside = pluginLiteral
			} else {
				inside += ", " + pluginLiteral
			}
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[enabledLine] = indent + prefix + inside + ")"
		}
	}
	updated := strings.Join(lines, lineEnding)
	if err := os.WriteFile(projectFile, []byte(updated), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureAutoloads installs the runtime nodes needed outside the editor. The
// operation is idempotent and refuses to replace a project's different
// autoload with the same name. A UID-based reference produced by Godot's
// script UID rewrite is accepted unchanged when it matches the .uid sidecar
// shipped with the addon.
func EnsureAutoloads(projectFile string) (bool, error) {
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return false, err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lineEnding := "\n"
	if strings.Contains(string(data), "\r\n") {
		lineEnding = "\r\n"
	}
	lines := strings.Split(content, "\n")
	section := -1
	sectionEnd := len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) != "[autoload]" {
			continue
		}
		section = i
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				sectionEnd = j
				break
			}
		}
		break
	}

	missing := make([]string, 0, len(requiredAutoloads))
	for _, required := range requiredAutoloads {
		want := required.Name + `="*` + required.Path + `"`
		found := false
		if section >= 0 {
			prefix := required.Name + "="
			for i := section + 1; i < sectionEnd; i++ {
				trimmed := strings.TrimSpace(lines[i])
				if !strings.HasPrefix(trimmed, prefix) {
					continue
				}
				found = true
				if trimmed != want && !matchesUIDReference(projectFile, required.Path, trimmed, required.Name) {
					return false, fmt.Errorf("autoload %s already points elsewhere: %s", required.Name, trimmed)
				}
				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	if section < 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[autoload]", "")
		lines = append(lines, missing...)
		lines = append(lines, "")
	} else {
		insertion := append([]string(nil), missing...)
		if sectionEnd > section+1 && strings.TrimSpace(lines[sectionEnd-1]) != "" {
			insertion = append(insertion, "")
		}
		lines = append(lines[:sectionEnd], append(insertion, lines[sectionEnd:]...)...)
	}
	if err := os.WriteFile(projectFile, []byte(strings.Join(lines, lineEnding)), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// matchesUIDReference reports whether an existing autoload line is Godot's
// UID-based form of the required path. The expected value is built from the
// UID recorded in the .uid sidecar next to the required script; the sidecar
// must be readable and hold a nonempty uid:// value.
func matchesUIDReference(projectFile, requiredPath, trimmed, name string) bool {
	sidecar := filepath.Join(filepath.Dir(projectFile), filepath.FromSlash(strings.TrimPrefix(requiredPath, "res://"))+".uid")
	uid, err := os.ReadFile(sidecar)
	if err != nil {
		return false
	}
	value := strings.TrimSpace(string(uid))
	if !strings.HasPrefix(value, "uid://") || strings.TrimPrefix(value, "uid://") == "" {
		return false
	}
	return trimmed == name+`="*`+value+`"`
}
