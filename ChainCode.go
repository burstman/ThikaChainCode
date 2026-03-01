package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const roles = "org_admin,record_editor"

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

func (s *SmartContract) CreateBusinessDataRecord(ctx contractapi.TransactionContextInterface,
	businessdata string) (*LedgerRecord, error) {
	businessdataBytes := []byte(businessdata)
	// 1. Get the full Transaction ID (64 chars)
	fullTxID := ctx.GetStub().GetTxID()

	// 2. Create a 12-Character ID (Deterministic)
	// We take the first 12 characters.
	// Example: "e5b38f9a2d1c..." -> "e5b38f9a2d1c"
	shortID := fullTxID[:12]
	recordID := fmt.Sprintf("REC-%s", shortID)

	// 3. SAFETY CHECK: Collision Detection
	// Even with 281 trillion possibilities, it is good practice to check.
	exists, err := s.OrderExists(ctx, recordID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("record with ID %s already exists", recordID)
	}

	//4. Validate and Unmarshal the input string into a generic interface map
	var bizDataInterface interface{}
	if err := json.Unmarshal(businessdataBytes, &bizDataInterface); err != nil {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	actor, err := s.getClientActor(ctx)
	if err != nil {
		return nil, err
	}

	// 3. check permission
	targetRoles := strings.Split(roles, ",")
	if err = AssertClientOrgAndAttribute(ctx, *actor, "role", targetRoles...); err != nil {
		return nil, err // Returns: "authorization failed for attribute 'role': ..."
	}

	// 4. Initialize the struct
	// Note: We do not need to explicitly define the types for Delivery and Payment
	// inside the literal if we are just using zero-values, but here is how
	// you would initialize them if needed.

	record := &LedgerRecord{
		DocType:      "LedgerRecord",
		RecordID:     recordID,
		Actor:        *actor,
		CreatedAt:    timestamp,
		BusinessData: bizDataInterface,
		Status: Status{Code: "CREATED",
			UpdatedAt: timestamp,
		}, // Default initial status
		Locked: false,
	}
	// 5. Save to State
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	err = ctx.GetStub().PutState(recordID, recordJSON)
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
) (*LedgerRecord, error) {
	// Convert the string input to bytes for processing
	businessDataBytes := []byte(newBusinessData)

	// 1. Read record
	record, err := s.ReadRecord(ctx, recordID)
	if err != nil {
		return nil, err
	}

	// 2. ENFORCE POLICY
	// Pass the record pointer. If EnforceLockPolicy determines it should be locked,
	// it will set record.Locked = true on this specific object instance.
	err = s.enforceLockPolicy(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("failed to enforce lock policy: %v", err)
	}

	// 2. Check lock
	if record.Locked {
		return nil, fmt.Errorf("record %s is locked and cannot be modified", recordID)
	}

	// 3. Permission check
	err = AssertClientOrgAndAttribute(ctx, record.Actor, "role", "record_editor")
	if err != nil {
		return nil, err
	}

	// 4. Validate JSON
	if !json.Valid(businessDataBytes) {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	// 5. Optional: enforce same org ownership
	callerOrg, err := GetClientOrgMSPKey(ctx)
	if err != nil {
		return nil, err
	}
	if record.Actor.OrgMSP != callerOrg {
		return nil, fmt.Errorf("organization %s cannot modify record owned by %s",
			callerOrg, record.Actor.OrgMSP)
	}

	// 6. Update data
	record.BusinessData = json.RawMessage(newBusinessData)

	// (Optional) Update status automatically
	record.Status.Code = "UPDATED"

	// Update timestamp
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	record.Status.UpdatedAt = timestamp

	// 7. Persist
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}

	return record, ctx.GetStub().PutState(recordID, recordJSON)

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
	pageSizeStr string,
	bookmark string,
) (*PaginatedResponse, error) {

	// 1. Parse and Validate pageSize
	var pageSize int32
	if pageSizeStr == "" {
		pageSize = 10
	} else {
		pageSize64, err := strconv.ParseInt(pageSizeStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid pageSize format: must be a number")
		}
		pageSize = int32(pageSize64)
	}

	// 2. Validate Input Dates
	if _, err := time.Parse(time.RFC3339, startStr); err != nil {
		return nil, fmt.Errorf("invalid start time format (use RFC3339): %v", err)
	}
	if _, err := time.Parse(time.RFC3339, endStr); err != nil {
		return nil, fmt.Errorf("invalid end time format (use RFC3339): %v", err)
	}

	// 3. Construct CouchDB Query
	// CRITICAL FIX: We use an "$or" operator here to catch both "ledgerRecord" and "LedgerRecord"
	// to handle the inconsistency in your creation functions.
	// Ideally, you should standardize your Create functions to use one or the other.
	queryMap := map[string]any{
		"selector": map[string]any{
			"docType": "LedgerRecord",
			"createdAt": map[string]any{
				"$gte": startStr,
				"$lte": endStr,
			},
		},
		// sorting,  index (META-INF/statedb/couchdb/indexes/index.json)
		"sort": []map[string]string{{"createdAt": "asc"}},
	}

	queryBytes, err := json.Marshal(queryMap)
	if err != nil {
		return nil, fmt.Errorf("failed to construct query: %v", err)
	}
	queryString := string(queryBytes)

	// 4. Execute Pagination
	resultsIterator, responseMetadata, err := ctx.GetStub().GetQueryResultWithPagination(queryString, pageSize, bookmark)
	if err != nil {
		return nil, fmt.Errorf("failed to execute paginated query: %w", err)
	}
	defer resultsIterator.Close()

	// 5. Iterate and Parse Results
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

		// 6. Apply Dynamic Policy (Lazy Enforcement)
		_ = s.enforceLockPolicy(ctx, &record)

		records = append(records, &record)
	}

	// 7. Construct Response
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

// MigrateDocType is a utility function to batch update the docType of assets.
// It is generic and works for LedgerRecords, LockPolicies, or any other JSON asset.
//
// Args:
//
//	oldDocType: The value to search for (e.g., "ledgerRecord")
//	newDocType: The value to replace it with (e.g., "LedgerRecord")
func (s *SmartContract) MigrateDocType(ctx contractapi.TransactionContextInterface,
	oldDocType string,
	newDocType string) (string, error) {

	// 1. Input Validation
	if oldDocType == "" || newDocType == "" {
		return "", fmt.Errorf("oldDocType and newDocType must not be empty")
	}
	if oldDocType == newDocType {
		return "", fmt.Errorf("oldDocType and newDocType are the same, nothing to do")
	}

	// 2. Construct Query
	// We look for any asset where "docType" matches the old value.
	queryString := fmt.Sprintf(`{"selector":{"docType":"%s"}}`, oldDocType)

	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return "", fmt.Errorf("failed to get query result: %v", err)
	}
	defer resultsIterator.Close()

	// 3. Iterate and Update
	counter := 0
	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return "", fmt.Errorf("failed to iterate: %v", err)
		}

		// 4. Generic Unmarshal
		// We use map[string]interface{} so we can handle ANY JSON structure
		// (LedgerRecord, LockPolicy, etc.) without needing the specific struct.
		var asset map[string]interface{}
		if err := json.Unmarshal(response.Value, &asset); err != nil {
			// If it's not JSON, we skip it or log an error.
			// Here we return error to be safe.
			return "", fmt.Errorf("failed to unmarshal asset %s: %v", response.Key, err)
		}

		// 5. Update the docType field
		asset["docType"] = newDocType

		// 6. Marshal back to bytes
		updatedBytes, err := json.Marshal(asset)
		if err != nil {
			return "", fmt.Errorf("failed to marshal asset %s: %v", response.Key, err)
		}

		// 7. Save to Ledger
		// This overwrites the existing key with the updated JSON
		err = ctx.GetStub().PutState(response.Key, updatedBytes)
		if err != nil {
			return "", fmt.Errorf("failed to put state for %s: %v", response.Key, err)
		}

		counter++
	}

	return fmt.Sprintf("Migration successful. Updated %d records from '%s' to '%s'.", counter, oldDocType, newDocType), nil
}
