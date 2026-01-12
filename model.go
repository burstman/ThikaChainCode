package main

type Status struct {
	Code      string `json:code`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt"`
}

type LedgerRecord struct {
	RecordID  string `json:"recordId"`
	Actor     Actor  `json:"actor"`
	CreatedAt string `json:"createdAt"`

	// This tells the schema it can be "Any" type (object), preventing the "Expected array" error.
	BusinessData interface{} `json:"businessData"`

	Status Status `json:"status"`
	Locked bool   `json:"locked"`

	// This ensures the field is always present in the JSON (as ""), satisfying the "required" check.
	LockedAt string `json:"lockedAt"`

	LockPolicyID  string `json:"lockPolicyId"`
	PolicyVersion int    `json:"policyVersion"`
}

type Actor struct {
	OrgMSP string `json:"orgMsp"`
	UserID string `json:"userId"`
}

type LockPolicy struct {
	PolicyID     string `json:"policyId"`
	OrgMSP       string `json:"orgMsp"`
	FinalState   string `json:"finalState"`
	DelaySeconds int64  `json:"delaySeconds"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"createdAt"`
	Active       bool   `json:"active"`
}

type HistoryEntry struct {
	TxID      string        `json:"txId"`
	Timestamp string        `json:"timestamp"`
	Value     *LedgerRecord `json:"value"`
}
