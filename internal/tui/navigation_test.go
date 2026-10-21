package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"ssh-mcp/internal/control"
	"ssh-mcp/internal/store"
)

func TestRepeatedFingerprintConfirmationCannotOverwriteSuccessfulSave(t *testing.T) {
	caller := &recordingCaller{fingerprint: "SHA256:fixture"}
	model := NewModel(caller)
	model.status.Unlocked = true
	model.beginSSHForm(store.SSHTarget{IP: "192.0.2.10", Mode: store.SSHDirect, Enabled: true})
	model.form.fields[2].value = "ops"
	model.form.fields[3].value = "fixture-password"
	finishRPC(model, model.submitForm())
	if model.screen != screenFingerprintConfirm {
		t.Fatal("fingerprint confirmation was not requested")
	}
	command := model.handleFingerprintConfirmation("y")
	if command == nil || model.handleFingerprintConfirmation("y") != nil {
		t.Fatal("repeated confirmation dispatched another validation")
	}
	model.confirmAccept = true
	if press(model, tea.KeyEnter) != nil || press(model, tea.KeyEsc) != nil {
		t.Fatal("keyboard input dispatched or canceled an in-flight validation")
	}
	if !strings.Contains(model.View().Content, "正在验证并保存") {
		t.Fatal("in-flight validation has no progress notice")
	}
	validation := command().(rpcMsg)
	finishRPC(model, model.applyRPC(validation))
	if !caller.saved || model.pending != nil || model.screen != screenTargets {
		t.Fatal("target was not saved")
	}
	success := model.notice
	if !strings.Contains(success, "配置已保存") {
		t.Fatalf("missing save confirmation: %q", success)
	}
	validation.err = errors.New("late validation failure")
	if model.applyRPC(validation) != nil || model.notice != success {
		t.Fatal("a stale validation failure overwrote the successful save")
	}
}

func TestBackgroundStatusFailureDoesNotReplaceSaveResult(t *testing.T) {
	model := NewModel(nil)
	model.notice = "目标验证通过，配置已保存。"
	model.screen = screenTargets
	model.applyRPC(rpcMsg{action: "status", err: errors.New("local transport disconnected")})
	if model.notice != "目标验证通过，配置已保存。" || !strings.Contains(model.connectionNotice, "不表示目标保存失败") {
		t.Fatalf("save result was replaced: notice=%q, connection=%q", model.notice, model.connectionNotice)
	}
	model.applyRPC(rpcMsg{action: "status", value: control.Status{Unlocked: true}})
	if model.connectionNotice != "" || !strings.Contains(model.notice, "配置已保存") {
		t.Fatal("successful status refresh did not clear only the connection notice")
	}
}

func TestLateTargetReplyCannotCancelNewAttempt(t *testing.T) {
	model := NewModel(&recordingCaller{})
	model.status.Unlocked = true
	model.beginSSHForm(store.SSHTarget{IP: "192.0.2.10", Mode: store.SSHDirect, Enabled: true})
	model.form.fields[2].value = "ops"
	model.form.fields[3].value = "fixture-password"
	first := model.submitForm()().(rpcMsg)
	first.err = errors.New("validation failed")
	model.applyRPC(first)
	if model.pending != nil {
		t.Fatal("failed validation cannot be retried")
	}
	model.form.fields[3].value = "fixture-password"
	next := model.submitForm()
	if next == nil || model.pending == nil {
		t.Fatal("retry did not start")
	}
	request := model.pending.requestID
	model.applyRPC(first)
	if model.pending == nil || model.pending.requestID != request || !model.pending.busy {
		t.Fatal("stale reply canceled the new attempt")
	}
	finishRPC(model, next)
}

func press(m *Model, code rune) tea.Cmd { return m.handleKey(tea.KeyPressMsg{Code: code}) }
func finishRPC(m *Model, command tea.Cmd) {
	for command != nil {
		command = m.applyRPC(command().(rpcMsg))
	}
}

func TestArrowAndTabJourneyRegistersPrivateKeyTarget(t *testing.T) {
	caller := &recordingCaller{fingerprint: "SHA256:fixture"}
	model := NewModel(caller)
	model.status.Unlocked = true
	press(model, tea.KeyDown)
	finishRPC(model, press(model, tea.KeyEnter))
	if model.screen != screenTargets {
		t.Fatal("menu did not open targets")
	}
	press(model, tea.KeyTab)
	if model.targetAction != 0 {
		t.Fatal("first Tab skipped Add SSH")
	}
	press(model, tea.KeyEnter)
	if model.screen != screenForm {
		t.Fatal("Add SSH not activated")
	}
	values := map[int]string{0: "192.0.2.10", 1: "22", 2: "ops", 9: "/tmp/test-key", 4: "", 5: "test host", 6: "test", 7: "true"}
	for range 9 {
		if model.form.index == 8 {
			press(model, tea.KeyRight)
		} else {
			model.input.SetValue(values[model.form.index])
		}
		press(model, tea.KeyTab)
	}
	if model.formButton != 1 || !strings.Contains(model.View().Content, "▶ 验证并保存") {
		t.Fatalf("save button not reachable: %s", model.View().Content)
	}
	finishRPC(model, press(model, tea.KeyEnter))
	if model.screen != screenFingerprintConfirm || model.confirmAccept {
		t.Fatal("fingerprint confirmation must default to Back")
	}
	press(model, tea.KeyRight)
	finishRPC(model, press(model, tea.KeyEnter))
	if !caller.saved || caller.savedSSH.Target.AuthMethod != "private_key" || caller.savedSSH.Target.PrivateKeyPath != "/tmp/test-key" || caller.savedSSH.Password != "" {
		t.Fatalf("saved=%+v", caller.savedSSH)
	}
}

func TestKeyUnlockPasteMaskAndCancellation(t *testing.T) {
	model := NewModel(nil)
	model.status.Unlocked = true
	model.beginKeyUnlock("/tmp/test-key")
	model.handlePaste(" pass phrase with spaces ")
	if model.input.Value() != " pass phrase with spaces " || strings.Contains(model.View().Content, "pass phrase") {
		t.Fatal("passphrase was altered or exposed")
	}
	press(model, tea.KeyEsc)
	if model.input.Value() != "" {
		t.Fatal("cancel retained passphrase")
	}
	model.applyRPC(rpcMsg{action: "status", value: control.Status{Unlocked: true, PendingKeys: []string{"/tmp/test-key"}}})
	if model.screen == screenKeyUnlock {
		t.Fatal("polling reopened canceled prompt")
	}
	model.screen = screenDashboard
	press(model, tea.KeyEnter)
	if model.screen != screenKeyUnlock {
		t.Fatal("pending key cannot be reopened from menu")
	}
}

func TestConfirmationEnterDefaultsToCancel(t *testing.T) {
	caller := &recordingCaller{}
	model := NewModel(caller)
	model.status.Unlocked = true
	model.targets.SSH = []store.SSHTarget{{IP: "192.0.2.10"}}
	model.beginTargetDelete()
	press(model, tea.KeyEnter)
	if model.screen != screenTargets || caller.deletedSSH != "" {
		t.Fatal("Enter deleted without selection")
	}
	model.pending = &pendingTargetSave{fingerprint: "SHA256:test"}
	model.screen = screenFingerprintConfirm
	press(model, tea.KeyEnter)
	if model.screen != screenForm || model.pending != nil {
		t.Fatal("Enter trusted without selection")
	}
}

func TestToolbarActionsStayVisibleAtEightyColumns(t *testing.T) {
	model := NewModel(nil)
	model.setLanguage("en")
	model.width = 80
	model.screen = screenTargets
	model.actionFocus, model.targetAction = true, 5
	view := model.View().Content
	if !strings.Contains(view, "▶ Refresh") {
		t.Fatalf("focused action hidden: %s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("overflow: %q", line)
		}
	}
}
