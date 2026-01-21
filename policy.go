package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// LockPolicy struct definition (assumed based on context)
type LockPolicy struct {
	PolicyID     string `json:"policy_id"`
	OrgMSP       string `json:"org_msp"`
	FinalState   string `json:"final_state"`
	DelaySeconds int64  `json:"delay_seconds"`
	Version      int    `json:"version"`
	CreatedAt    string `json:"created_at"` // Assuming string for timestamp
	Active       bool   `json:"active"`
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

	// 1️⃣ Permission check (ORG ADMIN ONLY)
	err := AssertClientAttribute(ctx, "role", "org_admin")
	if err != nil {
		return nil, err
	}

	// 2️⃣ Identify organization
	orgMSP, err := GetClientOrgMSPKey(ctx)
	if err != nil {
		return nil, err
	}

	// 3️⃣ Get the "Head Pointer" (Current Version)
	// This replaces the expensive loop. We look up one specific key.
	headKey, err := ctx.GetStub().CreateCompositeKey(indexPolicyHead, []string{orgMSP})
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

	// 4️⃣ Deactivate the *current* policy before creating the new one
	// We only do this if a version actually exists (version > 0)
	if currentVersion > 0 {
		// Reconstruct the key for the current active policy
		oldPolicyKey, err := ctx.GetStub().CreateCompositeKey(indexPolicy, []string{orgMSP, fmt.Sprintf("%d", currentVersion)})
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

	// 5️⃣ Create the NEW policy
	newVersion := currentVersion + 1
	timestamp, err := s.getTxTimestamp(ctx)
	if err != nil {
		return nil, err
	}

	newPolicy := &LockPolicy{
		PolicyID:     orgMSP,
		OrgMSP:       orgMSP,
		FinalState:   finalState,
		DelaySeconds: delaySeconds,
		Version:      newVersion,
		CreatedAt:    timestamp,
		Active:       true,
	}

	err = s.saveLockPolicy(ctx, newPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to save new lock policy: %v", err)
	}

	// 6️⃣ Update the Head Pointer
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
	json.Unmarshal(headBytes, &latestVersion)

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

func (s *SmartContract) GetLockPoliciesByOrg(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
) ([]LockPolicy, error) {

	resultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey(
		indexPolicy,
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

// helper function to save a LockPolicy
func (s *SmartContract) saveLockPolicy(
	ctx contractapi.TransactionContextInterface,
	policy *LockPolicy,
) error {

	// This function looks okay, provided 'version' matches the format used in CreateLockPolicy
	key, err := ctx.GetStub().CreateCompositeKey(
		indexPolicy,
		[]string{policy.OrgMSP, strconv.Itoa(policy.Version)},
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
