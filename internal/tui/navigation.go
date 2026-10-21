package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ssh-mcp/internal/control"
	"ssh-mcp/internal/store"
)

type pulseMsg struct{}

func pulse() tea.Cmd { return tea.Tick(time.Second, func(time.Time) tea.Msg { return pulseMsg{} }) }
func (m *Model) label(zh, en string) string {
	if m.lang == "en" {
		return en
	}
	return zh
}
func (m *Model) menuRow(label string, selected bool) string {
	if selected {
		// Contrast follows the terminal palette; the arrow still identifies
		// selection when ANSI styling is unavailable (Refactoring UI §5.7).
		return lipgloss.NewStyle().Bold(true).Reverse(true).Render("▶ " + label + " ")
	}
	return "  " + label
}
func (m *Model) dashboardLabels() []string {
	lock := m.label("解锁凭据库", "Unlock credentials")
	if m.status.Unlocked {
		lock = m.label("锁定凭据库", "Lock credentials")
	}
	if m.status.Unlocked && len(m.status.PendingKeys) > 0 {
		lock = m.label("解锁 SSH 私钥", "Unlock SSH private key")
	}
	return []string{lock, m.label("管理服务器与数据库", "Servers and databases"), m.label("创建加密备份", "Create encrypted backup"), m.label("从备份恢复", "Restore backup"), m.label("更改主密码", "Change master password"), m.label("轮换数据密钥", "Rotate data key"), m.label("切换语言 / Language", "Language / 切换语言"), m.label("退出控制台", "Exit console")}
}
func (m *Model) navigationKey(message tea.KeyMsg, key string) (bool, tea.Cmd) {
	if m.screen == screenDashboard {
		switch key {
		case "up", "shift+tab":
			m.menuIndex = (m.menuIndex + 7) % 8
			return true, nil
		case "down", "tab":
			m.menuIndex = (m.menuIndex + 1) % 8
			return true, nil
		case "enter":
			switch m.menuIndex {
			case 0:
				if m.status.Unlocked && len(m.status.PendingKeys) > 0 {
					m.beginKeyUnlock(m.status.PendingKeys[0])
					return true, nil
				}
				if m.status.Unlocked {
					return true, m.call("locked", "lock", nil, &struct{}{})
				}
				m.beginUnlock()
			case 1:
				m.screen = screenTargets
				return true, m.loadTargets()
			case 2:
				m.beginMaintenance(maintenanceBackup)
			case 3:
				m.beginMaintenance(maintenanceRestore)
			case 4:
				m.beginMaintenance(maintenanceChangeMasterPassword)
			case 5:
				m.beginMaintenance(maintenanceRotate)
			case 6:
				return true, m.toggleLanguage()
			case 7:
				return true, tea.Quit
			}
			return true, nil
		}
	}
	if m.screen == screenTargets {
		switch key {
		case "tab", "shift+tab":
			m.actionFocus = !m.actionFocus
			return true, nil
		case "left":
			m.actionFocus = true
			m.targetAction = (m.targetAction + 5) % 6
			return true, nil
		case "right":
			m.actionFocus = true
			m.targetAction = (m.targetAction + 1) % 6
			return true, nil
		case "up", "down":
			if m.actionFocus {
				delta := 1
				if key == "up" {
					delta = -1
				}
				m.targetAction = (m.targetAction + delta + 6) % 6
				return true, nil
			}
		case "esc":
			if m.actionFocus {
				m.actionFocus = false
				return true, nil
			}
		case "enter":
			if m.actionFocus {
				return true, m.performTargetAction()
			}
		}
	}
	if m.screen == screenFingerprintConfirm || m.screen == screenTargetDeleteConfirm {
		switch key {
		case "up", "down", "left", "right", "tab", "shift+tab":
			m.confirmAccept = !m.confirmAccept
			return true, nil
		case "enter":
			translated := "n"
			if m.confirmAccept {
				translated = "y"
			}
			if m.screen == screenFingerprintConfirm {
				return true, m.handleFingerprintConfirmation(translated)
			}
			return true, m.handleTargetDeleteConfirmation(translated)
		}
	}
	return false, nil
}
func (m *Model) targetToolbar() string {
	labels := []string{m.label("新增 SSH", "Add SSH"), m.label("新增数据库", "Add database"), m.label("编辑", "Edit"), m.label("启用 / 停用", "Enable / disable"), m.label("删除", "Delete"), m.label("刷新", "Refresh")}
	rows := []string{m.label("操作菜单", "Actions")}
	var actions []string
	for i, label := range labels {
		actions = append(actions, m.menuRow(label, m.actionFocus && i == m.targetAction))
	}
	limit := 0
	if m.height > 0 {
		limit = max(1, m.height-11-min(max(1, m.targetCount()), 4))
		if m.notice != "" {
			limit = max(1, limit-2)
		}
		if m.connectionNotice != "" {
			limit = max(1, limit-2)
		}
	}
	rows = append(rows, selectedRows(actions, m.targetAction, limit)...)
	help := m.label("↑↓ 选择目标 · Tab 操作菜单 · Enter 编辑 · Esc 返回", "↑↓ target · Tab actions · Enter edit · Esc back")
	if m.actionFocus {
		help = m.label("↑↓ 选择操作 · Enter 执行 · Tab / Esc 返回目标", "↑↓ action · Enter run · Tab / Esc targets")
	}
	if m.isCompactLayout() {
		help = m.label("↑↓ 选目标 · Tab 操作\nEnter 编辑 · Esc 返回", "↑↓ target · Tab actions\nEnter edit · Esc back")
		if m.actionFocus {
			help = m.label("↑↓ 选操作 · Enter 执行\nTab / Esc 返回目标", "↑↓ action · Enter run\nTab / Esc targets")
		}
	}
	return strings.Join(append(rows, "", help), "\n")
}
func (m *Model) performTargetAction() tea.Cmd {
	switch m.targetAction {
	case 0:
		m.beginSSHForm(store.SSHTarget{Mode: store.SSHDirect, Enabled: true, AllowFileOperations: true})
	case 1:
		m.beginDatabaseForm(store.DatabaseInstance{Engine: store.EngineMySQL, Port: 3306, TransportPolicy: store.DatabaseLegacyPlaintext, Enabled: true})
	case 2:
		m.editSelectedTarget()
	case 3:
		return m.toggleSelectedTarget()
	case 4:
		m.beginTargetDelete()
	case 5:
		return m.loadTargets()
	}
	return nil
}
func (m *Model) confirmationButtons(accept string) string {
	cancel := m.label("返回", "Back")
	return m.menuRow(cancel, !m.confirmAccept) + "\n" + m.menuRow(accept, m.confirmAccept) + "\n\n" + m.label("↑↓ / Tab 选择 · Enter 确认", "↑↓ / Tab select · Enter confirm")
}
func (m *Model) formButtons() string {
	save := m.label("验证并保存", "Verify and save")
	if m.screen == screenMaintenance {
		save = m.label("确认执行", "Confirm action")
	}
	cancel := m.label("取消", "Cancel")
	return m.menuRow(save, m.formButton == 1) + "\n" + m.menuRow(cancel, m.formButton == 2)
}

func (m *Model) choiceValue(index int) (string, bool) {
	if m.form == nil || m.form.kind != formSSH {
		return "", false
	}
	switch index {
	case 8:
		password, key := "● ", "○ "
		if m.form.fields[index].value == "private_key" {
			password, key = key, password
		}
		return password + m.label("密码登录", "Password") + "    " + key + m.label("私钥登录", "Private key"), true
	case 7:
		enabled, disabled := "○ ", "● "
		if m.form.fields[index].value == "true" {
			enabled, disabled = disabled, enabled
		}
		return enabled + m.label("允许", "Allow") + "    " + disabled + m.label("禁止", "Deny"), true
	}
	return "", false
}

func (m *Model) choiceLabel(index int) string {
	if index == 7 {
		return m.label("文件操作权限", "File operations")
	}
	return m.form.fields[index].label
}
func (m *Model) formNavigation(message tea.KeyMsg, key string) (bool, tea.Cmd) {
	if m.form == nil {
		return false, nil
	}
	if _, choice := m.choiceValue(m.form.index); choice && m.formButton == 0 {
		switch key {
		case "left", "right", " ", "tab", "shift+tab", "up", "down", "enter", "esc", "ctrl+s":
		default:
			return true, nil
		}
	}
	if key == "enter" && m.formButton == 0 || key == "down" || key == "up" || key == "tab" || key == "shift+tab" {
		m.saveCurrentField()
		order := m.visibleFields()
		pos := 0
		for i, index := range order {
			if index == m.form.index {
				pos = i
				break
			}
		}
		if m.formButton > 0 {
			pos = len(order) + m.formButton - 1
		}
		delta := 1
		if key == "shift+tab" || key == "up" {
			delta = -1
		}
		pos = (pos + delta + len(order) + 2) % (len(order) + 2)
		if pos >= len(order) {
			m.formButton = pos - len(order) + 1
			m.input.Blur()
		} else {
			m.formButton = 0
			m.form.index = order[pos]
			m.loadCurrentField()
		}
		return true, nil
	}
	if m.formButton > 0 {
		if key == "enter" {
			if m.formButton == 1 {
				return true, m.submitForm()
			}
			m.clearForm()
			m.screen = screenTargets
			return true, nil
		}
		if key == "esc" {
			return false, nil
		}
		return true, nil
	}
	if m.form.kind == formSSH && m.form.index == 8 && (key == "left" || key == "right" || key == " ") {
		field := &m.form.fields[8]
		if field.value == "private_key" {
			field.value = "password"
		} else {
			field.value = "private_key"
			m.form.fields[3].value = ""
		}
		m.loadCurrentField()
		return true, nil
	}
	if m.form.kind == formSSH && m.form.index == 7 && (key == "left" || key == "right" || key == " ") {
		field := &m.form.fields[7]
		if field.value == "true" {
			field.value = "false"
		} else {
			field.value = "true"
		}
		m.loadCurrentField()
		return true, nil
	}
	if key == "enter" && m.form.index == len(m.form.fields)-1 {
		m.saveCurrentField()
		m.formButton = 1
		m.input.Blur()
		return true, nil
	}
	return false, nil
}
func (m *Model) beginKeyUnlock(path string) {
	m.keyUnlockPath = path
	m.screen = screenKeyUnlock
	m.input = textinput.New()
	m.input.SetVirtualCursor(false)
	m.input.Prompt = m.label("密钥口令：", "Key passphrase: ")
	m.input.EchoMode = textinput.EchoPassword
	m.input.EchoCharacter = '*'
	m.input.SetWidth(textInputWidth(m.width, m.input.Prompt, ""))
	_ = m.input.Focus()
}
func (m *Model) loadStatusForKey() tea.Cmd {
	return m.callPendingTarget("key_status", "status", nil, &control.Status{})
}
func (m *Model) handleKeyUnlock(message tea.KeyMsg, key string) tea.Cmd {
	if key == "esc" {
		m.dismissedKey = m.keyUnlockPath
		m.input.SetValue("")
		m.clearPendingTarget()
		m.clearForm()
		m.screen = screenTargets
		return nil
	}
	if key == "enter" {
		params := struct {
			Path       string `json:"path"`
			Passphrase string `json:"passphrase"`
		}{m.keyUnlockPath, m.input.Value()}
		m.input.SetValue("")
		if m.pending != nil {
			return m.callPendingTarget("key_unlocked", "ssh.unlock_key", params, &struct{}{})
		}
		return m.call("key_unlocked", "ssh.unlock_key", params, &struct{}{})
	}
	return m.updateInput(message)
}

func (m *Model) visibleFields() []int {
	if m.form == nil {
		return nil
	}
	if m.form.kind == formSSH {
		credential := 3
		if m.form.fields[8].value == "private_key" {
			credential = 9
		}
		return []int{0, 1, 2, 8, credential, 4, 5, 6, 7}
	}
	order := make([]int, len(m.form.fields))
	for i := range order {
		order[i] = i
	}
	return order
}
func (m *Model) maintenanceNavigation(key string) (bool, tea.Cmd) {
	if m.maintenance == nil {
		return false, nil
	}
	if m.formButton > 0 && key == "enter" {
		if m.formButton == 1 {
			return true, m.submitMaintenance()
		}
		m.clearMaintenance()
		m.screen = screenDashboard
		return true, nil
	}
	if key == "tab" || key == "shift+tab" || key == "down" || key == "up" || key == "enter" {
		m.saveMaintenanceField()
		n := len(m.maintenance.fields)
		pos := m.maintenance.index
		if m.formButton > 0 {
			pos = n + m.formButton - 1
		}
		delta := 1
		if key == "up" || key == "shift+tab" {
			delta = -1
		}
		pos = (pos + delta + n + 2) % (n + 2)
		if pos >= n {
			m.formButton = pos - n + 1
			m.input.Blur()
		} else {
			m.formButton = 0
			m.maintenance.index = pos
			m.loadMaintenanceField()
		}
		return true, nil
	}
	if m.formButton > 0 && key != "esc" {
		return true, nil
	}
	return false, nil
}

func (m *Model) visibleFieldPosition() int {
	for pos, index := range m.visibleFields() {
		if index == m.form.index {
			return pos
		}
	}
	return 0
}

// Keep the selected row visible when a console has limited height.
func selectedRows(rows []string, selected, limit int) []string {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	start := max(0, min(selected-limit/2, len(rows)-limit))
	return rows[start : start+limit]
}
