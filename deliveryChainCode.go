package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract provides functions for managing an Order
type SmartContract struct {
	contractapi.Contract
}

func main() {
	chaincode, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		log.Panicf("Error creating delivery-proof chaincode: %v", err)
	}

	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting delivery-proof chaincode: %v", err)
	}
}

func (s *SmartContract) CreateRecord(ctx contractapi.TransactionContextInterface,
	RecordID string,
	businessdata string) (*LedgerRecord, error) {
	businessdataBytes := []byte(businessdata)

	// 1.check permission
	err := AssertClientAttribute(ctx, "role", "record_creator")

	// Check for system errors or denial
	if err != nil {
		return nil, err // Returns: "authorization failed for attribute 'role': ..."
	}

	// 2. Check if order already exists
	exists, err := s.OrderExists(ctx, RecordID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("record  %s already exists", RecordID)
	}
	// Validate and Unmarshal the input string into a generic interface map
	var bizDataInterface interface{}
	if err := json.Unmarshal(businessdataBytes, &bizDataInterface); err != nil {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	if !json.Valid(businessdataBytes) {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	clientUserID, err := GetClientIdentity(ctx)
	if err != nil {
		return nil, err
	}
	mspOrg, err := GetClientOrgMSPKey(ctx)
	if err != nil {
		return nil, err
	}

	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	actor := Actor{
		OrgMSP: mspOrg,
		UserID: clientUserID,
	}

	// 2. Initialize the struct
	// Note: We do not need to explicitly define the types for Delivery and Payment
	// inside the literal if we are just using zero-values, but here is how
	// you would initialize them if needed.
	record := &LedgerRecord{
		DocType:      "ledgerRecord",
		RecordID:     RecordID,
		Actor:        actor,
		CreatedAt:    timestamp,
		BusinessData: bizDataInterface,
		Status:       Status{Code: "CREATED"}, // Default initial status
		Locked:       false,
	}

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	err = ctx.GetStub().PutState(RecordID, recordJSON)
	if err != nil {
		return nil, err
	}

	return record, nil
}

// UpdateBusinessData updates the business-specific data of a delivery record.
// It retrieves the record by ID, modifies the data field, and commits the change.
func (s *SmartContract) UpdateBusinessData(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	newBusinessData string,
) error {
	// Convert the string input to bytes for processing
	businessDataBytes := []byte(newBusinessData)

	// 1. Read record
	record, err := s.ReadRecord(ctx, recordID)
	if err != nil {
		return err
	}

	// 2. ENFORCE POLICY
	// Pass the record pointer. If EnforceLockPolicy determines it should be locked,
	// it will set record.Locked = true on this specific object instance.
	err = s.enforceLockPolicy(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to enforce lock policy: %v", err)
	}

	// 2. Check lock
	if record.Locked {
		return fmt.Errorf("record %s is locked and cannot be modified", recordID)
	}

	// 3. Permission check
	err = AssertClientAttribute(ctx, "role", "record_editor")
	if err != nil {
		return err
	}

	// 4. Validate JSON
	if !json.Valid(businessDataBytes) {
		return fmt.Errorf("businessData must be valid JSON")
	}

	// 5. Optional: enforce same org ownership
	callerOrg, err := GetClientOrgMSPKey(ctx)
	if err != nil {
		return err
	}
	if record.Actor.OrgMSP != callerOrg {
		return fmt.Errorf("organization %s cannot modify record owned by %s",
			callerOrg, record.Actor.OrgMSP)
	}

	// 6. Update data
	record.BusinessData = json.RawMessage(newBusinessData)

	// (Optional) Update status automatically
	record.Status.Code = "UPDATED"

	// 7. Persist
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(recordID, recordJSON)

}

func (s *SmartContract) UpdateRecord(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	status string, // Example argument
) error {

	// 1️⃣ READ THE RECORD
	// Fabric caches reads within a transaction, so this is performant.
	record, err := s.ReadRecord(ctx, recordID)
	if err != nil {
		return err
	}

	// 2️⃣ ENFORCE POLICY
	// Pass the record pointer. If EnforceLockPolicy determines it should be locked,
	// it will set record.Locked = true on this specific object instance.
	err = s.enforceLockPolicy(ctx, record)
	if err != nil {
		return fmt.Errorf("failed to enforce lock policy: %v", err)
	}

	// 3️⃣ CHECK LOCK STATUS
	// If EnforceLockPolicy just ran and determined time is up, record.Locked is now true.
	if record.Locked {
		// We return an error here.
		// NOTE: In Fabric, returning an error aborts the transaction.
		// This means the "Locked=true" change from step 2 is NOT saved to the ledger.
		// This is acceptable for "Lazy Enforcement" because the next time someone tries
		// to touch this record, EnforceLockPolicy will run again and block them again.
		return fmt.Errorf("record %s is locked and cannot be updated", recordID)
	}

	// 4️⃣ APPLY UPDATES
	// If we are here, the record is safe to update.
	record.Status.Code = status
	record.Status.UpdatedAt, err = s.getTxTimestamp(ctx) // Update timestamp
	if err != nil {
		return err
	}

	// 5️⃣ SAVE
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(recordID, recordJSON)
}

// GetRecordHistoryByID returns the history of a record within a specific time interval.
// Pass empty strings "" for startStr or endStr to ignore that boundary.
// Date Format: RFC3339 (e.g., "2024-01-01T00:00:00Z")
func (s *SmartContract) GetRecordHistoryByID(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	startStr string,
	endStr string,
) ([]HistoryEntry, error) {

	// 1. Parse Time Filters
	var startTime, endTime time.Time
	var err error

	// Parse Start Time if provided
	if startStr != "" {
		startTime, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start time format (use RFC3339, e.g., 2024-01-01T00:00:00Z): %v", err)
		}
	}

	// Parse End Time if provided
	if endStr != "" {
		endTime, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end time format (use RFC3339): %v", err)
		}
	}
	ledger, err := s.ReadRecord(ctx, recordID)

	// ENFORCE POLICY ON CURRENT RECORD
	err = s.enforceLockPolicy(ctx, ledger)
	if err != nil {
		return nil, fmt.Errorf("failed to enforce lock policy: %v", err)
	}

	// 2. Get History Iterator
	it, err := ctx.GetStub().GetHistoryForKey(recordID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history for key %s: %w", recordID, err)
	}
	defer it.Close()

	// Initialize empty slice for consistent JSON output
	history := []HistoryEntry{}

	// 3. Iterate and Filter
	for it.HasNext() {
		response, err := it.Next()
		if err != nil {
			return nil, fmt.Errorf("error iterating history: %w", err)
		}

		// Convert Fabric Timestamp (seconds/nanos) to Go Time
		txTime := time.Unix(response.Timestamp.Seconds, int64(response.Timestamp.Nanos)).UTC()

		// Filter: Check Start Boundary
		if !startTime.IsZero() && txTime.Before(startTime) {
			continue
		}

		// Filter: Check End Boundary
		if !endTime.IsZero() && txTime.After(endTime) {
			continue
		}

		var record *LedgerRecord
		// Only unmarshal if not a delete operation
		if response.Value != nil {
			if err := json.Unmarshal(response.Value, &record); err != nil {
				// Log error but maybe don't fail the whole query?
				// For strictness, we return error here.
				return nil, fmt.Errorf("failed to unmarshal history value: %w", err)
			}
		}

		entry := HistoryEntry{
			TxID:      response.TxId,
			Timestamp: txTime, // Return time.Time object
			Value:     record,
			IsDelete:  response.IsDelete,
		}
		history = append(history, entry)
	}

	return history, nil
}

func (s *SmartContract) GetRecordsByDateRange(
	ctx contractapi.TransactionContextInterface,
	startStr string,
	endStr string,
	pageSize int32,
	bookmark string,
) (*PaginatedResponse, error) {

	// 1. Validate Input Dates
	if _, err := time.Parse(time.RFC3339, startStr); err != nil {
		return nil, fmt.Errorf("invalid start time format (use RFC3339): %v", err)
	}
	if _, err := time.Parse(time.RFC3339, endStr); err != nil {
		return nil, fmt.Errorf("invalid end time format (use RFC3339): %v", err)
	}

	// 2. Construct CouchDB Query
	// We use a map to safely construct the JSON query string.
	queryMap := map[string]interface{}{
		"selector": map[string]interface{}{
			"docType": "ledgerRecord", // Ensure your CreateRecord sets this!
			"createdAt": map[string]interface{}{
				"$gte": startStr,
				"$lte": endStr,
			},
		},
		// OPTIONAL: To ensure consistent pagination, it is best practice to sort.
		// However, this requires a CouchDB index on ["createdAt"].
		// "sort": []map[string]string{{"createdAt": "asc"}},
	}

	queryBytes, err := json.Marshal(queryMap)
	if err != nil {
		return nil, fmt.Errorf("failed to construct query: %v", err)
	}
	queryString := string(queryBytes)

	// 3. Execute Pagination
	// This API returns the iterator and metadata (which contains the bookmark).
	resultsIterator, responseMetadata, err := ctx.GetStub().GetQueryResultWithPagination(queryString, pageSize, bookmark)
	if err != nil {
		return nil, fmt.Errorf("failed to execute paginated query: %w", err)
	}
	defer resultsIterator.Close()

	// 4. Iterate and Parse Results
	records := []*LedgerRecord{}

	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var record LedgerRecord
		if err := json.Unmarshal(queryResponse.Value, &record); err != nil {
			return nil, fmt.Errorf("failed to unmarshal record: %w", err)
		}

		// 5. Apply Dynamic Policy (Lazy Enforcement)
		// We run the policy check here so the frontend sees the *actual* effective state
		// (e.g., Locked=true) even if the DB state is technically outdated.
		_ = s.enforceLockPolicy(ctx, &record)

		records = append(records, &record)
	}

	// 6. Construct Response
	return &PaginatedResponse{
		Records:      records,
		Bookmark:     responseMetadata.Bookmark,
		RecordsCount: responseMetadata.FetchedRecordsCount,
	}, nil
}

// ------------------- HELPER FUNCTIONS -------------------

// ReadOrder returns the order stored in the world state with given id.
func (s *SmartContract) ReadRecord(ctx contractapi.TransactionContextInterface, id string) (*LedgerRecord, error) {
	RecordJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %v", err)
	}
	if RecordJSON == nil {
		return nil, fmt.Errorf("record %s does not exist", id)
	}

	var Record LedgerRecord
	err = json.Unmarshal(RecordJSON, &Record)
	if err != nil {
		return nil, err
	}

	return &Record, nil
}

// OrderExists returns true when asset with given ID exists in world state
func (s *SmartContract) OrderExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	orderJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %v", err)
	}

	return orderJSON != nil, nil
}

// getTxTimestamp retrieves the transaction timestamp from the ledger stub.
// This ensures determinism across all peers.
func (s *SmartContract) getTxTimestamp(ctx contractapi.TransactionContextInterface) (string, error) {
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve transaction timestamp: %v", err)
	}

	// Convert protobuf timestamp to Go time.Time
	// txTimestamp.Seconds is int64, txTimestamp.Nanos is int32
	tm := time.Unix(txTimestamp.Seconds, int64(txTimestamp.Nanos))

	return tm.Format(time.RFC3339), nil
}
