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

	// Validate and Unmarshal the input string into a generic interface map
	var bizDataInterface interface{}
	if err := json.Unmarshal(businessdataBytes, &bizDataInterface); err != nil {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	// 1. Ceck if order already exists
	exists, err := s.OrderExists(ctx, RecordID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("order  %s already exists", RecordID)
	}
	// 1.check permission
	err = AssertClientAttribute(ctx, "role", "record_creator")

	// Check for system errors or denial
	if err != nil {
		return nil, err // Returns: "authorization failed for attribute 'role': ..."
	}

	if !json.Valid(businessdataBytes) {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	clientUserOrg, err := GetClientIdentity(ctx)
	if err != nil {
		return nil, err
	}
	mspOrg, err := GetClientOrgMSP(ctx)
	if err != nil {
		return nil, err
	}

	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}
	actor := Actor{
		OrgMSP: mspOrg,
		UserID: clientUserOrg,
	}

	// 2. Initialize the struct
	// Note: We do not need to explicitly define the types for Delivery and Payment
	// inside the literal if we are just using zero-values, but here is how
	// you would initialize them if needed.
	order := &LedgerRecord{
		RecordID:     RecordID,
		Actor:        actor,
		CreatedAt:    timestamp,
		BusinessData: bizDataInterface,
		Status:       Status{Code: "CREATED"}, // Default initial status
		Locked:       false,
	}

	orderJSON, err := json.Marshal(order)
	if err != nil {
		return nil, err
	}

	err = ctx.GetStub().PutState(RecordID, orderJSON)
	if err != nil {
		return nil, err
	}

	return order, nil
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
	callerOrg, err := GetClientOrgMSP(ctx)
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

// UpdateRecordStatus updates the status of an existing delivery record in the ledger.
//
// Parameters:
//
//	ctx    - the transaction context, which provides access to the world state.
//	id     - the unique identifier of the delivery record to be updated.
//	status - the new status string to assign to the record (e.g., "DELIVERED", "IN_TRANSIT").
//
// Returns:
//
//	error - returns an error if the record does not exist, validation fails, or writing to the ledger fails.
func (s *SmartContract) UpdateRecordStatus(
	ctx contractapi.TransactionContextInterface,
	recordID string,
	newStatusCode string,
	note string,
) error {

	// 1. Read record
	record, err := s.ReadRecord(ctx, recordID)
	if err != nil {
		return err
	}

	// 2. Locked records cannot change state
	if record.Locked {
		return fmt.Errorf("record %s is locked and status cannot be changed", recordID)
	}

	// 3. Permission check
	err = AssertClientAttribute(ctx, "role", "status_updater")
	if err != nil {
		return err
	}

	// 4. Optional: same org rule
	callerOrg, err := GetClientOrgMSP(ctx)
	if err != nil {
		return err
	}
	if record.Actor.OrgMSP != callerOrg {
		return fmt.Errorf(
			"organization %s cannot update status of record owned by %s",
			callerOrg,
			record.Actor.OrgMSP,
		)
	}

	// 5. Update status
	record.Status = Status{
		Code: newStatusCode,
		Note: note,
	}

	// 6. Persist
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(recordID, recordJSON)
}

// GetRecordHistory returns the history of a record within a specific time interval.
// Pass empty strings "" for startStr or endStr to ignore that boundary.
// Date Format: RFC3339 (e.g., "2024-01-01T00:00:00Z")
func (s *SmartContract) GetRecordHistory(
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

		var record LedgerRecord
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
			Value:     &record,
			IsDelete:  response.IsDelete,
		}
		history = append(history, entry)
	}

	return history, nil
}

// GetRecordsByDateRange performs a rich query to find records created within a time range.
// Dates must be in RFC3339 format (e.g., "2026-01-12T12:00:00Z").
func (s *SmartContract) GetRecordsByDateRange(ctx contractapi.TransactionContextInterface, startStr string, endStr string) ([]*LedgerRecord, error) {

	// 1. Validate Input Dates
	if _, err := time.Parse(time.RFC3339, startStr); err != nil {
		return nil, fmt.Errorf("invalid start time format (use RFC3339): %v", err)
	}
	if _, err := time.Parse(time.RFC3339, endStr); err != nil {
		return nil, fmt.Errorf("invalid end time format (use RFC3339): %v", err)
	}

	// 2. Construct CouchDB Query String
	// We use $gte (Greater Than or Equal) and $lte (Less Than or Equal)
	queryString := fmt.Sprintf(`{
		"selector": {
			"createdAt": {
				"$gte": "%s",
				"$lte": "%s"
			}
		}
	}`, startStr, endStr)

	// 3. Execute Query
	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	// 4. Iterate and Parse Results
	var records []*LedgerRecord
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var record LedgerRecord
		if err := json.Unmarshal(queryResponse.Value, &record); err != nil {
			return nil, err
		}
		records = append(records, &record)
	}

	return records, nil
}

func (s *SmartContract) EnforceLockPolicy(
	ctx contractapi.TransactionContextInterface,
	recordID string,
) error {

	record, err := s.ReadRecord(ctx, recordID)
	if err != nil {
		return err
	}

	if record.Locked {
		return nil
	}

	orgMSP := record.Actor.OrgMSP

	policy, err := s.LoadLockPolicy(
		ctx,
		orgMSP,
		record.LockPolicyID,
		record.PolicyVersion,
	)
	if err != nil {
		return err
	}

	if !policy.Active {
		return nil
	}

	if record.Status.Code != policy.FinalState {
		return nil
	}

	stateTime, err := time.Parse(time.RFC3339, record.Status.UpdatedAt)
	if err != nil {
		return err
	}

	txTime, err := s.getTxTimestamp(ctx)
	if err != nil {
		return err
	}
	now, _ := time.Parse(time.RFC3339, txTime)

	lockDelay := time.Duration(policy.DelaySeconds) * time.Second

	if now.Sub(stateTime) >= lockDelay {
		record.Locked = true
		record.LockedAt = txTime

		data, _ := json.Marshal(record)
		return ctx.GetStub().PutState(recordID, data)
	}

	return nil
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
