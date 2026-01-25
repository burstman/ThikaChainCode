package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// AssertClientAttribute checks if the client has a specific attribute value.
// It returns true if the attribute matches, or false and an error if it does not.
func AssertClientAttribute(ctx contractapi.TransactionContextInterface, attrName string, expectedValue string) error {

	// DEBUG: Get the ID to see WHO is calling
	// This usually returns something like "x509::CN=creator1,OU=client..."
	clientID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return fmt.Errorf("failed to get client ID: %v", err)
	}

	val, found, err := ctx.GetClientIdentity().GetAttributeValue(attrName)
	if err != nil {
		return fmt.Errorf("error retrieving attribute '%s': %v", attrName, err)
	}

	if !found {
		// Include the Client ID in the error message for debugging
		return fmt.Errorf("access denied: attribute '%s' NOT found. Caller ID: %s", attrName, clientID)
	}

	if val != expectedValue {
		return fmt.Errorf("access denied: attribute '%s' value mismatch. Expected '%s', got '%s'", attrName, expectedValue, val)
	}

	return nil
}

func GetClientIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	id, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("failed to get client identity: %v", err)
	}
	return id, nil
}

func GetClientOrgMSPKey(ctx contractapi.TransactionContextInterface) (string, error) {
	orgMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to get client MSP ID: %v", err)
	}
	return orgMSP, nil
}

// Helper to load a specific historical policy version
func (s *SmartContract) LoadLockPolicy(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
	policyID string, // Likely redundant if ID == OrgMSP
	version int,
) (*LockPolicy, error) {

	// Construct key for specific version: LOCKPOLICY_Org1_1
	key, err := ctx.GetStub().CreateCompositeKey(indexPolicy, []string{orgMSP, strconv.Itoa(version)})
	if err != nil {
		return nil, err
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("policy version %d not found", version)
	}

	var policy LockPolicy
	err = json.Unmarshal(data, &policy)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}
