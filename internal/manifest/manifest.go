// Package manifest 读 gpm 的 <简称>-manifest.yaml。
//
// 只读 GUI-Setup 用得着的那几个字段：id / name / version / requires /
// setup（本平台该带哪个图形安装器）。安装逻辑一个字节都不碰——那是 gpm 的事。
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Suffix 是清单文件名的后缀。
const Suffix = "-manifest.yaml"

// Entry 是"某个平台上这个包长什么样"，与 gpm 的同名类型一个意思：
// bundle 与 exe 二选一（macOS 是 .app，Windows / Linux 是单个可执行文件）。
type Entry struct {
	Bundle string `yaml:"bundle,omitempty"`
	Exe    string `yaml:"exe,omitempty"`
}

// Rel 返回相对路径（bundle 优先）。
func (e Entry) Rel() string {
	if e.Bundle != "" {
		return e.Bundle
	}
	return e.Exe
}

// Kind 返回 "bundle" / "exe" / ""。
func (e Entry) Kind() string {
	switch {
	case e.Bundle != "":
		return "bundle"
	case e.Exe != "":
		return "exe"
	default:
		return ""
	}
}

// Launch 是启动器那一段。
type Launch struct {
	Cmd  string `yaml:"cmd,omitempty"`
	Mode string `yaml:"mode,omitempty"`
}

// Manifest 是 <简称>-manifest.yaml 的子集。
type Manifest struct {
	ID       string           `yaml:"id"`
	Name     string           `yaml:"name"`
	Version  string           `yaml:"version"`
	Requires []string         `yaml:"requires,omitempty"`
	Entry    map[string]Entry `yaml:"entry,omitempty"`
	Setup    map[string]Entry `yaml:"setup,omitempty"`
	Launch   Launch           `yaml:"launch,omitempty"`

	// Path 是清单文件自己的路径，Short 是文件名里的简称（ad）。
	// 两个都不来自 YAML。
	Path  string `yaml:"-"`
	Short string `yaml:"-"`
}

// ShortFromName 从文件名里取简称：ad-manifest.yaml → ad。
func ShortFromName(name string) string {
	return strings.TrimSuffix(filepath.Base(name), Suffix)
}

// Parse 读一份清单。
func Parse(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s 不是一份能读的清单：%w", path, err)
	}
	m.Path = path
	m.Short = ShortFromName(path)
	if strings.TrimSpace(m.ID) == "" {
		return nil, fmt.Errorf("%s 里没有 id", path)
	}
	if m.Launch.Cmd == "" {
		m.Launch.Cmd = m.Short
	}
	return &m, nil
}

// Toolchains 返回 requires 里的工具链（cot / tdp），保持原顺序。
func (m *Manifest) Toolchains() []string {
	var out []string
	for _, r := range m.Requires {
		switch r {
		case "cot", "tdp":
			out = append(out, r)
		}
	}
	return out
}

// DefaultToolchain 是界面 B 的缺省选项：requires 里第一个认识的工具链。
func (m *Manifest) DefaultToolchain() string {
	if ts := m.Toolchains(); len(ts) > 0 {
		return ts[0]
	}
	return ""
}

// SetupEntry 返回这个平台上该带的图形安装器（可能是个空 Entry）。
func (m *Manifest) SetupEntry(goos string) Entry {
	if m.Setup == nil {
		return Entry{}
	}
	return m.Setup[goos]
}
