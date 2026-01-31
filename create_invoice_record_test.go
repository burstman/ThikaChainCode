package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks Setup ---

// MockTransactionContext implements contractapi.TransactionContextInterface
type MockTransactionContext struct {
	contractapi.TransactionContextInterface
	mock.Mock
}

func (m *MockTransactionContext) GetStub() shim.ChaincodeStubInterface {
	args := m.Called()
	return args.Get(0).(shim.ChaincodeStubInterface)
}

func (m *MockTransactionContext) GetClientIdentity() cid.ClientIdentity {
	args := m.Called()
	return args.Get(0).(cid.ClientIdentity)
}

// MockChaincodeStub implements shim.ChaincodeStubInterface
type MockChaincodeStub struct {
	shim.ChaincodeStubInterface
	mock.Mock
}

func (m *MockChaincodeStub) PutState(key string, value []byte) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockChaincodeStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*timestamp.Timestamp), args.Error(1)
}

// MockClientIdentity implements cid.ClientIdentity
type MockClientIdentity struct {
	cid.ClientIdentity
	mock.Mock
}

func (m *MockClientIdentity) GetID() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockClientIdentity) GetMSPID() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func (m *MockClientIdentity) GetAttributeValue(attrName string) (string, bool, error) {
	args := m.Called(attrName)
	return args.String(0), args.Bool(1), args.Error(2)
}

// --- Tests ---

func TestCreateInvoiceRecord(t *testing.T) {
	// Define constants for the test
	const (
		validRecordID = "INV-001"
		validFilename = "invoice.xml"
		validXML      = "<invoice>data</invoice>"
		validBase64   = "PGludm9pY2U+ZGF0YTwvaW52b2ljZT4=" // Base64 of validXML
		mockClientID  = "x509::CN=User1,OU=Client::CN=FabricCA,OU=Fabric::US"
		mockMSP       = "Org1MSP"
	)

	// Helper to create a valid timestamp
	txTime, _ := time.Parse(time.RFC3339, "2023-10-01T12:00:00Z")
	pbTimestamp := &timestamp.Timestamp{Seconds: txTime.Unix(), Nanos: 0}

	tests := []struct {
		name           string
		recordID       string
		filename       string
		xmlBase64      string
		setupMocks     func(*MockTransactionContext, *MockChaincodeStub, *MockClientIdentity)
		expectedError  string
		expectedRecord *LedgerRecord
	}{
		{
			name:      "Success: Valid invoice creation",
			recordID:  validRecordID,
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// 1. Identity Mocks (for getClientActor and AssertClientOrgAndAttribute)
				cid.On("GetID").Return(mockClientID, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				// Assuming AssertClientOrgAndAttribute checks the "role" attribute
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil)

				ctx.On("GetClientIdentity").Return(cid)

				// 2. Stub Mocks (Timestamp and PutState)
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
				stub.On("PutState", validRecordID, mock.Anything).Run(func(args mock.Arguments) {
					// Optional: Validate the JSON being saved
					jsonBytes := args.Get(1).([]byte)
					var rec LedgerRecord
					err := json.Unmarshal(jsonBytes, &rec)
					assert.NoError(t, err)
					assert.Equal(t, validRecordID, rec.RecordID)
					assert.Equal(t, "CREATED", rec.Status.Code)
				}).Return(nil)

				ctx.On("GetStub").Return(stub)
			},
			expectedError: "",
		},
		{
			name:     "Error: File size too large",
			recordID: validRecordID,
			filename: validFilename,
			// Create a string larger than MaxBase64Size (assuming MaxBase64Size is accessible or we mock the check logic)
			// Note: Since MaxBase64Size is a constant in your package, ensure this string exceeds it.
			// Here we simulate a large string.
			xmlBase64: strings.Repeat("A", MaxBase64Size+1),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// No mocks needed as it fails before context usage
			},
			expectedError: fmt.Sprintf("invoice file too large: %d bytes", MaxBase64Size+1),
		},
		{
			name:      "Error: Empty payload",
			recordID:  validRecordID,
			filename:  validFilename,
			xmlBase64: "",
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// No mocks needed as it fails before context usage
			},
			expectedError: "invoice content cannot be empty",
		},
		{
			name:      "Error: Client is not an admin",
			recordID:  validRecordID,
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				cid.On("GetID").Return(mockClientID, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				// Return a role that is NOT org_admin
				cid.On("GetAttributeValue", "role").Return("member", true, nil)

				ctx.On("GetClientIdentity").Return(cid)
			},
			expectedError: "client is not an org_admin", // Assuming this is the error from AssertClientOrgAndAttribute
		},
		{
			name:      "Error: Failed to get timestamp",
			recordID:  validRecordID,
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				cid.On("GetID").Return(mockClientID, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil)
				ctx.On("GetClientIdentity").Return(cid)

				stub.On("GetTxTimestamp").Return(nil, errors.New("timestamp error"))
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "failed to get transaction timestamp",
		},
		{
			name:      "Error: PutState fails",
			recordID:  validRecordID,
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				cid.On("GetID").Return(mockClientID, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil)
				ctx.On("GetClientIdentity").Return(cid)

				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
				stub.On("PutState", validRecordID, mock.Anything).Return(errors.New("ledger error"))
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "ledger error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize Mocks
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)
			mockCID := new(MockClientIdentity)

			// Setup expectations
			tt.setupMocks(mockCtx, mockStub, mockCID)

			// Initialize Contract
			contract := &SmartContract{}

			// Execute
			result, err := contract.CreateInvoiceRecord(mockCtx, tt.recordID, tt.filename, tt.xmlBase64)

			// Assertions
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.recordID, result.RecordID)
				data, ok := result.BusinessData.(InvoiceData)
				assert.True(t, ok, "BusinessData should be of type InvoiceData")
				assert.Equal(t, tt.filename, data.Filename)
				assert.Equal(t, tt.xmlBase64, data.XMLContent)
			}

			// Verify that all expectations were met
			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
			mockCID.AssertExpectations(t)
		})
	}
}
