package state

// OfficeState contains display inputs, never DOS addresses or a serialized roster.
type OfficeState struct {
	PilotName         string `json:"pilot_name"`
	Rank              int    `json:"rank"`
	Squadron          string `json:"squadron"`
	Emblem            int    `json:"emblem"`
	MissionType       int    `json:"mission_type"`
	Theatre           int    `json:"theatre"`
	Active            bool   `json:"active"`
	SnapshotAvailable bool   `json:"snapshot_available"`
	ComputerMode      int    `json:"computer_mode"`
}
