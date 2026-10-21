package tui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"ssh-mcp/internal/store"
)

func (m *Model) permissionField(index int) bool {
	return m.form != nil && (m.form.kind == formSSH && index == 10 || m.form.kind == formDatabase && index == 12)
}

func (m *Model) permissionLabel(tier store.PermissionTier) string {
	labels := []string{m.label("只读巡检", "Read only"), m.label("确认后操作", "Confirm operations"), m.label("完全授权", "Full authorization")}
	for i, t := range []store.PermissionTier{store.PermissionReadOnly, store.PermissionConfirm, store.PermissionFull} {
		mark := "○ "
		if t == store.EffectivePermissionTier(tier) {
			mark = "● "
		}
		labels[i] = mark + labels[i]
	}
	return strings.Join(labels, "\n    ")
}

func (m *Model) syncOperationApproval() {
	if m.operationApproval != nil {
		found := false
		for _, p := range m.status.PendingApprovals {
			if p.ID == m.operationApproval.ID {
				found = true
			}
		}
		if !found {
			m.operationApproval = nil
			m.screen = screenDashboard
		}
	}
	if m.status.Unlocked && !m.approvalBusy && m.operationApproval == nil && (m.screen == screenDashboard || m.screen == screenTargets) {
		for _, p := range m.status.PendingApprovals {
			if m.decidedApprovals[p.ID] {
				continue
			}
			m.operationApproval = &p
			m.approvalScroll = 0
			m.confirmAccept = false
			m.screen = screenOperationConfirm
			break
		}
	}
}

func (m *Model) handleOperationApproval(key string) tea.Cmd {
	if m.operationApproval == nil {
		m.screen = screenDashboard
		return nil
	}
	switch key {
	case "up", "k":
		m.approvalScroll = max(0, m.approvalScroll-1)
	case "down", "j":
		m.approvalScroll++
	case "left", "right", "tab", "shift+tab":
		m.confirmAccept = !m.confirmAccept
	case "esc", "n":
		return m.resolveOperation(false)
	case "enter":
		if m.confirmAccept {
			if m.width < 30 || m.height < 18 {
				return nil
			}
			width := max(16, m.width-2)
			count := len(strings.Split(ansi.Hardwrap(approvalText(m.operationApproval.Details), width, true), "\n"))
			limit := max(3, m.height-15)
			if m.approvalScroll+limit < count {
				return nil
			}
		}
		return m.resolveOperation(m.confirmAccept)
	}
	return nil
}

func (m *Model) resolveOperation(approved bool) tea.Cmd {
	if m.operationApproval == nil {
		return nil
	}
	id := m.operationApproval.ID
	if m.decidedApprovals == nil || len(m.decidedApprovals) >= 128 {
		m.decidedApprovals = make(map[string]bool)
	}
	m.decidedApprovals[id] = true
	m.approvalBusy = true
	// Disable repeated Enter while the authenticated control response is pending.
	m.operationApproval = nil
	m.screen = screenDashboard
	return m.call("approval_resolved", "approval.resolve", struct {
		ID       string `json:"id"`
		Approved bool   `json:"approved"`
	}{id, approved}, &struct{}{})
}

// Escape terminal control sequences and bidi formatting before displaying
// untrusted commands. Preserve line breaks so the entire request can be read.
func approvalText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r != '\n' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Model) renderOperationApproval() []string {
	p := m.operationApproval
	if p == nil {
		return nil
	}
	header := []string{m.label("远端操作确认", "Confirm remote operation"), approvalText(fmt.Sprintf("%s  %s  revision=%d", p.Protocol, p.Target, p.Revision)), approvalText("operation=" + p.Operation), m.label("请审阅全部内容；允许只对本次请求有效。", "Review the entire request; approval applies once."), ""}
	width := max(16, m.width-2)
	details := strings.Split(ansi.Hardwrap(approvalText(p.Details), width, true), "\n")
	limit := max(3, m.height-len(header)-10)
	m.approvalScroll = min(m.approvalScroll, max(0, len(details)-limit))
	end := min(len(details), m.approvalScroll+limit)
	header = append(header, details[m.approvalScroll:end]...)
	header = append(header, fmt.Sprintf("[%d–%d / %d]  expires=%s", m.approvalScroll+1, end, len(details), p.ExpiresAt.Local().Format("15:04:05")), "")
	// Require the final page before enabling Allow. Default remains Deny.
	canAllow := end == len(details) && m.width >= 30 && m.height >= 18
	allow := m.label("允许本次操作", "Allow once")
	if !canAllow {
		allow = m.label("先向下滚动查看全部内容", "Scroll down to review all details")
	}
	header = append(header, m.menuRow(m.label("拒绝", "Deny"), !m.confirmAccept), m.menuRow(allow, m.confirmAccept), m.label("↑ ↓ 滚动 · Tab 切换 · Enter 确认 · Esc 拒绝", "↑ ↓ scroll · Tab select · Enter confirm · Esc deny"))
	return header
}
