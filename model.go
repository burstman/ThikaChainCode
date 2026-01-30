package main

import "time"

type Status struct {
	Code      string `json:"code"`
	Note      string `json:"note"`
	UpdatedAt string `json:"updatedAt"`
}

type LedgerRecord struct {
	DocType   string `json:"docType"` // e.g., "LedgerRecord"
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

// HistoryEntry defines the structure for a single history record.
type HistoryEntry struct {
	TxID      string        `json:"txId"`
	Timestamp time.Time     `json:"timestamp"`
	Value     *LedgerRecord `json:"value"`
	IsDelete  bool          `json:"isDelete"`
}

// PaginatedResponse wraps the results and the bookmark for the next page.
type PaginatedResponse struct {
	Records      []*LedgerRecord `json:"records"`
	Bookmark     string          `json:"bookmark"`
	RecordsCount int32           `json:"recordsCount"`
}

// InvoiceData represents the structure for storing XML invoice information.
// The XML content is expected to be Base64 encoded.
type InvoiceData struct {
	Filename   string `json:"filename"`   // e.g., "invoice-123.xml"
	MIMEType   string `json:"mimeType"`   // "application/xml"
	XMLContent string `json:"xmlContent"` // The Base64 encoded XML string
}
