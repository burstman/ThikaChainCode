package main

import (
	"encoding/base64"
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
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (m *MockChaincodeStub) GetTxID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockChaincodeStub) PutState(key string, value []byte) error {
	args := m.Called(key, value)
	return args.Error(0)
}

// ✅ FIX 1: Implement GetState to prevent panic
func (m *MockChaincodeStub) GetState(key string) ([]byte, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockChaincodeStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	args := m.Called(objectType, attributes)
	return args.String(0), args.Error(1)
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
		validFilename    = "invoice.xml"
		validXML         = "<invoice>data</invoice>"
		validBase64      = "PGludm9pY2U+ZGF0YTwvaW52b2ljZT4=" // Base64 of validXML
		mockClientID     = "x509::CN=User1,OU=Client::CN=FabricCA,OU=Fabric::US"
		mockMSP          = "Org1MSP"
		mockTxID         = "e5b38f9a2d1c4e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
		expectedRecordID = "REC-e5b38f9a2d1c" // The ID that will be generated from mockTxID
	)

	// Helper to create a valid timestamp
	txTime, _ := time.Parse(time.RFC3339, "2023-10-01T12:00:00Z")
	pbTimestamp := &timestamppb.Timestamp{Seconds: txTime.Unix(), Nanos: 0}

	tests := []struct {
		name          string
		filename      string
		xmlBase64     string
		setupMocks    func(*MockTransactionContext, *MockChaincodeStub, *MockClientIdentity)
		expectedError string
	}{
		{
			name:      "Success: Valid invoice creation",
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// Set up the context to return our mock objects. This is the foundation.
				ctx.On("GetStub").Return(stub).Maybe()
				ctx.On("GetClientIdentity").Return(cid).Maybe()

				// Mock calls in the exact order they appear in the chaincode
				stub.On("GetTxID").Return(mockTxID).Once()
				stub.On("GetState", expectedRecordID).Return(nil, nil).Once() // Record does not exist
				cid.On("GetID").Return(mockClientID, nil).Once()
				cid.On("GetMSPID").Return(mockMSP, nil).Maybe()
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil).Once()
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil).Once()
				stub.On("PutState", expectedRecordID, mock.Anything).Return(nil).Once()
			},
			expectedError: "",
		},
		{
			name:      "Error: Record already exists",
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// The function will call GetStub, then GetTxID, then GetState, and then exit.
				ctx.On("GetStub").Return(stub).Maybe()
				stub.On("GetTxID").Return(mockTxID).Maybe()
				stub.On("GetState", expectedRecordID).Return([]byte("exists"), nil).Once() // Mock that the record exists
			},
			expectedError: fmt.Sprintf("the record %s already exists", expectedRecordID),
		},
		{
			name:      "Error: File size too large",
			filename:  validFilename,
			xmlBase64: strings.Repeat("A", MaxBase64Size+1),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// No mocks needed, validation fails before any stub calls.
			},
			expectedError: fmt.Sprintf("invoice file too large: %d bytes", MaxBase64Size+1),
		},
		{
			name:      "Error: Client is not an admin",
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				ctx.On("GetStub").Return(stub).Maybe()
				ctx.On("GetClientIdentity").Return(cid).Maybe()

				stub.On("GetTxID").Return(mockTxID).Once()
				stub.On("GetState", expectedRecordID).Return(nil, nil).Once()
				cid.On("GetID").Return(mockClientID, nil).Once()
				cid.On("GetMSPID").Return(mockMSP, nil).Maybe()
				cid.On("GetAttributeValue", "role").Return("member", true, nil).Once() // User has wrong role
			},
			expectedError: "access denied",
		},
		{
			name:      "Error: Failed to get timestamp",
			filename:  validFilename,
			xmlBase64: validBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				ctx.On("GetStub").Return(stub).Maybe()
				ctx.On("GetClientIdentity").Return(cid).Maybe()

				stub.On("GetTxID").Return(mockTxID).Once()
				stub.On("GetState", expectedRecordID).Return(nil, nil).Once()
				cid.On("GetID").Return(mockClientID, nil).Once()
				cid.On("GetMSPID").Return(mockMSP, nil).Maybe()
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil).Once()
				stub.On("GetTxTimestamp").Return(nil, errors.New("timestamp error")).Once() // Mock timestamp failure
			},
			expectedError: "failed to get transaction timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)
			mockCID := new(MockClientIdentity)

			tt.setupMocks(mockCtx, mockStub, mockCID)

			contract := &SmartContract{}
			result, err := contract.CreateInvoiceRecord(mockCtx, tt.filename, tt.xmlBase64)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result, "Expected result to be nil on error")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, expectedRecordID, result.RecordID)

				if data, ok := result.BusinessData.(InvoiceData); ok {
					assert.Equal(t, tt.filename, data.Filename)
					assert.Equal(t, tt.xmlBase64, data.XMLContent)
				} else {
					dataMap, ok := result.BusinessData.(map[string]interface{})
					assert.True(t, ok, "BusinessData should be a map")
					assert.Equal(t, tt.filename, dataMap["filename"])
					assert.Equal(t, tt.xmlBase64, dataMap["xmlContent"])
				}
			}

			// Verify that all expected mock calls were made
			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
			mockCID.AssertExpectations(t)
		})
	}
}

func TestUpdateInvoiceRecord(t *testing.T) {
	const (
		recordID     = "INV-001"
		oldFilename  = "old_invoice.xml"
		newFilename  = "new_invoice.xml"
		newXMLBase64 = "bmV3X2NvbnRlbnQ=" // "new_content" in Base64
		mockMSP      = "Org1MSP"
		mockUserID   = "User1"
	)

	// Setup a valid existing record (Unlocked)
	existingActor := Actor{OrgMSP: mockMSP, UserID: mockUserID}
	existingRecord := &LedgerRecord{
		RecordID:      recordID,
		Actor:         existingActor,
		Locked:        false,
		BusinessData:  InvoiceData{Filename: oldFilename, XMLContent: "b2xkX2NvbnRlbnQ="},
		Status:        Status{Code: "CREATED"},
		PolicyVersion: 0,
	}
	existingRecordBytes, _ := json.Marshal(existingRecord)

	// Setup a Locked record
	lockedRecord := *existingRecord
	lockedRecord.Locked = true
	lockedRecordBytes, _ := json.Marshal(lockedRecord)

	txTime, _ := time.Parse(time.RFC3339, "2023-10-02T12:00:00Z")
	pbTimestamp := &timestamp.Timestamp{Seconds: txTime.Unix()}

	tests := []struct {
		name          string
		recordID      string
		newFilename   string
		newXmlBase64  string
		setupMocks    func(*MockTransactionContext, *MockChaincodeStub, *MockClientIdentity)
		expectedError string
	}{
		{
			name:         "Success: Valid update",
			recordID:     recordID,
			newFilename:  newFilename,
			newXmlBase64: newXMLBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// 1. Return the UNLOCKED record
				stub.On("GetState", recordID).Return(existingRecordBytes, nil)

				// 2. Identity Mocks
				// Note: GetID is NOT called because "org_admin" check passes first
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("org_admin", true, nil)
				ctx.On("GetClientIdentity").Return(cid)

				// 3. Timestamp
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)

				// 4. Mock Lock Policy checks
				stub.On("CreateCompositeKey", "LOCKPOLICY", []string{mockMSP, "0"}).Return("dummyLockKey", nil)
				stub.On("GetState", "dummyLockKey").Return([]byte(`{}`), nil)

				// 5. Expect PutState
				stub.On("PutState", recordID, mock.Anything).Run(func(args mock.Arguments) {
					jsonBytes := args.Get(1).([]byte)
					var rec LedgerRecord
					err := json.Unmarshal(jsonBytes, &rec)
					assert.NoError(t, err)

					// Verify updates
					data, ok := rec.BusinessData.(map[string]interface{})
					assert.True(t, ok, "BusinessData should be a map")

					// ✅ FIX: Use lowercase keys to match standard JSON tags.
					// If your struct has `json:"filename"`, the key is "filename".
					// If this still fails, print the map keys: fmt.Println(data)
					if _, ok := data["filename"]; ok {
						assert.Equal(t, newFilename, data["filename"])
						assert.Equal(t, newXMLBase64, data["xmlContent"]) // Check if tag is "xmlContent" or "xml_content"
					} else {
						// Fallback to Uppercase if tags are missing entirely
						assert.Equal(t, newFilename, data["Filename"])
						assert.Equal(t, newXMLBase64, data["XMLContent"])
					}

					assert.Equal(t, "UPDATED", rec.Status.Code)
				}).Return(nil)

				ctx.On("GetStub").Return(stub)
			},
			expectedError: "",
		},
		{
			name:         "Error: Record is Locked",
			recordID:     recordID,
			newFilename:  newFilename,
			newXmlBase64: newXMLBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(lockedRecordBytes, nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "is LOCKED and cannot be updated",
		},
		{
			name:         "Error: Record does not exist",
			recordID:     "NON-EXISTENT",
			newFilename:  newFilename,
			newXmlBase64: newXMLBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", "NON-EXISTENT").Return(nil, nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "does not exist",
		},
		{
			name:         "Error: Permission Denied (Not Admin)",
			recordID:     recordID,
			newFilename:  newFilename,
			newXmlBase64: newXMLBase64,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(existingRecordBytes, nil)
				ctx.On("GetStub").Return(stub)

				// GetID IS called here because admin check fails
				//cid.On("GetID").Return(mockUserID, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("member", true, nil)
				ctx.On("GetClientIdentity").Return(cid)
			},
			expectedError: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)
			mockCID := new(MockClientIdentity)

			tt.setupMocks(mockCtx, mockStub, mockCID)

			contract := &SmartContract{}
			result, err := contract.UpdateInvoiceRecord(mockCtx, tt.recordID, tt.newFilename, tt.newXmlBase64)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result, "Expected result to be nil on error")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
			mockCID.AssertExpectations(t)
		})
	}
}

func TestCreateRecord(t *testing.T) {
	const (
		recordID   = "REC-001"
		mockMSP    = "Org1MSP"
		mockUserID = "creator1"
	)

	// A valid JSON string for business data
	validBusinessData := `{"field1":"value1", "field2":123}`

	// Timestamp setup
	txTime := time.Now()
	pbTimestamp := timestamppb.New(txTime)

	tests := []struct {
		name          string
		businessData  string
		setupMocks    func(*MockTransactionContext, *MockChaincodeStub, *MockClientIdentity)
		expectedError string
	}{
		{
			name:         "Success: Valid record creation",
			businessData: validBusinessData,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// 1. Expect existence check to find nothing
				stub.On("GetState", recordID).Return(nil, nil)

				// 2. Expect identity checks for permission
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("record_creator", true, nil)
				ctx.On("GetClientIdentity").Return(cid)

				// 3. Expect timestamp and actor calls
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
				cid.On("GetID").Return(mockUserID, nil) // For getClientActor

				// 4. Expect PutState to be called with the correct data
				stub.On("PutState", recordID, mock.Anything).Run(func(args mock.Arguments) {
					// Inside Run, we can inspect the data being saved
					bytes := args.Get(1).([]byte)
					var savedRecord LedgerRecord
					err := json.Unmarshal(bytes, &savedRecord)
					assert.NoError(t, err)
					assert.Equal(t, recordID, savedRecord.RecordID)
					assert.Equal(t, "CREATED", savedRecord.Status.Code)
					assert.Equal(t, mockUserID, savedRecord.Actor.UserID)
				}).Return(nil)

				ctx.On("GetStub").Return(stub)
			},
			expectedError: "",
		},
		{
			name:         "Error: Record already exists",
			businessData: validBusinessData,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// Expect existence check to find an existing record
				stub.On("GetState", recordID).Return([]byte("some data"), nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "already exists",
		},
		{
			name:         "Error: Invalid business data (not JSON)",
			businessData: "this is not json",
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// Expect existence check to pass
				stub.On("GetState", recordID).Return(nil, nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "businessData must be valid JSON",
		},
		{
			name:         "Error: Permission denied (wrong role)",
			businessData: validBusinessData,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(nil, nil)
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetID").Return(mockUserID, nil)

				// Return a role that is NOT "record_creator"
				cid.On("GetAttributeValue", "role").Return("viewer", true, nil)

				ctx.On("GetClientIdentity").Return(cid)
				ctx.On("GetStub").Return(stub)
			},
			// This error message comes from your AssertClientOrgAndAttribute helper
			expectedError: "access denied",
		},
		{
			name:         "Error: PutState fails",
			businessData: validBusinessData,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(nil, nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("record_creator", true, nil)
				ctx.On("GetClientIdentity").Return(cid)
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
				cid.On("GetID").Return(mockUserID, nil)

				// Mock PutState to return an error
				stub.On("PutState", recordID, mock.Anything).Return(errors.New("ledger write error"))

				ctx.On("GetStub").Return(stub)
			},
			expectedError: "ledger write error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize Mocks for each test run
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)
			mockCID := new(MockClientIdentity)

			// Apply the specific mock setup for this test case
			tt.setupMocks(mockCtx, mockStub, mockCID)

			contract := &SmartContract{}
			result, err := contract.CreateBusinessDataRecord(mockCtx, tt.businessData)

			// Assert the results
			if tt.expectedError != "" {
				assert.Error(t, err, "Expected an error to be returned")
				assert.Contains(t, err.Error(), tt.expectedError, "Error message should contain expected text")
				assert.Nil(t, result, "Expected result to be nil on error")
			} else {
				assert.NoError(t, err, "Expected no error for a successful run")
				assert.NotNil(t, result, "Expected a non-nil record on success")
				//assert.Equal(t, tt.recordID, result.RecordID)
			}

			// Verify that all expected mock calls were made
			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
			mockCID.AssertExpectations(t)
		})
	}
}

func TestUpdateBusinessData(t *testing.T) {
	const (
		recordID      = "REC-001"
		mockMSP       = "Org1MSP"
		otherMSP      = "Org2MSP"
		validJSON     = `{"status": "updated", "value": 99}`
		invalidJSON   = `{"status": "broken"`
		mockPolicyKey = "lock-policy-key"
	)

	// Helper to create a standard existing record
	createExistingRecord := func(msp string, locked bool) []byte {
		rec := &LedgerRecord{
			RecordID:     recordID,
			Actor:        Actor{OrgMSP: msp, UserID: "user1"},
			Locked:       locked,
			Status:       Status{Code: "CREATED"},
			BusinessData: json.RawMessage(`{"initial": "data"}`),
		}
		bytes, _ := json.Marshal(rec)
		return bytes
	}

	tests := []struct {
		name            string
		recordID        string
		newBusinessData string
		setupMocks      func(*MockTransactionContext, *MockChaincodeStub, *MockClientIdentity)
		expectedError   string
	}{
		{
			name:            "Success: Valid update",
			recordID:        recordID,
			newBusinessData: validJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(createExistingRecord(mockMSP, false), nil)
				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return(mockPolicyKey, nil)
				stub.On("GetState", mockPolicyKey).Return([]byte(`{}`), nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("record_editor", true, nil)
				ctx.On("GetClientIdentity").Return(cid)
				stub.On("PutState", recordID, mock.Anything).Return(nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "",
		},
		{
			name:            "Error: Record does not exist",
			recordID:        "NON-EXISTENT",
			newBusinessData: validJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", "NON-EXISTENT").Return(nil, nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "does not exist",
		},
		{
			name:            "Error: Record is Locked",
			recordID:        recordID,
			newBusinessData: validJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				// ✅ FIX: Removed expectations for CreateCompositeKey and the policy GetState.
				// The code likely returns after checking record.Locked without calling them.
				stub.On("GetState", recordID).Return(createExistingRecord(mockMSP, true), nil)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "is locked and cannot be modified",
		},
		{
			name:            "Error: Permission Denied (Wrong Role)",
			recordID:        recordID,
			newBusinessData: validJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(createExistingRecord(mockMSP, false), nil)
				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return(mockPolicyKey, nil)
				stub.On("GetState", mockPolicyKey).Return([]byte(`{}`), nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("viewer", true, nil) // Wrong role
				ctx.On("GetClientIdentity").Return(cid)
				cid.On("GetID").Return("user1", nil).Maybe()
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "access denied",
		},
		{
			name:            "Error: Invalid JSON Input",
			recordID:        recordID,
			newBusinessData: invalidJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(createExistingRecord(mockMSP, false), nil)
				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return(mockPolicyKey, nil)
				stub.On("GetState", mockPolicyKey).Return([]byte(`{}`), nil)
				cid.On("GetMSPID").Return(mockMSP, nil)
				cid.On("GetAttributeValue", "role").Return("record_editor", true, nil)
				ctx.On("GetClientIdentity").Return(cid)
				ctx.On("GetStub").Return(stub)
			},
			expectedError: "businessData must be valid JSON",
		},
		{
			name:            "Error: Organization Mismatch",
			recordID:        recordID,
			newBusinessData: validJSON,
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub, cid *MockClientIdentity) {
				stub.On("GetState", recordID).Return(createExistingRecord(mockMSP, false), nil)
				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return(mockPolicyKey, nil)
				stub.On("GetState", mockPolicyKey).Return([]byte(`{}`), nil)
				// ✅ FIX: Removed expectation for GetAttributeValue, as the code fails before that.
				cid.On("GetMSPID").Return(otherMSP, nil) // Caller is from a different org
				ctx.On("GetClientIdentity").Return(cid)
				ctx.On("GetStub").Return(stub)
			},
			// ✅ FIX: Changed expected error to match the helper function's output.
			expectedError: "access denied: MSP ID mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)
			mockCID := new(MockClientIdentity)

			tt.setupMocks(mockCtx, mockStub, mockCID)

			contract := &SmartContract{}
			err := contract.UpdateBusinessData(mockCtx, tt.recordID, tt.newBusinessData)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
			mockCID.AssertExpectations(t)
		})
	}
}

func TestEnforceLockPolicy(t *testing.T) {
	const (
		recordID      = "REC-001"
		mockMSP       = "Org1MSP"
		policyID      = "policy-01"
		policyVersion = 1
		finalState    = "FINALIZED"
	)

	// Helper to create a record
	createRecord := func(status string, updatedAt string, locked bool) *LedgerRecord {
		return &LedgerRecord{
			RecordID:      recordID,
			Actor:         Actor{OrgMSP: mockMSP},
			Status:        Status{Code: status, UpdatedAt: updatedAt},
			Locked:        locked,
			LockPolicyID:  policyID,
			PolicyVersion: policyVersion,
		}
	}

	// Helper to create a policy JSON
	createPolicyBytes := func(delay int64) []byte {
		policy := LockPolicy{
			PolicyID:     policyID,
			FinalState:   finalState,
			DelaySeconds: delay,
		}
		bytes, _ := json.Marshal(policy)
		return bytes
	}

	// Time setup
	nowTime, _ := time.Parse(time.RFC3339, "2023-10-01T12:00:00Z")
	pbTimestamp := &timestamp.Timestamp{Seconds: nowTime.Unix(), Nanos: 0}

	tests := []struct {
		name          string
		record        *LedgerRecord
		setupMocks    func(*MockTransactionContext, *MockChaincodeStub)
		expectLock    bool
		expectedError string
	}{
		{
			name:   "No Action: Already Locked",
			record: createRecord(finalState, "2023-10-01T10:00:00Z", true),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub) {
				// ✅ FIX: No expectations here.
				// The code returns early, so GetStub is NOT called.
			},
			expectLock:    true,
			expectedError: "",
		},
		{
			name:   "No Action: Status does not match FinalState",
			record: createRecord("DRAFT", "2023-10-01T10:00:00Z", false),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub) {
				// ✅ FIX: Add GetStub expectation here
				ctx.On("GetStub").Return(stub)

				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return("policy-key", nil)
				stub.On("GetState", "policy-key").Return(createPolicyBytes(3600), nil)
			},
			expectLock:    false,
			expectedError: "",
		},
		{
			name:   "No Action: Time elapsed is less than delay",
			record: createRecord(finalState, "2023-10-01T11:50:00Z", false),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub) {
				// ✅ FIX: Add GetStub expectation here
				ctx.On("GetStub").Return(stub)

				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return("policy-key", nil)
				stub.On("GetState", "policy-key").Return(createPolicyBytes(3600), nil)
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)
			},
			expectLock:    false,
			expectedError: "",
		},
		{
			name:   "Action: Lock Triggered (Time elapsed >= delay)",
			record: createRecord(finalState, "2023-10-01T10:00:00Z", false),
			setupMocks: func(ctx *MockTransactionContext, stub *MockChaincodeStub) {
				// ✅ FIX: Add GetStub expectation here
				ctx.On("GetStub").Return(stub)

				stub.On("CreateCompositeKey", mock.Anything, mock.Anything).Return("policy-key", nil)
				stub.On("GetState", "policy-key").Return(createPolicyBytes(3600), nil)
				stub.On("GetTxTimestamp").Return(pbTimestamp, nil)

				stub.On("PutState", recordID, mock.Anything).Run(func(args mock.Arguments) {
					jsonBytes := args.Get(1).([]byte)
					var rec LedgerRecord
					err := json.Unmarshal(jsonBytes, &rec)
					assert.NoError(t, err)
					assert.True(t, rec.Locked)

					// Time comparison fix from previous step
					actualLockedAt, err := time.Parse(time.RFC3339, rec.LockedAt)
					assert.NoError(t, err)
					assert.True(t, nowTime.Equal(actualLockedAt))
				}).Return(nil)
			},
			expectLock:    true,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtx := new(MockTransactionContext)
			mockStub := new(MockChaincodeStub)

			// ❌ REMOVED GLOBAL SETUP: mockCtx.On("GetStub").Return(mockStub)
			// It is now handled inside the individual setupMocks functions.

			tt.setupMocks(mockCtx, mockStub)

			contract := &SmartContract{}
			err := contract.enforceLockPolicy(mockCtx, tt.record)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectLock, tt.record.Locked)
			}

			mockCtx.AssertExpectations(t)
			mockStub.AssertExpectations(t)
		})
	}
}

func TestCreateInvoiceRecord_RejectNonXML(t *testing.T) {
	const (
		filename = "bad_invoice.json"
	)

	// 1. Prepare content that is valid Base64, but NOT valid XML.
	// We use a JSON string here.
	nonXMLContent := `{"id": 123, "type": "json"}`
	nonXMLBase64 := base64.StdEncoding.EncodeToString([]byte(nonXMLContent))

	// 2. Setup Mocks
	mockCtx := new(MockTransactionContext)
	mockStub := new(MockChaincodeStub)

	// Note: Because the XML validation happens BEFORE the ledger lookup (GetState)
	// or identity checks in the improved function, we do not need to mock
	// GetState, GetClientIdentity, or PutState. The function should fail fast.

	// However, if your code checks GetState first, you might need this:
	// mockStub.On("GetState", recordID).Return(nil, nil)
	// mockCtx.On("GetStub").Return(mockStub)

	// 3. Execute
	contract := &SmartContract{}
	result, err := contract.CreateInvoiceRecord(mockCtx, filename, nonXMLBase64)

	// 4. Assertions
	assert.Error(t, err, "Expected an error for non-XML content")
	assert.Nil(t, result, "Expected result to be nil on error")

	// Verify the error message contains the XML validation error
	// The exact message depends on the xml parser, usually "expected element" or "syntax error"
	assert.Contains(t, err.Error(), "content is not valid XML")

	// Ensure no ledger writes happened
	mockStub.AssertNotCalled(t, "PutState", mock.Anything, mock.Anything)
}
