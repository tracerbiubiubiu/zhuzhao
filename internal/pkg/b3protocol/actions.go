package b3protocol

const (
	Report = "ticket.evidence.report.generate"
	Update = "activelist.document.update"
	Read   = "activelist.document.get"
)

func IsAction(action string) bool {
	switch action {
	case Report, Update, Read:
		return true
	default:
		return false
	}
}
