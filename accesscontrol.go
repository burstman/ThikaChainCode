package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// CheckPermissionClientOrgID checks if the client has a specific attribute value.
// It returns true if the attribute matches, or false and an error if it does not.
func CheckPermissionClientOrgID(ctx contractapi.TransactionContextInterface, attrName string, attrValue string) (bool, error) {
	// AssertAttributeValue returns nil if the attribute matches the value
	err := ctx.GetClientIdentity().AssertAttributeValue(attrName, attrValue)

	if err != nil {
		// Return false and a formatted error message
		return false, fmt.Errorf("access denied: user does not have attribute '%s' with value '%s'", attrName, attrValue)
	}

	// Return true if the check passed
	return true, nil
}

func GetClientIdentity(ctx contractapi.TransactionContextInterface) string {
	id, _ := ctx.GetClientIdentity().GetID()
	return id
}

func GetClientOrgMSP(ctx contractapi.TransactionContextInterface) string {
	orgMSP, _ := ctx.GetClientIdentity().GetMSPID()
	return orgMSP

}

func (s *SmartContract) LoadLockPolicy(
	ctx contractapi.TransactionContextInterface,
	orgMSP string,
	policyID string,
	version int,
) (*LockPolicy, error) {

	key := fmt.Sprintf(
		"LOCKPOLICY_%s_%s_v%d",
		orgMSP,
		policyID,
		version,
	)

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("lock policy not found")
	}

	var policy LockPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, err
	}

	return &policy, nil
}
