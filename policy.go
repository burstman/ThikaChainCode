package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// LockPolicy struct definition (assumed based on context)
type LockPolicy struct {
	DocType      string `json:"docType"` // e.g., "lockPolicy"
	PolicyID     string `json:"policy_id"`
	FinalState   string `json:"final_state"`
	DelaySeconds int64  `json:"delay_seconds"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"created_at"` // Assuming string for timestamp
	Active       bool   `json:"active"`
	CreatedBy    Actor  `json:"createdBy"`
}

const (
	indexPolicy     = "LOCKPOLICY"
	indexPolicyHead = "LOCKPOLICY_HEAD"
)

func (s *SmartContract) CreateLockPolicy(
	ctx contractapi.TransactionContextInterface,
	finalState string,
	delaySeconds int64,
) (*LockPolicy, error) {

	actor, err := s.getClientActor(ctx)

	//  Permission check (ORG ADMIN ONLY)
	err = AssertClientOrgAndAttribute(ctx, *actor, "role", "org_admin")
	if err != nil {
		return nil, err
	}

	// Get the "Head Pointer" (Current Version)
	// This replaces the expensive loop. We look up one specific key.
	headKey, err := ctx.GetStub().CreateCompositeKey(indexPolicyHead, []string{actor.OrgMSP})
	if err != nil {
		return nil, fmt.Errorf("failed to create head key: %v", err)
	}

	headBytes, err := ctx.GetStub().GetState(headKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy head: %v", err)
	}

	var currentVersion int
	if headBytes != nil {
		// If the key exists, unmarshal the integer.
		// If it doesn't exist (nil), currentVersion remains 0 (default int value).
		err = json.Unmarshal(headBytes, &currentVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal policy head version: %v", err)
		}
	}

	// Deactivate the *current* policy before creating the new one
	// We only do this if a version actually exists (version > 0)
	if currentVersion > 0 {
		// Reconstruct the key for the current active policy
		oldPolicyKey, err := ctx.GetStub().CreateCompositeKey(indexPolicy, []string{actor.OrgMSP, fmt.Sprintf("%d", currentVersion)})
		if err != nil {
			return nil, err
		}

		oldPolicyBytes, err := ctx.GetStub().GetState(oldPolicyKey)
		if err != nil {
			return nil, err
		}

		if oldPolicyBytes != nil {
			var oldPolicy LockPolicy
			if err := json.Unmarshal(oldPolicyBytes, &oldPolicy); err != nil {
				return nil, fmt.Errorf("failed to unmarshal old policy: %v", err)
			}

			oldPolicy.Active = false

			// Use the helper to save the old policy too
			if err := s.saveLockPolicy(ctx, &oldPolicy); err != nil {
				return nil, fmt.Errorf("failed to update old policy status: %v", err)
			}
		}
	}

	// Create the NEW policy
	newVersion := currentVersion + 1
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	newPolicy := &LockPolicy{
		DocType:      "LockPolicy",
		PolicyID:     actor.OrgMSP,
		FinalState:   finalState,
		DelaySeconds: delaySeconds,
		Version:      newVersion,
		CreatedAt:    timestamp,
		CreatedBy:    *actor,
		Active:       true,
	}

	err = s.saveLockPolicy(ctx, newPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to save new lock policy: %v", err)
	}

	// Update the Head Pointer
	// Save the new version number back to the head key so the next transaction knows where to start.
	newHeadBytes, _ := json.Marshal(newVersion)
	err = ctx.GetStub().PutState(headKey, newHeadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update policy head: %v", err)
	}

	return newPolicy, nil
}

//Performance: It always takes the same amount of time to run, whether you have 1 policy or 1,000,000 policies.
//Concurrency: It utilizes Fabric's MVCC (Multi-Version Concurrency Control) effectively.
//If two admins try to update the policy at the exact same time, they will both read the same "Head Version" (e.g., 5).
//Both will try to write "6" to the Head Key. The first one will succeed; the second one will fail with an MVCC Read Conflict, preventing data corruption.

func (s *SmartContract) GetActiveLockPolicy(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
) (*LockPolicy, error) {

	// OPTIMIZATION: Use the Head Pointer instead of iterating history.
	// 1. Get the Head Version
	headKey, err := ctx.GetStub().CreateCompositeKey(indexPolicyHead, []string{orgMSP})
	if err != nil {
		return nil, err
	}
	headBytes, err := ctx.GetStub().GetState(headKey)
	if err != nil {
		return nil, err
	}
	if headBytes == nil {
		return nil, fmt.Errorf("no active lock policy found for org %s", orgMSP)
	}

	var latestVersion int
	if err := json.Unmarshal(headBytes, &latestVersion); err != nil {
		return nil, fmt.Errorf("failed to unmarshal policy head version for org %s: %v", orgMSP, err)
	}

	// 2. Get the specific policy directly (O(1) lookup)
	policyKey, err := ctx.GetStub().CreateCompositeKey(indexPolicy, []string{orgMSP, strconv.Itoa(latestVersion)})
	if err != nil {
		return nil, err
	}

	policyBytes, err := ctx.GetStub().GetState(policyKey)
	if err != nil {
		return nil, err
	}
	if policyBytes == nil {
		return nil, fmt.Errorf("policy data missing for version %d", latestVersion)
	}

	var p LockPolicy
	err = json.Unmarshal(policyBytes, &p)
	if err != nil {
		return nil, err
	}

	// Double check active status (though head usually implies active in this logic)
	if !p.Active {
		return nil, fmt.Errorf("latest policy is not active")
	}

	return &p, nil
}

// GetLockPoliciesByOrg finds all historical policy versions for an organization within a given date range.
// This uses a CouchDB rich query for efficient, database-side filtering.
func (s *SmartContract) GetLockPoliciesByOrg(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
	startStr string, // e.g., "2024-01-01T00:00:00Z"
	endStr string, // e.g., "2024-12-31T23:59:59Z"
) ([]LockPolicy, error) {

	// 1. Validate Input Dates
	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, fmt.Errorf("invalid start time format (use RFC3339): %v", err)
	}
	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, fmt.Errorf("invalid end time format (use RFC3339): %v", err)
	}
	if startTime.After(endTime) {
		return nil, fmt.Errorf("start time must be before end time")
	}

	// 2. Construct CouchDB Query String
	// This query finds documents that match the docType, policy_id, and fall within the date range.
	queryString := fmt.Sprintf(`{
		"selector": {
			"docType": "LockPolicy",
			"policy_id": "%s",
			"created_at": {
				"$gte": "%s",
				"$lte": "%s"
			}
		},
		"sort": [{"created_at": "asc"}]
	}`, orgMSP, startStr, endStr)

	// 3. Execute Query
	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer resultsIterator.Close()

	// 4. Iterate and Parse Results
	var policies []LockPolicy
	for resultsIterator.HasNext() {
		res, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var policy LockPolicy
		if err := json.Unmarshal(res.Value, &policy); err != nil {
			// Log this error but continue? For now, we fail the entire query.
			return nil, fmt.Errorf("failed to unmarshal policy: %w", err)
		}
		policies = append(policies, policy)
	}

	return policies, nil
}

// helper function to save a LockPolicy
func (s *SmartContract) saveLockPolicy(
	ctx contractapi.TransactionContextInterface,
	policy *LockPolicy,
) error {

	// This function looks okay, provided 'version' matches the format used in CreateLockPolicy
	key, err := ctx.GetStub().CreateCompositeKey(
		indexPolicy,
		[]string{policy.CreatedBy.OrgMSP, strconv.Itoa(policy.Version)},
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

func (s *SmartContract) enforceLockPolicy(
	ctx contractapi.TransactionContextInterface,
	record *LedgerRecord,
) error {

	// 1. Optimization: If already locked, stop here.
	if record.Locked {
		return nil
	}

	// 2. Load the SPECIFIC policy version this record is tied to.
	// We do NOT check if policy.Active is true. Historical policies must still be enforced.
	policy, err := s.loadLockPolicy(
		ctx,
		record.Actor.OrgMSP,
		record.LockPolicyID,
		record.PolicyVersion,
	)
	if err != nil {
		return err
	}

	// 3. Check if the record is in the state that triggers the lock (e.g. "PENDING_APPROVAL")
	if record.Status.Code != policy.FinalState {
		return nil
	}

	// 4. Calculate Time
	stateTime, err := time.Parse(time.RFC3339, record.Status.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse record time: %v", err)
	}

	txTimeStr, err := s.getTxTimestamp(ctx)
	if err != nil {
		return err
	}
	now, err := time.Parse(time.RFC3339, txTimeStr)
	if err != nil {
		return err
	}

	lockDelay := time.Duration(policy.DelaySeconds) * time.Second

	// 5. Compare Time (Fixed syntax error here)
	if now.Sub(stateTime) >= lockDelay {
		record.Locked = true
		record.LockedAt = txTimeStr

		// Save the updated state
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return ctx.GetStub().PutState(record.RecordID, data)
	}

	return nil
}
