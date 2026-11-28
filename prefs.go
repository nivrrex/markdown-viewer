package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// prefs 是跨次启动保留的用户设置。零值表示跟随系统。
type prefs struct {
	Font     string  `json:"font,omitempty"`
	CodeFont string  `json:"codeFont,omitempty"`
	FontSize float64 `json:"fontSize,omitempty"`
}

// loadPrefs 读取设置文件。文件不存在或内容损坏时返回零值，不让启动失败。
func loadPrefs(path string) prefs {
	if path == "" {
		return prefs{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return prefs{}
	}
	var p prefs
	if err := json.Unmarshal(b, &p); err != nil {
		return prefs{}
	}
	if p.FontSize < fontSizeMin || p.FontSize > fontSizeMax {
		p.FontSize = 0
	}
	return p
}

// savePrefs 原子写入：先写同目录临时文件再改名，避免留下半截文件。
func savePrefs(path string, p prefs) error {
	if path == "" {
		return errors.New("设置文件路径未知")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// migrateLegacySettings 把旧应用名目录里的设置复制到新目录，只在新目录还没有
// 设置文件时做一次，覆盖改名前的用户设置。
func migrateLegacySettings(newDir, newPath string) {
	if _, err := os.Stat(newPath); err == nil {
		return // 已有设置
	}
	old := filepath.Join(filepath.Dir(newDir), legacyAppName, "settings.json")
	b, err := os.ReadFile(old)
	if err != nil {
		return
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(newPath, b, 0o644)
}
