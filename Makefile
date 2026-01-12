# Makefile to manage Hyperledger Fabric environment variables

# Global Exports
export PATH := $(BIN_DIR):$(PATH)
export FABRIC_CFG_PATH := $(CONFIG_DIR)
export CORE_PEER_TLS_ENABLED := true



# Base paths
FABRIC_SAMPLES ?= $(HOME)/fabric-samples
TEST_NETWORK := $(FABRIC_SAMPLES)/test-network
BIN_DIR := $(FABRIC_SAMPLES)/bin
CONFIG_DIR := $(FABRIC_SAMPLES)/config

# Upgrade Configuration
CC_VERSION  ?= 1.1
CC_SEQUENCE ?= 2


# Chaincode Name (Default is thika, can be overridden via CLI)
CC_NAME ?= thika

# Default to localhost, but allow overriding for remote servers
PEER_HOST ?= localhost

ORDERER_HOST ?= $(PEER_HOST)
ORDERER_PORT ?= 7050

ORG1_HOST ?= $(PEER_HOST)
ORG1_PORT ?= 7051

ORG2_HOST ?= $(PEER_HOST)
ORG2_PORT ?= 9051

ORG3_HOST ?= $(PEER_HOST)
ORG3_PORT ?= 11051

CHANNEL_NAME ?= mychannel

# Centralized CA Paths (Fixes "No such file" and "Empty Channel ID" errors)
ORDERER_CA := $(TEST_NETWORK)/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem
ORG1_TLS_ROOT := $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
ORG2_TLS_ROOT := $(TEST_NETWORK)/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
ORG3_TLS_ROOT := $(TEST_NETWORK)/organizations/peerOrganizations/org3.example.com/peers/peer0.org3.example.com/tls/ca.crt

.PHONY: env-org1 env-org2 env-org3 help package clean test reset rollback tunnel start start-org3 export-certs history init resume create check-accepted invoke-update

help:
	@echo "Usage: eval \$$(make <target>)"
	@echo ""
	@echo "Targets:"
	@echo "  check-accepted  Check if the chaincode is committed on the channel"
	@echo "  invoke-update   Invoke UpdateBusinessData with JSON arguments"
	@echo "  env-org1        Set environment for Organization 1 (Port 7051)"
	@echo "  env-org2        Set environment for Organization 2 (Port 9051)"
	@echo "  package         Package chaincode for production deployment"
	@echo "  test            Run unit tests"
	@echo "  clean           Remove generated package files"
	@echo "  reset           Tear down the Fabric network (removes all data)"
	@echo "  rollback        Rollback a record (Usage: make rollback REC_ID=... TARGET_TIME=...)"
	@echo "  start           Start network, create channel, and deploy chaincode"
	@echo "  history         View history of a record (Usage: make history REC_ID=...)"
	@echo "  init            Initialize ledger with dummy data"
	@echo "  create          Create a new record manually (Usage: make create REC_ID=... DESC=... STATUS=...)"

env-org1:
	@echo "export PATH=$(BIN_DIR):\$$PATH"
	@echo "export FABRIC_CFG_PATH=$(CONFIG_DIR)"
	@echo "export CORE_PEER_TLS_ENABLED=true"
	@echo "export CORE_PEER_LOCALMSPID=Org1MSP"
	@echo "export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT)"
	@echo "export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp"
	@echo "export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT)"
	@echo "export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org1.example.com"

env-org2:
	@echo "export PATH=$(BIN_DIR):\$$PATH"
	@echo "export FABRIC_CFG_PATH=$(CONFIG_DIR)"
	@echo "export CORE_PEER_TLS_ENABLED=true"
	@echo "export CORE_PEER_LOCALMSPID=Org2MSP"
	@echo "export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG2_TLS_ROOT)"
	@echo "export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp"
	@echo "export CORE_PEER_ADDRESS=$(ORG2_HOST):$(ORG2_PORT)"
	@echo "export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org2.example.com"

env-org3:
	@echo "export PATH=$(BIN_DIR):\$$PATH"
	@echo "export FABRIC_CFG_PATH=$(CONFIG_DIR)"
	@echo "export CORE_PEER_TLS_ENABLED=true"
	@echo "export CORE_PEER_LOCALMSPID=Org3MSP"
	@echo "export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG3_TLS_ROOT)"
	@echo "export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org3.example.com/users/Admin@org3.example.com/msp"
	@echo "export CORE_PEER_ADDRESS=$(ORG3_HOST):$(ORG3_PORT)"
	@echo "export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org3.example.com"

# Check if the chaincode is accepted (committed)
check-accepted:
	@echo "🔍 Checking if chaincode '$(CC_NAME)' is committed on '$(CHANNEL_NAME)'..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	peer lifecycle chaincode querycommitted --channelID $(CHANNEL_NAME) --name $(CC_NAME)

# Invoke the UpdateBusinessData transaction
# Uses your CURRENT active identity (Org1, Org2, Admin, or Creator)
invoke-update:
	@echo "🚀 Invoking UpdateBusinessData on $(CC_NAME)..."
	@# We explicitly unset SERVERHOSTOVERRIDE to allow connecting to both Org1 and Org2 simultaneously
	@export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"UpdateBusinessData","Args":["REC001", "{\"orderId\":\"ORD-1001\",\"amount\":150.75,\"currency\":\"TND\",\"status\":\"updated\"}"]}'

# Production: Package the chaincode
package:
	export PATH=$(BIN_DIR):$$PATH && export FABRIC_CFG_PATH=$(CONFIG_DIR) && peer lifecycle chaincode package $(CC_NAME).tar.gz --path . --lang golang --label $(CC_NAME)_1.0

# Run unit tests
test:
	go test -v .

clean:
	rm -f $(CC_NAME).tar.gz

# Tear down the network
reset:
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh down

# Rollback a record to a specific timestamp
rollback:
	@if [ -z "$(REC_ID)" ] || [ -z "$(TARGET_TIME)" ]; then \
		echo "Error: REC_ID and TARGET_TIME must be set"; \
		echo "Usage: make rollback REC_ID=REC001 TARGET_TIME=2025-12-28T12:00:00Z"; \
		exit 1; \
	fi
	@echo "Rolling back record $(REC_ID) to $(TARGET_TIME)..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"RollbackRecord","Args":["$(REC_ID)", "$(TARGET_TIME)"]}'

# Start ngrok tunnels
tunnel:
	ngrok start --config ngrok-fabric.yml --all

# Start the network, create channel, and deploy chaincode
start:
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh down
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh up createChannel -c $(CHANNEL_NAME) -ca
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh deployCC -ccn $(CC_NAME) -ccp $(CURDIR) -ccl go

# Start the network with Org3, create channel, and deploy chaincode with Org3 included
start-org3:
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh down
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh up createChannel -c $(CHANNEL_NAME) -ca
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK)/addOrg3 && ./addOrg3.sh up -c $(CHANNEL_NAME) -ca
	unset CORE_PEER_TLS_SERVERHOSTOVERRIDE CORE_PEER_ADDRESS CORE_PEER_LOCALMSPID CORE_PEER_MSPCONFIGPATH && cd $(TEST_NETWORK) && ./network.sh deployCC -ccn $(CC_NAME) -ccp $(CURDIR) -ccl go -ccep "OR('Org1MSP.peer','Org2MSP.peer','Org3MSP.peer')"

# Resume the network after a crash/reboot without wiping data
resume:
	@echo "Resuming nodes..."
	-docker start orderer.example.com peer0.org1.example.com peer0.org2.example.com
	@if [ -n "$$(docker ps -a -q -f name=peer0.org3.example.com)" ]; then \
		docker start peer0.org3.example.com; \
	fi
	@echo "Resuming chaincode containers..."
	@CC_CONTAINERS=$$(docker ps -a -q --filter "name=dev-peer"); if [ -n "$$CC_CONTAINERS" ]; then docker start $$CC_CONTAINERS; fi

# Package credentials for remote client
export-certs:
	@echo "Packaging crypto material for remote client..."
	tar -czf client-certs.tar.gz -C $(TEST_NETWORK) organizations
	@echo "Created client-certs.tar.gz. Copy this file to your remote device."

# Query the history of a record
history:
	@if [ -z "$(REC_ID)" ]; then \
		echo "Error: REC_ID must be set"; \
		echo "Usage: make history REC_ID=REC001"; \
		exit 1; \
	fi
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org1.example.com && \
	peer chaincode query \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		-c '{"function":"GetRecordHistory","Args":["$(REC_ID)"]}'

# Initialize the ledger with dummy data
init:
	@echo "Initializing ledger with dummy data..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"InitLedger","Args":[]}'

# Create a new record manually
create:
	@if [ -z "$(REC_ID)" ] || [ -z "$(DESC)" ] || [ -z "$(STATUS)" ]; then \
		echo "Error: REC_ID, DESC, and STATUS must be set"; \
		echo "Usage: make create REC_ID=REC005 DESC=\"New Item\" STATUS=CREATED"; \
		exit 1; \
	fi
	@echo "Creating record $(REC_ID)..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"CreateRecord","Args":["$(REC_ID)", "$(DESC)", "$(STATUS)"]}'

# Create a new admin user for Org1
create-admin-user:
	@if [ -z "$(USERNAME)" ] || [ -z "$(PASSWORD)" ]; then \
		echo "Error: USERNAME and PASSWORD must be set"; \
		echo "Usage: make create-admin-user USERNAME=newadmin PASSWORD=password123"; \
		exit 1; \
	fi
	@echo "1. Enrolling CA Bootstrap Admin..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/ && \
	fabric-ca-client enroll -u https://admin:adminpw@localhost:7054 --caname ca-org1 --tls.certfiles $(TEST_NETWORK)/organizations/fabric-ca/org1/tls-cert.pem
	@echo "2. Registering new user $(USERNAME)..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/ && \
	fabric-ca-client register --caname ca-org1 --id.name $(USERNAME) --id.secret $(PASSWORD) --id.type admin --tls.certfiles $(TEST_NETWORK)/organizations/fabric-ca/org1/tls-cert.pem
	@echo "3. Enrolling new user $(USERNAME)..."
	@export PATH=$(BIN_DIR):$$PATH && \
	fabric-ca-client enroll -u https://$(USERNAME):$(PASSWORD)@localhost:7054 --caname ca-org1 --tls.certfiles $(TEST_NETWORK)/organizations/fabric-ca/org1/tls-cert.pem --mspdir $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USERNAME)@org1.example.com/msp
	@echo "-----------------------------------------------------------------"
	@echo "Identity created at: $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USERNAME)@org1.example.com/msp"
	@echo "To make this user a Node Admin (able to install chaincode/join channels), run:"
	@echo "cp $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USERNAME)@org1.example.com/msp/signcerts/cert.pem $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/msp/admincerts/$(USERNAME)-cert.pem"


# ==============================================================================
# CHAINCODE UPGRADE AUTOMATION
# Usage: make upgrade CC_VERSION=1.1 CC_SEQUENCE=2
# ==============================================================================

.PHONY: upgrade package-cc install-cc approve-cc commit-cc

# Main target to run the full upgrade flow
upgrade: package-cc install-cc approve-cc commit-cc
	@echo "✅ Upgrade to version $(CC_VERSION) (Sequence $(CC_SEQUENCE)) complete!"

# 1. Package the chaincode with the new version label
# We remove old .tar.gz files first to prevent recursive packaging (bloating size)
package-cc:
	@echo "🧹 Cleaning up old package files..."
	@rm -f *.tar.gz
	@echo "📦 Packaging chaincode version $(CC_VERSION)..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	peer lifecycle chaincode package $(CC_NAME)_$(CC_VERSION).tar.gz \
		--path . --lang golang --label $(CC_NAME)_$(CC_VERSION)

# 2. Install the package on both Org1 and Org2
install-cc:
	@echo "⬇️  Installing chaincode on Org1..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org1.example.com && \
	peer lifecycle chaincode install $(CC_NAME)_$(CC_VERSION).tar.gz

	@echo "⬇️  Installing chaincode on Org2..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org2MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG2_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG2_HOST):$(ORG2_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org2.example.com && \
	peer lifecycle chaincode install $(CC_NAME)_$(CC_VERSION).tar.gz

# 3. Approve the definition for both Orgs
# Uses calculatepackageid to get the exact ID of the local tarball
approve-cc:
	@echo "👍 Approving for Org1..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org1.example.com && \
	PACKAGE_ID=$$(peer lifecycle chaincode calculatepackageid $(CC_NAME)_$(CC_VERSION).tar.gz) && \
	echo "   Calculated Package ID: $$PACKAGE_ID" && \
	peer lifecycle chaincode approveformyorg -o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		--channelID $(CHANNEL_NAME) --name $(CC_NAME) --version $(CC_VERSION) \
		--package-id $$PACKAGE_ID --sequence $(CC_SEQUENCE)

	@echo "👍 Approving for Org2..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org2MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG2_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG2_HOST):$(ORG2_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org2.example.com && \
	PACKAGE_ID=$$(peer lifecycle chaincode calculatepackageid $(CC_NAME)_$(CC_VERSION).tar.gz) && \
	peer lifecycle chaincode approveformyorg -o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		--channelID $(CHANNEL_NAME) --name $(CC_NAME) --version $(CC_VERSION) \
		--package-id $$PACKAGE_ID --sequence $(CC_SEQUENCE)

# 4. Commit the new definition
commit-cc:
	@echo "🚀 Committing chaincode definition..."
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer lifecycle chaincode commit -o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		--channelID $(CHANNEL_NAME) --name $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		--version $(CC_VERSION) --sequence $(CC_SEQUENCE)


# Create a new record using data from a file
# Usage: make create-file REC_ID=REC003 FILE=data.json USER_NAME=creator1
create-file:
	@if [ -z "$(REC_ID)" ] || [ -z "$(FILE)" ]; then \
		echo "Error: REC_ID and FILE must be set"; \
		exit 1; \
	fi
	@echo "--------------------------------------------------"
	@echo "📄 Reading data from: $(FILE)"
	@# Read file, remove newlines, and escape quotes for the CLI
	@$(eval JSON_DATA := $(shell cat $(FILE) | tr -d '\n' | sed 's/"/\\"/g'))
	@echo "--------------------------------------------------"
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USER_NAME)@org1.example.com/msp && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"CreateRecord","Args":["$(REC_ID)", "$(JSON_DATA)"]}'
