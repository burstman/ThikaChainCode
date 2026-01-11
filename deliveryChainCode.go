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

func (s *SmartContract) CreateRecord(ctx contractapi.TransactionContextInterface, RecordID string, businessdata []byte) (*LedgerRecord, error) {
	// 1. Ceck if order already exists
	exists, err := s.OrderExists(ctx, RecordID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("order  %s already exists", RecordID)
	}
	// 1.check permission
	isAuthorized, err := CheckPermissionClientOrgID(ctx, "role", "record_creator")

	// Check for system errors or denial
	if err != nil {
		return nil, err // Returns: "authorization failed for attribute 'role': ..."
	}

	// Double check boolean (redundant if err handles it, but good for safety)
	if !isAuthorized {
		return nil, fmt.Errorf("access denied")
	}
	if !json.Valid(businessdata) {
		return nil, fmt.Errorf("businessData must be valid JSON")
	}

	clientUserOrg := GetClientIdentity(ctx)
	mspOrg := GetClientOrgMSP(ctx)

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
		BusinessData: businessdata,
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
	newBusinessData []byte,
) error {

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
	isAuthorized, err := CheckPermissionClientOrgID(ctx, "role", "record_editor")
	if err != nil {
		return err
	}
	if !isAuthorized {
		return fmt.Errorf("access denied")
	}

	// 4. Validate JSON
	if !json.Valid(newBusinessData) {
		return fmt.Errorf("businessData must be valid JSON")
	}

	// 5. Optional: enforce same org ownership
	callerOrg := GetClientOrgMSP(ctx)
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
	isAuthorized, err := CheckPermissionClientOrgID(ctx, "role", "status_updater")
	if err != nil {
		return err
	}
	if !isAuthorized {
		return fmt.Errorf("access denied")
	}

	// 4. Optional: same org rule
	callerOrg := GetClientOrgMSP(ctx)
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

func (s *SmartContract) CreateLockPolicy(
	ctx contractapi.TransactionContextInterface,
	finalState string,
	delaySeconds int64,
) (*LockPolicy, error) {

	// 1️⃣ Permission check (ORG ADMIN ONLY)
	isAuthorized, err := CheckPermissionClientOrgID(ctx, "role", "org_admin")
	if err != nil {
		return nil, err
	}
	if !isAuthorized {
		return nil, fmt.Errorf("access denied: org_admin role required")
	}

	// 2️⃣ Identify organization
	orgMSP := GetClientOrgMSP(ctx)

	// 3️⃣ Find latest policy version
	latestVersion := 0
	latestPolicyKey := ""

	resultsIterator, err := ctx.GetStub().GetStateByRange(
		fmt.Sprintf("LOCKPOLICY_%s_v", orgMSP),
		fmt.Sprintf("LOCKPOLICY_%s_v~", orgMSP),
	)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	for resultsIterator.HasNext() {
		kv, _ := resultsIterator.Next()
		var p LockPolicy
		json.Unmarshal(kv.Value, &p)
		if p.Version > latestVersion {
			latestVersion = p.Version
			latestPolicyKey = kv.Key
		}
	}

	// 4️⃣ Deactivate old policy (if exists)
	if latestPolicyKey != "" {
		oldPolicyBytes, _ := ctx.GetStub().GetState(latestPolicyKey)
		var oldPolicy LockPolicy
		json.Unmarshal(oldPolicyBytes, &oldPolicy)

		oldPolicy.Active = false
		updatedBytes, _ := json.Marshal(oldPolicy)
		ctx.GetStub().PutState(latestPolicyKey, updatedBytes)
	}

	// 5️⃣ Create new policy version
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	newPolicy := &LockPolicy{
		PolicyID:     orgMSP,
		OrgMSP:       orgMSP,
		FinalState:   finalState,
		DelaySeconds: delaySeconds,
		Version:      latestVersion + 1,
		CreatedAt:    timestamp,
		Active:       true,
	}

	policyJSON, err := json.Marshal(newPolicy)
	if err != nil {
		return nil, err
	}

	pKey := policyKey(orgMSP, newPolicy.Version)
	err = ctx.GetStub().PutState(pKey, policyJSON)
	if err != nil {
		return nil, err
	}

	return newPolicy, nil
}

func (s *SmartContract) GetActiveLockPolicy(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
) (*LockPolicy, error) {

	resultsIterator, err := ctx.GetStub().GetStateByRange(
		fmt.Sprintf("LOCKPOLICY_%s_v", orgMSP),
		fmt.Sprintf("LOCKPOLICY_%s_v~", orgMSP),
	)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	for resultsIterator.HasNext() {
		kv, _ := resultsIterator.Next()
		var p LockPolicy
		json.Unmarshal(kv.Value, &p)
		if p.Active {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("no active lock policy found for org %s", orgMSP)
}

func (s *SmartContract) SaveLockPolicy(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
	version string,
	policy LockPolicy,
) error {

	key, err := ctx.GetStub().CreateCompositeKey(
		"LOCKPOLICY",
		[]string{orgMSP, version},
	)
	if err != nil {
		return err
	}

	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(key, policyBytes)
}

func (s *SmartContract) GetLockPoliciesByOrg(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
) ([]LockPolicy, error) {

	resultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey(
		"LOCKPOLICY",
		[]string{orgMSP},
	)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var policies []LockPolicy

	for resultsIterator.HasNext() {
		res, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var policy LockPolicy
		if err := json.Unmarshal(res.Value, &policy); err != nil {
			return nil, err
		}

		policies = append(policies, policy)
	}

	return policies, nil
}

func (s *SmartContract) GetRecordHistory(
	ctx contractapi.TransactionContextInterface,
	recordID string,
) ([]HistoryEntry, error) {

	it, err := ctx.GetStub().GetHistoryForKey(recordID)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	var history []HistoryEntry

	for it.HasNext() {
		r, _ := it.Next()

		var record LedgerRecord
		if r.Value != nil {
			_ = json.Unmarshal(r.Value, &record)
		}

		ts := time.Unix(r.Timestamp.Seconds, int64(r.Timestamp.Nanos))

		history = append(history, HistoryEntry{
			TxID:      r.TxId,
			Timestamp: ts.Format(time.RFC3339),
			Value:     &record,
		})
	}
	return history, nil
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

func policyKey(orgMSP string, version int) string {
	return fmt.Sprintf("LOCKPOLICY_%s_v%d", orgMSP, version)
}

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
