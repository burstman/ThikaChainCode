package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// AssertClientOrgAndAttribute checks if the client has a specific attribute value.
// It returns true if the attribute matches, or false and an error if it does not.
func AssertClientOrgAndAttribute(ctx contractapi.TransactionContextInterface,
	expectedActor Actor, attrName string, expectedValue string) error {

	// DEBUG: Get the ID to see WHO is calling
	// This usually returns something like "x509::CN=creator1,OU=client..."
	clientMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to get client MSP ID: %v", err)
	}

	if clientMSP != expectedActor.OrgMSP {
		return fmt.Errorf("access denied: MSP ID mismatch. Expected '%s', got '%s'", expectedActor.OrgMSP, clientMSP)
	}

	// Now check the specific attribute
	val, found, err := ctx.GetClientIdentity().GetAttributeValue(attrName)
	if err != nil {
		return fmt.Errorf("error retrieving attribute '%s': %v", attrName, err)
	}

	// Happy Path: If found and matches, return immediately
	if found && val == expectedValue {
		return nil
	}

	// 3️⃣ Error Handling (Fetch ID only on failure for debugging)
	clientID, _ := ctx.GetClientIdentity().GetID()

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
