// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

// KindName names a typed record type in output: input state, commands,
// script calls, the ranked result and screenshots. It returns "" for other
// types.
func KindName(t byte) string {
	switch t {
	case TypeHover:
		return "hover"
	case TypePointer:
		return "pointer"
	case TypeAPM:
		return "apm"
	case TypeAim:
		return "aim"
	case TypePlace:
		return "place"
	case TypeStrut:
		return "strut"
	case TypeLink:
		return "link"
	case TypeDeleteLink:
		return "delete_link"
	case TypeDeleteNode:
		return "delete_node"
	case TypeScrap:
		return "scrap"
	case TypeUpgrade:
		return "upgrade"
	case TypeRepair:
		return "repair"
	case TypeDoor:
		return "door"
	case TypeFire:
		return "fire"
	case TypeFireBlocked:
		return "fire_blocked"
	case TypeFireStart:
		return "fire_start"
	case TypeFireEnd:
		return "fire_end"
	case TypeAbility:
		return "ability"
	case TypeRepairLink:
		return "repair_link"
	case TypeRepairDevice:
		return "repair_device"
	case TypeRepairArea:
		return "repair_area"
	case TypePortalLink:
		return "portal_link"
	case TypePortalReverse:
		return "portal_reverse"
	case TypeFireAt:
		return "fire_at"
	case TypeAmmo:
		return "ammo"
	case TypeScriptCall:
		return "script"
	case TypeResult:
		return "result"
	case TypeScreenshot:
		return "screenshot"
	}
	return ""
}
