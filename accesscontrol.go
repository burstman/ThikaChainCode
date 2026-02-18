package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// AssertClientOrgAndAttribute checks if the client belongs to the specific MSP
// and possesses the specific attribute with ONE OF the expected values.
func AssertClientOrgAndAttribute(ctx contractapi.TransactionContextInterface,
	expectedActor Actor, attrName string, expectedValues ...string) error {

	// 1. Check MSP ID
	clientMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to get client MSP ID: %v", err)
	}

	if clientMSP != expectedActor.OrgMSP {
		return fmt.Errorf("access denied: MSP ID mismatch. Expected '%s', got '%s'", expectedActor.OrgMSP, clientMSP)
	}

	// 2. Get the Attribute Value from the certificate
	val, found, err := ctx.GetClientIdentity().GetAttributeValue(attrName)
	if err != nil {
		return fmt.Errorf("error retrieving attribute '%s': %v", attrName, err)
	}

	// 3. Check if the attribute exists
	if !found {
		clientID, _ := ctx.GetClientIdentity().GetID()
		return fmt.Errorf("access denied: attribute '%s' NOT found. Caller ID: %s", attrName, clientID)
	}

	// 4. Check if the value matches ANY of the expected values
	for _, expected := range expectedValues {
		if val == expected {
			return nil // Success: Found a match!
		}
	}

	// 5. Failure: The value did not match any of the allowed options
	return fmt.Errorf("access denied: attribute '%s' value mismatch. Expected one of %v, got '%s'", attrName, expectedValues, val)
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
func (s *SmartContract) loadLockPolicy(
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

// getClientActor is a helper function to extract the Actor details from the transaction context.
func (s *SmartContract) getClientActor(ctx contractapi.TransactionContextInterface) (*Actor, error) {

	// 1. Get the Client Identity object
	clientIdentity := ctx.GetClientIdentity()
	if clientIdentity == nil {
		return nil, fmt.Errorf("failed to get client identity")
	}

	// 2. Get the Organization MSP ID (e.g., "Org1MSP")
	orgMSP, err := clientIdentity.GetMSPID()
	if err != nil {
		return nil, fmt.Errorf("failed to get client MSP ID: %v", err)
	}

	// 3. Get the Unique User ID
	// Note: This returns the Base64-encoded concatenation of the Subject DN and Issuer DN.
	userID, err := clientIdentity.GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get client ID: %v", err)
	}

	// 4. Construct and return the Actor struct
	return &Actor{
		OrgMSP: orgMSP,
		UserID: userID,
	}, nil
}
