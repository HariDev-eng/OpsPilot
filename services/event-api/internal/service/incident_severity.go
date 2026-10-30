package service

func CalculateSeverity(count int64) string {
	switch {
	case count >= 20:
		return "CRITICAL"
	case count >= 5:
		return "HIGH"
	default:
		return ""
	}
}
