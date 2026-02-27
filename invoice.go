package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const (
	// MaxXMLFileSize is the limit for the original XML file (1 MB).
	MaxXMLFileSize = 1 * 1024 * 1024

	// MaxBase64Size is the calculated limit for the encoded string.
	// Base64 encoding represents 3 bytes of binary data as 4 characters,
	// so the size increases by a factor of 4/3.
	MaxBase64Size = (4 * MaxXMLFileSize / 3)
)

// validateXMLContent checks if the base64 string decodes to valid XML
func validateXMLContent(xmlBase64 string) error {
	// 1. Security Check: File Size
	inputSize := len(xmlBase64)
	if inputSize > MaxBase64Size {
		return fmt.Errorf("invoice file too large: %d bytes. Max allowed is %d bytes", inputSize, MaxBase64Size)
	}
	if inputSize == 0 {
		return fmt.Errorf("invoice content cannot be empty")
	}

	// 2. Integrity Check: Base64 Decoding
	decodedBytes, err := base64.StdEncoding.DecodeString(xmlBase64)
	if err != nil {
		return fmt.Errorf("invalid base64 encoding: %v", err)
	}

	// 3. Content Check: XML Validation
	decoder := xml.NewDecoder(bytes.NewReader(decodedBytes))
	hasRootElement := false
	for {
		t, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("content is not valid XML: %v", err)
		}
		if _, ok := t.(xml.StartElement); ok {
			hasRootElement = true
			break
		}
	}
	if !hasRootElement {
		return fmt.Errorf("content is not valid XML: no root element found")
	}

	return nil
}

// CreateInvoiceRecord creates a new record in the ledger for an XML invoice.
func (s *SmartContract) CreateInvoiceRecord(
	ctx contractapi.TransactionContextInterface,
	filename string,
	xmlBase64 string,
) (*LedgerRecord, error) {

	// 1. Validate Content (Size, Base64, XML)
	if err := validateXMLContent(xmlBase64); err != nil {
		return nil, err
	}

	// 2. Generate ID
	fullTxID := ctx.GetStub().GetTxID()
	shortID := fullTxID[:12]
	recordID := fmt.Sprintf("REC-%s", shortID)

	// 3. Idempotency Check
	exists, err := s.OrderExists(ctx, recordID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("the record %s already exists", recordID)
	}

	// 4. Get Actor & Permission
	actor, err := s.getClientActor(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get client actor: %v", err)
	}

	err = AssertClientOrgAndAttribute(ctx, *actor, "role", "org_admin")
	if err != nil {
		return nil, err
	}

	invoiceData := InvoiceData{
		Filename:   filename,
		MIMEType:   "application/xml",
		XMLContent: xmlBase64,
	}

	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction timestamp: %v", err)
	}

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

	recordBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %v", err)
	}

	err = ctx.GetStub().PutState(recordID, recordBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to put state: %v", err)
	}

	return record, nil
}

// UpdateInvoiceRecord updates an existing invoice record with a new XML file.
func (s *SmartContract) UpdateInvoiceRecord(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	newFilename string,
	newXmlBase64 string,
) (*LedgerRecord, error) {

	// 1️⃣ SECURITY CHECK: Validate New File Size
	if err := validateXMLContent(newXmlBase64); err != nil {
		return nil, err
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

	// 2️⃣ INTEGRITY CHECK: Validate Base64 AND XML Content
	decodedBytes, err := base64.StdEncoding.DecodeString(newXmlBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 encoding: %v", err)
	}

	// Verify the content is actually XML
	// We use a decoder to scan the tokens. If it encounters syntax errors, it fails.
	decoder := xml.NewDecoder(bytes.NewReader(decodedBytes))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("content is not valid XML: %v", err)
		}
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
