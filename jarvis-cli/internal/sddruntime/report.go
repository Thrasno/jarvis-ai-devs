package sddruntime

type IntegrityStatus string

const (
	StatusPass IntegrityStatus = "pass"
	StatusWarn IntegrityStatus = "warn"
	StatusFail IntegrityStatus = "fail"
)

type DriftClass string

const (
	DriftOwned    DriftClass = "owned"
	DriftNonOwned DriftClass = "non-owned"
	DriftNone     DriftClass = "none"
)

type CheckResult struct {
	Key       string
	Status    IntegrityStatus
	DriftClass DriftClass
	Expected  string
	Observed  string
	Message   string
}

type IntegrityReport struct {
	Status          IntegrityStatus
	ContractVersion string
	Agent           string
	Checks          []CheckResult
	Notes           []string
}

func NewIntegrityReport(agent string, contract Contract) IntegrityReport {
	return IntegrityReport{
		Status:          StatusPass,
		ContractVersion: contract.Version,
		Agent:           agent,
		Checks:          []CheckResult{},
		Notes:           []string{},
	}
}

func (r *IntegrityReport) AddCheck(check CheckResult) {
	r.Checks = append(r.Checks, check)
	if severity(check.Status) > severity(r.Status) {
		r.Status = check.Status
	}
}

func severity(status IntegrityStatus) int {
	switch status {
	case StatusFail:
		return 3
	case StatusWarn:
		return 2
	default:
		return 1
	}
}
