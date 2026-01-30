package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const (
	// MaxXMLFileSize is the limit for the original XML file (1 MB).
	MaxXMLFileSize = 1 * 1024 * 1024

	// MaxBase64Size is the calculated limit for the encoded string.
	// Base64 is 4/3 larger than the original binary.
	MaxBase64Size = (MaxXMLFileSize * 137) / 100
)

// CreateInvoiceRecord creates a new record in the ledger for an XML invoice.
func (s *SmartContract) CreateInvoiceRecord(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	filename string,
	xmlBase64 string,
) (*LedgerRecord, error) {

	// 1️⃣ SECURITY CHECK: Validate File Size
	// We check the length of the input string immediately.
	// len() in Go returns the number of bytes in the string.
	inputSize := len(xmlBase64)

	if inputSize > MaxBase64Size {
		return nil, fmt.Errorf("invoice file too large: %d bytes. Max allowed is %d bytes (approx 1MB original file)", inputSize, MaxBase64Size)
	}

	// 2️⃣ Check for empty payload
	if inputSize == 0 {
		return nil, fmt.Errorf("invoice content cannot be empty")
	}

	// --- Continue with existing logic ---

	// Get client identity
	actor, err := s.getClientActor(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get client actor: %v", err)
	}
	// Permission check: Only ORG ADMIN can create records
	err = AssertClientOrgAndAttribute(ctx, *actor, "role", "org_admin")
	if err != nil {
		return nil, err
	}

	// Create the business data payload
	invoiceData := InvoiceData{
		Filename:   filename,
		MIMEType:   "application/xml",
		XMLContent: xmlBase64,
	}

	// Get transaction timestamp
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction timestamp: %v", err)
	}

	// Assemble the record
	record := &LedgerRecord{
		DocType:       "LedgerRecord",
		RecordID:      recordID,
		Actor:         *actor,
		CreatedAt:     timestamp,
		BusinessData:  invoiceData,
		Status:        Status{Code: "CREATED", UpdatedAt: timestamp},
		Locked:        false,
		LockedAt:      "",
		PolicyVersion: 0,
	}
	// Marshal and save
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %v", err)
	}

	return record, ctx.GetStub().PutState(recordID, recordBytes)
}

// UpdateInvoiceRecord updates an existing invoice record with a new XML file.
func (s *SmartContract) UpdateInvoiceRecord(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	newFilename string,
	newXmlBase64 string,
) (*LedgerRecord, error) {

	// 1️⃣ SECURITY CHECK: Validate New File Size
	// We reuse the constants defined previously
	inputSize := len(newXmlBase64)
	if inputSize > MaxBase64Size {
		return nil, fmt.Errorf("new invoice file too large: %d bytes. Max allowed is %d bytes", inputSize, MaxBase64Size)
	}

	if inputSize == 0 {
		return nil, fmt.Errorf("invoice content cannot be empty")
	}

	// 2️⃣ Retrieve the existing record
	recordBytes, err := ctx.GetStub().GetState(recordID)
	if err != nil {
		return nil, fmt.Errorf("failed to read record: %v", err)
	}
	if recordBytes == nil {
		return nil, fmt.Errorf("record %s does not exist", recordID)
	}

	// 3️⃣ Unmarshal the record
	var record LedgerRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %v", err)
	}

	// 4️⃣ LOGIC CHECK: Is the record Locked?
	// If the record is locked, no updates are allowed.
	if record.Locked {
		return nil, fmt.Errorf("record %s is LOCKED and cannot be updated", recordID)
	}

	// 5️⃣ PERMISSION CHECK: Verify Ownership
	err = AssertClientOrgAndAttribute(ctx, record.Actor, "role", "org_admin")
	if err != nil {
		return nil, err
	}

	// 6️⃣ Update the Record
	// Get current timestamp
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get timestamp: %v", err)
	}

	// Create new Invoice Data
	newInvoiceData := InvoiceData{
		Filename:   newFilename,
		MIMEType:   "application/xml",
		XMLContent: newXmlBase64,
	}

	// Update fields
	record.BusinessData = newInvoiceData
	record.Status.UpdatedAt = timestamp
	record.Status.Code = "UPDATED" // Optional: Change status code to indicate update

	err = s.enforceLockPolicy(ctx, &record)
	if err != nil {
		return nil, fmt.Errorf("failed to enforce lock policy: %v", err)
	}

	// 7️⃣ Marshal and Save
	updatedRecordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated record: %v", err)
	}

	err = ctx.GetStub().PutState(recordID, updatedRecordBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to put state: %v", err)
	}

	return &record, nil
}
