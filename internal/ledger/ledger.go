// Package ledger 只读地读 gpm 的账本。
//
// 账本是"这个家里装了什么"的唯一真相（gpm D28）。GUI-Setup 只读它，
// 一个字节都不写——要改什么都是让 gpm 去改。
//
// 名字规则（gpm v3.7 起）：`<家>/<家目录名>-state.json`；v3.6 及以前的
// `<家>/state.json` 还能读，所以两边都试。
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Link 是一条图形入口记录。
type Link struct {
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	Kind   string `json:"kind"`
}

// PathEdit 是我们动过的 shell 配置文件。
type PathEdit struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// Package 是一个已安装的包。
type Package struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Platform    string     `json:"platform"`
	InstalledAt time.Time  `json:"installedAt"`
	Dir         string     `json:"dir"`
	Entry       string     `json:"entry"`
	EntryKind   string     `json:"entryKind"`
	Cmd         string     `json:"cmd"`
	Launcher    string     `json:"launcher"`
	Links       []Link     `json:"links,omitempty"`
	PathEdits   []PathEdit `json:"pathEdits,omitempty"`
	Verified    bool       `json:"verified"`
}

// Ledger 是整个家的账本。
type Ledger struct {
	SchemaVersion int       `json:"schemaVersion"`
	Self          string    `json:"self,omitempty"`
	Packages      []Package `json:"packages"`
}

// FileName 是账本的文件名：家目录名 + -state.json。
func FileName(home string) string {
	return filepath.Base(filepath.Clean(home)) + "-state.json"
}

// Path 是账本的位置。
func Path(home string) string {
	return filepath.Join(home, FileName(home))
}

// LegacyPath 是 v3.6 及以前的位置。
func LegacyPath(home string) string {
	return filepath.Join(home, "state.json")
}

// Load 读账本。**文件不存在返回 (nil, nil)** —— "这个家还没装过东西"不是错误。
func Load(home string) (*Ledger, error) {
	for _, p := range []string{Path(home), LegacyPath(home)} {
		raw, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		var l Ledger
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, err
		}
		return &l, nil
	}
	return nil, nil
}

// Find 找某个 id 的包；没有就返回 nil。
func (l *Ledger) Find(id string) *Package {
	if l == nil {
		return nil
	}
	for i := range l.Packages {
		if l.Packages[i].ID == id {
			return &l.Packages[i]
		}
	}
	return nil
}

// PathEditFiles 是这次安装动过的 shell 配置文件（给卸载界面列的清单用）。
func (p *Package) PathEditFiles() []string {
	out := make([]string, 0, len(p.PathEdits))
	for _, e := range p.PathEdits {
		out = append(out, e.Path)
	}
	return out
}

// LinkPaths 是图形入口（软链 / 桌面项 / 快捷方式）。
func (p *Package) LinkPaths() []string {
	out := make([]string, 0, len(p.Links))
	for _, l := range p.Links {
		out = append(out, l.Path)
	}
	return out
}
