package conf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/InazumaV/V2bX/common/json5"
)

const (
	MachineCreated = "created"
	MachineUpdated = "updated"
	MachineAdded   = "added"
)

type machineFile struct {
	raw      map[string]json.RawMessage
	primary  *MachineConfig
	extras   []*MachineConfig
	hasValid bool
}

func loadMachineFile(path string) (*machineFile, error) {
	f := &machineFile{raw: map[string]json.RawMessage{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return f, nil
	}
	trimmed, err := io.ReadAll(json5.NewTrimNodeReader(bytes.NewReader(data)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(trimmed, &f.raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if v, ok := f.raw["Machine"]; ok {
		_ = json.Unmarshal(v, &f.primary)
	}
	if v, ok := f.raw["Machines"]; ok {
		_ = json.Unmarshal(v, &f.extras)
	}
	f.hasValid = f.primary.Enabled()
	for _, m := range f.extras {
		if m.Enabled() {
			f.hasValid = true
		}
	}
	return f, nil
}

func (f *machineFile) save(path string) error {
	if f.primary != nil {
		b, err := json.Marshal(f.primary)
		if err != nil {
			return err
		}
		f.raw["Machine"] = b
	} else {
		delete(f.raw, "Machine")
	}
	if len(f.extras) > 0 {
		b, err := json.Marshal(f.extras)
		if err != nil {
			return err
		}
		f.raw["Machines"] = b
	} else {
		delete(f.raw, "Machines")
	}
	out, err := json.MarshalIndent(f.raw, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// UpsertMachine 写入机器配置：同一面板地址则更新凭据，不同面板追加为多站；
// 原配置没有任何机器（全新或旧的 Nodes 模式）时重写为纯机器模式配置
func UpsertMachine(path string, nm MachineConfig) (string, error) {
	if !nm.Enabled() || nm.panelKey() == "" || nm.Token == "" {
		return "", fmt.Errorf("面板地址、机器 ID、Token 不能为空")
	}
	nm.APIHost = nm.panelKey()
	f, err := loadMachineFile(path)
	if err != nil {
		return "", err
	}

	if !f.hasValid {
		f.raw = map[string]json.RawMessage{"Log": json.RawMessage(`{"Level":"error","Output":""}`)}
		f.primary, f.extras = &nm, nil
		return MachineCreated, f.save(path)
	}

	update := func(m *MachineConfig) {
		m.APIHost = nm.APIHost
		m.MachineID = nm.MachineID
		m.Token = nm.Token
		if nm.Core != "" {
			m.Core = nm.Core
		}
		if nm.Timeout > 0 {
			m.Timeout = nm.Timeout
		}
	}
	if f.primary.Enabled() && f.primary.panelKey() == nm.APIHost {
		update(f.primary)
		return MachineUpdated, f.save(path)
	}
	for _, m := range f.extras {
		if m.Enabled() && m.panelKey() == nm.APIHost {
			update(m)
			return MachineUpdated, f.save(path)
		}
	}
	f.extras = append(f.extras, &nm)
	return MachineAdded, f.save(path)
}

// RemoveMachine 删除指定面板的机器配置，主站被删时由第一个附加站顶替
func RemoveMachine(path, apiHost string) (bool, error) {
	key := (&MachineConfig{APIHost: apiHost}).panelKey()
	f, err := loadMachineFile(path)
	if err != nil {
		return false, err
	}
	found := false
	kept := f.extras[:0]
	for _, m := range f.extras {
		if m != nil && m.panelKey() == key {
			found = true
			continue
		}
		kept = append(kept, m)
	}
	f.extras = kept
	if f.primary != nil && f.primary.panelKey() == key {
		found = true
		f.primary = nil
		if len(f.extras) > 0 {
			f.primary, f.extras = f.extras[0], f.extras[1:]
		}
	}
	if !found {
		return false, nil
	}
	return true, f.save(path)
}
