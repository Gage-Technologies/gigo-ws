package models

type WorkspaceState int

const (
	WorkspaceStateActive WorkspaceState = iota
	WorkspaceStateStopped
	WorkspaceStateDestroyed
)

func (s WorkspaceState) String() string {
	switch s {
	case WorkspaceStateActive:
		return "Active"
	case WorkspaceStateStopped:
		return "Stopped"
	case WorkspaceStateDestroyed:
		return "Destroyed"
	default:
		return "Unknown"
	}
}
