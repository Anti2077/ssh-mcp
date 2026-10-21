package store

// PermissionTier is editable only through the trusted local control plane.
type PermissionTier string

const (
	PermissionReadOnly PermissionTier = "read_only"
	PermissionConfirm  PermissionTier = "confirm"
	PermissionFull     PermissionTier = "full"
)

func EffectivePermissionTier(tier PermissionTier) PermissionTier {
	if tier == "" {
		return PermissionConfirm
	}
	return tier
}

func ValidPermissionTier(tier PermissionTier) bool {
	switch tier {
	case PermissionReadOnly, PermissionConfirm, PermissionFull:
		return true
	}
	return false
}
