package model

type DashboardSummary struct {
	TotalIncidents        int64 `json:"total_incidents"`
	CriticalIncidents     int64 `json:"critical_incidents"`
	HighIncidents         int64 `json:"high_incidents"`
	MediumIncidents       int64 `json:"medium_incidents"`
	LowIncidents          int64 `json:"low_incidents"`
	OpenIncidents         int64 `json:"open_incidents"`
	AcknowledgedIncidents int64 `json:"acknowledged_incidents"`
	ResolvedIncidents     int64 `json:"resolved_incidents"`
}
