package models

// SetModuleScheduleInput schedules when a module becomes visible to a
// batch's students. Deleting the schedule (DELETE the same route) reverts
// the module to "released from day one".
type SetModuleScheduleInput struct {
	ReleaseDate string `json:"release_date" binding:"required" example:"2026-08-01"`
}

// ModuleScheduleEntry is one configured module-release row for a batch,
// returned by GET /batches/{short_id}/modules/schedule.
type ModuleScheduleEntry struct {
	ModuleShortID string `json:"module_short_id"`
	ModuleName    string `json:"module_name"`
	ReleaseDate   string `json:"release_date"`
}
