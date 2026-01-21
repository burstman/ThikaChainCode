package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
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
	orgMSP, err := GetClientOrgMSP(ctx)
	if err != nil {
		return nil, err
	}

	// 3️⃣ Find latest policy version
	latestVersion := 0
	latestPolicyKey := ""

	// Use PartialCompositeKey to find the latest version

	resultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey("LOCKPOLICY", []string{orgMSP})
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

	// Create the new key using CompositeKey
	newPolicy.Version = latestVersion + 1
	pKey, err := ctx.GetStub().CreateCompositeKey("LOCKPOLICY", []string{orgMSP, fmt.Sprintf("%d", newPolicy.Version)})
	if err != nil {
		return nil, err
	}

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
