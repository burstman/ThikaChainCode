package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// UserProfile defines the metadata we store about a user
type UserProfile struct {
	DocType string `json:"docType"` // Always "UserProfile"
	UserID  string `json:"userId"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
}

// RegisterUser creates a new user profile in the world state
func (s *SmartContract) RegisterUser(ctx contractapi.TransactionContextInterface, userId string, name string, email string, role string) error {

	// 1. Check permissions (Only Admins should be able to register new users)
	actor, err := s.getClientActor(ctx)
	if err != nil {
		return err
	}
	err = AssertClientOrgAndAttribute(ctx, *actor, "role", "org_admin")

	// 2. Check if user already exists
	userKey := "USER_" + userId
	existingBytes, err := ctx.GetStub().GetState(userKey)
	if err != nil {
		return fmt.Errorf("failed to read from world state: %v", err)
	}
	if existingBytes != nil {
		return fmt.Errorf("user %s already exists", userId)
	}

	// 3. Create the User object
	user := UserProfile{
		DocType: "UserProfile",
		UserID:  userId,
		Name:    name,
		Email:   email,
		Role:    role,
	}

	userJSON, err := json.Marshal(user)
	if err != nil {
		return err
	}

	// 4. Save to state
	return ctx.GetStub().PutState(userKey, userJSON)
}

// GetAllUsers returns all registered users
func (s *SmartContract) GetAllUsers(ctx contractapi.TransactionContextInterface) ([]*UserProfile, error) {
	// Range query for keys starting with "USER_"

	actor, err := s.getClientActor(ctx)
	if err != nil {
		return nil, err
	}
	err = AssertClientOrgAndAttribute(ctx, *actor, "role", "org_admin")

	resultsIterator, err := ctx.GetStub().GetStateByRange("USER_", "USER_\uffff")
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var users []*UserProfile

	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var user UserProfile
		err = json.Unmarshal(queryResponse.Value, &user)
		if err != nil {
			return nil, err
		}
		users = append(users, &user)
	}

	return users, nil
}
