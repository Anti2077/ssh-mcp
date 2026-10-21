package runner

import (
	"context"
	"crypto/sha256"
	"fmt"

	"ssh-mcp/internal/approval"
	"ssh-mcp/internal/auditlog"
	"ssh-mcp/internal/policy"
	"ssh-mcp/internal/store"
)

func permissionHardStops(tier store.PermissionTier) []string {
	var ids []string
	for _, id := range policy.HardStopRuleIDs() {
		if tier != store.PermissionReadOnly && id == string(policy.ReasonOpaqueShellEffect) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (e *Engine) authorizeOperation(ctx context.Context, tier store.PermissionTier, readOnly bool, request approval.Request, decision policy.Result, payload string) (Result, bool) {
	tier = store.EffectivePermissionTier(tier)
	deny := func(reason, message string) (Result, bool) {
		decision.Decision = policy.DecisionRejected
		decision.Reason = policy.Reason(reason)
		result, err := e.rejectedResult(ctx, request.Protocol, request.Target, payload, decision)
		if err != nil {
			result = notDispatchedResult(StatusRejected, decision, payload)
			result.AuditWriteFailed = true
			result.AuditOutcome = AuditOutcomeFailed
		}
		result.Message = message
		return result, false
	}
	if !store.ValidPermissionTier(tier) {
		return deny("permission_invalid", "目标权限配置无效，操作未执行。")
	}
	if tier == store.PermissionReadOnly && !readOnly {
		return deny("permission_read_only", "目标为只读巡检：该操作无法明确判定为读取，未执行。请由本地用户调整目标权限。")
	}
	if tier != store.PermissionConfirm || readOnly {
		return Result{}, true
	}
	request.Owner = e.deps.ExecutionOwner
	// The digest identifies the exact displayed operation without persisting
	// arbitrary commands, environment values or source paths in approval events.
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%+v", request))))
	confirmationID, err := newID()
	if err != nil {
		return deny("confirmation_unavailable", "无法创建本地确认请求，操作未执行。")
	}
	log := func(status string) error {
		if e.deps.Audit == nil {
			return nil
		}
		return e.deps.Audit.Record(ctx, auditlog.Event{OperationID: confirmationID, Phase: auditlog.PhaseDecision, Action: "permission_confirmation", Actor: e.deps.AuditActor, Target: auditlog.Target{Kind: request.Protocol, ID: request.Target}, Policy: auditlog.Policy{Version: policy.Version, Decision: status, Reason: "permission_confirm:" + digest}, Result: auditlog.Result{Status: status}})
	}
	if err := log("confirmation_requested"); err != nil {
		return deny("confirmation_audit_failed", "确认审计写入失败，操作未执行。")
	}
	if err := e.deps.Approvals.Wait(ctx, request, e.deps.OpenTUI); err != nil {
		_ = log("confirmation_denied")
		return deny("confirmation_denied", "未收到本地用户确认（拒绝、取消、超时或管理界面不可用），操作未执行。请在本地 ssh-mcp manage 中确认操作。")
	}
	if err := log("confirmation_approved"); err != nil {
		return deny("confirmation_audit_failed", "确认审计写入失败，操作未执行。")
	}
	// Configuration and unlock state may have changed while the user reviewed
	// the request. Transport pinning and dispatch leases check again at dispatch.
	if _, err := e.vault(); err != nil {
		return deny("confirmation_stale", "凭据解锁状态已改变，确认失效，操作未执行。")
	}
	if request.Protocol == "ssh" {
		current, err := e.sshTarget(ctx, request.Target)
		if err != nil || !sshTargetAvailable(current) || current.Revision != request.Revision || store.EffectivePermissionTier(current.PermissionTier) != tier {
			return deny("confirmation_stale", "目标配置已改变，确认失效，操作未执行。")
		}
	} else {
		current, _, err := e.lookupDatabaseTarget(ctx, request.Target)
		if err != nil || !current.Enabled || current.Revision != request.Revision || store.EffectivePermissionTier(current.PermissionTier) != tier {
			return deny("confirmation_stale", "目标配置已改变，确认失效，操作未执行。")
		}
	}
	return Result{}, true
}
