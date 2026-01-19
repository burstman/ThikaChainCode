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
	@echo "  create          Create a new record manually (Usage: make create REC_ID=... DESC=... STATUS=...)"
	@echo "  create-file     Create record from JSON file (Usage: make create-file REC_ID=... FILE=...)"

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

#absolute binary paths for use inside scripts
BIN_PATHS:
	@echo "export PATH=$(BIN_DIR):\$$PATH"



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

# Query History
# Redirects logs to stderr (>2) so you can pipe the JSON output to jq
history:
	@if [ -z "$(REC_ID)" ]; then echo "Error: REC_ID must be set" >&2; exit 1; fi
	@echo "Fetching history for record $(REC_ID)..." >&2
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	peer chaincode query -C $(CHANNEL_NAME) -n $(CC_NAME) \
		-c '{"function":"GetRecordHistory","Args":["$(REC_ID)", "$(START)", "$(END)"]}'

# Query records by Date Range (Rich Query)
# Usage: make query-range START="2026-01-01T00:00:00Z" END="2026-12-31T23:59:59Z"
query-range:
	@if [ -z "$(START)" ] || [ -z "$(END)" ]; then \
		echo "Error: START and END must be set (RFC3339 format)" >&2; \
		exit 1; \
	fi
	@echo "🔍 Searching records between $(START) and $(END)..." >&2
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export CORE_PEER_TLS_ENABLED=true && \
	peer chaincode query -C $(CHANNEL_NAME) -n $(CC_NAME) \
		-c '{"function":"GetRecordsByDateRange","Args":["$(START)", "$(END)"]}'


# Create a new record manually
# Usage: make create REC_ID=REC001 DESC="Item" STATUS=CREATED USER_NAME=creator1
USER_NAME ?= Admin
create:
	@if [ -z "$(REC_ID)" ] || [ -z "$(DESC)" ] || [ -z "$(STATUS)" ]; then \
		echo "Error: REC_ID, DESC, and STATUS must be set"; \
		exit 1; \
	fi
	@echo "👤 User: $(USER_NAME)"

	@export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USER_NAME)@org1.example.com/msp && \
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

# Create a new Network Admin using NodeOUs
# Usage: make create-admin USERNAME=opsadmin PASSWORD=opspass
create-admin:
	@if [ -z "$(USERNAME)" ] || [ -z "$(PASSWORD)" ]; then \
		echo "Error: USERNAME and PASSWORD must be set"; \
		exit 1; \
	fi
	@echo "👤 Creating Admin: $(USERNAME)..."
	
	@# CRITICAL FIX: Explicitly export PATH so the shell finds fabric-ca-client
	@export PATH=$(BIN_DIR):$$PATH && \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/ && \
	export TLS_CERT_FILE=$(TEST_NETWORK)/organizations/fabric-ca/org1/tls-cert.pem && \
	export USER_MSP_DIR=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USERNAME)@org1.example.com/msp && \
	\
	echo "🔍 Checking for CA TLS Certificate..." && \
	if [ ! -f "$$TLS_CERT_FILE" ]; then \
		echo "❌ Error: CA TLS certificate not found at $$TLS_CERT_FILE"; \
		echo "   👉 Did you start the network with the '-ca' flag? (e.g., ./network.sh up -ca)"; \
		exit 1; \
	fi && \
	\
	echo "1. Enrolling Bootstrap Admin (admin:adminpw) to get registrar permissions..." && \
	fabric-ca-client enroll -u https://admin:adminpw@localhost:7054 \
		--caname ca-org1 \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "2. Registering new admin (type=admin)..." && \
	(fabric-ca-client register \
		--caname ca-org1 \
		--id.name $(USERNAME) \
		--id.secret $(PASSWORD) \
		--id.type admin \
		--tls.certfiles "$$TLS_CERT_FILE" || echo "   ⚠️  User likely already registered, proceeding...") && \
	\
	echo "3. Enrolling new admin..." && \
	fabric-ca-client enroll \
		-u https://$(USERNAME):$(PASSWORD)@localhost:7054 \
		--caname ca-org1 \
		--mspdir "$$USER_MSP_DIR" \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "4. Copying config.yaml (Enables NodeOU Admin recognition)..." && \
	cp $(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/msp/config.yaml "$$USER_MSP_DIR/config.yaml" && \
	\
	echo "✅ Admin Identity ready at: $$USER_MSP_DIR"


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
	@echo "⬇️  Installing on Org1..."
	@export CORE_PEER_LOCALMSPID=Org1MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG1_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG1_HOST):$(ORG1_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org1.example.com && \
	peer lifecycle chaincode install $(CC_NAME)_$(CC_VERSION).tar.gz || true
	@echo "⬇️  Installing on Org2..."
	@export CORE_PEER_LOCALMSPID=Org2MSP && \
	export CORE_PEER_TLS_ROOTCERT_FILE=$(ORG2_TLS_ROOT) && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp && \
	export CORE_PEER_ADDRESS=$(ORG2_HOST):$(ORG2_PORT) && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=peer0.org2.example.com && \
	peer lifecycle chaincode install $(CC_NAME)_$(CC_VERSION).tar.gz || true

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
# Create a new record using data from a file
# Usage: make create-file REC_ID=REC003 FILE=data.json USER_NAME=creator1
create-file:
	@if [ -z "$(REC_ID)" ] || [ -z "$(FILE)" ]; then \
		echo "Error: REC_ID and FILE must be set"; \
		exit 1; \
	fi
	@echo "📄 Reading data from: $(FILE)"
	@$(eval JSON_DATA := $(shell cat $(FILE) | tr -d '\n' | sed 's/"/\\"/g'))
	@# We explicitly export FABRIC_CFG_PATH and PATH in this chain to guarantee peer finds the config
	@export FABRIC_CFG_PATH=$(CONFIG_DIR) && \
	export PATH=$(BIN_DIR):$$PATH && \
	export CORE_PEER_MSPCONFIGPATH=$(TEST_NETWORK)/organizations/peerOrganizations/org1.example.com/users/$(USER_NAME)@org1.example.com/msp && \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE= && \
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c '{"function":"CreateRecord","Args":["$(REC_ID)", "$(JSON_DATA)"]}'

# ==============================================================================
# 5. USER MANAGEMENT
# ==============================================================================

# Create or Restore a User Identity for the CURRENTLY ACTIVE organization
# Usage: make create-user USERNAME=creator1 PASSWORD=secret [ROLE=record_creator]
create-user:
	@if [ -z "$(USERNAME)" ] || [ -z "$(PASSWORD)" ]; then \
		echo "Error: USERNAME and PASSWORD must be set"; \
		echo "Usage: make create-user USERNAME=creator1 PASSWORD=secret [ROLE=record_creator]"; \
		exit 1; \
	fi
	@# 1. Determine Org details based on the environment variable
	@if [ "$$CORE_PEER_LOCALMSPID" = "Org1MSP" ]; then \
		ORG_NAME="org1"; \
		CA_NAME="ca-org1"; \
		CA_PORT="7054"; \
	elif [ "$$CORE_PEER_LOCALMSPID" = "Org2MSP" ]; then \
		ORG_NAME="org2"; \
		CA_NAME="ca-org2"; \
		CA_PORT="8054"; \
	else \
		echo "Error: CORE_PEER_LOCALMSPID is not set. Run 'eval \$$(make env-org1)' or 'eval \$$(make env-org2)' first." >&2; \
		exit 1; \
	fi; \
	\
	echo "👤 Processing user: $(USERNAME) for $$ORG_NAME..." && \
	\
	export PATH=$(BIN_DIR):$$PATH; \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/; \
	export TLS_CERT_FILE=$(TEST_NETWORK)/organizations/fabric-ca/$$ORG_NAME/tls-cert.pem; \
	export USER_MSP_DIR=$(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/users/$(USERNAME)@$$ORG_NAME.example.com/msp; \
	\
	echo "1. Enrolling Bootstrap Admin..." && \
	fabric-ca-client enroll -u https://admin:adminpw@localhost:$$CA_PORT \
		--caname $$CA_NAME \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "2. Registering user..." && \
	ATTRS_FLAG=""; \
	if [ -n "$(ROLE)" ]; then \
		ATTRS_FLAG="--id.attrs role=$(ROLE):ecert"; \
	fi; \
	(fabric-ca-client register \
		--caname $$CA_NAME \
		--id.name $(USERNAME) \
		--id.secret $(PASSWORD) \
		--id.type client \
		$$ATTRS_FLAG \
		--tls.certfiles "$$TLS_CERT_FILE" || echo "   ⚠️  User likely already registered. Proceeding to enrollment...") && \
	\
	echo "3. Enrolling user..." && \
	fabric-ca-client enroll \
		-u https://$(USERNAME):$(PASSWORD)@localhost:$$CA_PORT \
		--caname $$CA_NAME \
		--mspdir "$$USER_MSP_DIR" \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "4. Copying config.yaml..." && \
	cp $(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/msp/config.yaml "$$USER_MSP_DIR/config.yaml" && \
	\
	echo "✅ Identity for $(USERNAME) in $$ORG_NAME is ready!"


# List all registered users for the CURRENTLY ACTIVE organization
# We wrap the entire logic in one shell execution to preserve variables like ORG_NAME
list-users:
	@# 1. Determine Org details based on the environment variable
	@if [ "$$CORE_PEER_LOCALMSPID" = "Org1MSP" ]; then \
		ORG_NAME="org1"; \
		CA_NAME="ca-org1"; \
		CA_PORT="7054"; \
	elif [ "$$CORE_PEER_LOCALMSPID" = "Org2MSP" ]; then \
		ORG_NAME="org2"; \
		CA_NAME="ca-org2"; \
		CA_PORT="8054"; \
	else \
		echo "Error: CORE_PEER_LOCALMSPID is not set. Run 'eval \$$(make env-org1)' or 'eval \$$(make env-org2)' first." >&2; \
		exit 1; \
	fi; \
	\
	echo "📜 Listing all registered identities for $$CORE_PEER_LOCALMSPID..." >&2; \
	\
	# 2. Setup Environment and Run Commands (Chained with backslashes) \
	export PATH=$(BIN_DIR):$$PATH; \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/; \
	export TLS_CERT_FILE=$(TEST_NETWORK)/organizations/fabric-ca/$$ORG_NAME/tls-cert.pem; \
	\
	echo "1. Enrolling Bootstrap Admin for $$ORG_NAME..." >&2; \
	fabric-ca-client enroll -u https://admin:adminpw@localhost:$$CA_PORT \
		--caname $$CA_NAME \
		--tls.certfiles "$$TLS_CERT_FILE" > /dev/null 2>&1; \
	\
	echo "2. Fetching and parsing Identity List..." >&2; \
	echo "----------------------------------------------------------------" >&2; \
	fabric-ca-client identity list \
		--caname $$CA_NAME \
		--tls.certfiles "$$TLS_CERT_FILE" | \
	grep "Name:" | \
	awk -F', ' '{ \
		gsub("Name: ", "", $$1); \
		gsub("Type: ", "", $$2); \
		gsub("Affiliation: ", "", $$3); \
		role = ""; \
		if (match($$0, /Name:role Value:[^}]+/)) { \
			raw = substr($$0, RSTART, RLENGTH); \
			sub("Name:role Value:", "", raw); \
			role = raw; \
		} \
		printf "{\"id\": \"%s\", \"type\": \"%s\", \"affiliation\": \"%s\", \"role\": \"%s\"}\n", $$1, $$2, $$3, role \
	}' | jq -s '.'



# ==============================================================================
# 6. IDENTITY MANAGEMENT
# ==============================================================================

# Add an attribute to an existing user and re-enroll (Dynamic for Org1/Org2)
# Usage: make add-attribute USERNAME=hamed PASSWORD=pass ATTRS="role=record_creator"
add-attribute:
	@if [ -z "$(USERNAME)" ] || [ -z "$(PASSWORD)" ] || [ -z "$(ATTRS)" ]; then \
		echo "Error: USERNAME, PASSWORD, and ATTRS must be set"; \
		echo "Usage: make add-attribute USERNAME=hamed PASSWORD=pass ATTRS=\"role=record_creator\""; \
		exit 1; \
	fi
	@# 1. Determine Org details based on the environment variable
	@if [ "$$CORE_PEER_LOCALMSPID" = "Org1MSP" ]; then \
		ORG_NAME="org1"; \
		CA_NAME="ca-org1"; \
		CA_PORT="7054"; \
	elif [ "$$CORE_PEER_LOCALMSPID" = "Org2MSP" ]; then \
		ORG_NAME="org2"; \
		CA_NAME="ca-org2"; \
		CA_PORT="8054"; \
	else \
		echo "Error: CORE_PEER_LOCALMSPID is not set. Run 'eval \$$(make env-org1)' or 'eval \$$(make env-org2)' first." >&2; \
		exit 1; \
	fi; \
	\
	echo "🔄 Adding attribute '$(ATTRS)' to user '$(USERNAME)' in $$ORG_NAME..." && \
	\
	export PATH=$(BIN_DIR):$$PATH; \
	export FABRIC_CA_CLIENT_HOME=$(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/; \
	export TLS_CERT_FILE=$(TEST_NETWORK)/organizations/fabric-ca/$$ORG_NAME/tls-cert.pem; \
	export USER_MSP_DIR=$(TEST_NETWORK)/organizations/peerOrganizations/$$ORG_NAME.example.com/users/$(USERNAME)@$$ORG_NAME.example.com/msp; \
	\
	echo "1. Modifying identity in CA database..." && \
	fabric-ca-client identity modify $(USERNAME) \
		--attrs '$(ATTRS):ecert' \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "2. Re-enrolling to generate new certificate..." && \
	fabric-ca-client enroll \
		-u https://$(USERNAME):$(PASSWORD)@localhost:$$CA_PORT \
		--caname $$CA_NAME \
		--mspdir "$$USER_MSP_DIR" \
		--tls.certfiles "$$TLS_CERT_FILE" && \
	\
	echo "✅ Attribute added! Verifying..." && \
	openssl x509 -in "$$USER_MSP_DIR/signcerts/cert.pem" -text -noout | grep "$(ATTRS)" -A 1 || echo "⚠️  Warning: Attribute not found in grep check, please verify manually."

# Create a new lock policy
# Usage: make create-policy POLICY_ID=P01 VERSION=1 ACTIVE=true [ORG_MSP=Org1MSP]
create-policy:
	@if [ -z "$(POLICY_ID)" ] || [ -z "$(VERSION)" ] || [ -z "$(ACTIVE)" ]; then \
		echo "Error: POLICY_ID, VERSION, and ACTIVE must be set" >&2; \
		exit 1; \
	fi
	@# Logic: Use ORG_MSP arg if present, otherwise use Env Variable, otherwise fail
	@TARGET_MSP="$(ORG_MSP)"; \
	if [ -z "$$TARGET_MSP" ]; then \
		TARGET_MSP="$$CORE_PEER_LOCALMSPID"; \
	fi; \
	if [ -z "$$TARGET_MSP" ]; then \
		echo "Error: ORG_MSP not provided and CORE_PEER_LOCALMSPID not set." >&2; \
		exit 1; \
	fi; \
	\
	echo "Creating policy $(POLICY_ID) v$(VERSION) for $$TARGET_MSP..." >&2; \
	\
	export PATH=$(BIN_DIR):$$PATH; \
	export FABRIC_CFG_PATH=$(CONFIG_DIR); \
	export CORE_PEER_TLS_SERVERHOSTOVERRIDE=; \
	\
	peer chaincode invoke \
		-o $(ORDERER_HOST):$(ORDERER_PORT) \
		--ordererTLSHostnameOverride orderer.example.com \
		--tls --cafile "$(ORDERER_CA)" \
		-C $(CHANNEL_NAME) \
		-n $(CC_NAME) \
		--peerAddresses $(ORG1_HOST):$(ORG1_PORT) --tlsRootCertFiles "$(ORG1_TLS_ROOT)" \
		--peerAddresses $(ORG2_HOST):$(ORG2_PORT) --tlsRootCertFiles "$(ORG2_TLS_ROOT)" \
		-c "{\"function\":\"CreateLockPolicy\",\"Args\":[\"$$TARGET_MSP\", \"$(POLICY_ID)\", \"$(VERSION)\", \"$(ACTIVE)\", \"Example Condition\"]}"

# Get the currently active lock policy
# Usage: make get-active-policy [ORG_MSP=Org1MSP]
get-active-policy:
	@# Logic: Use ORG_MSP arg if present, otherwise use Env Variable, otherwise fail
	@TARGET_MSP="$(ORG_MSP)"; \
	if [ -z "$$TARGET_MSP" ]; then \
		TARGET_MSP="$$CORE_PEER_LOCALMSPID"; \
	fi; \
	if [ -z "$$TARGET_MSP" ]; then \
		echo "Error: ORG_MSP not provided and CORE_PEER_LOCALMSPID not set." >&2; \
		exit 1; \
	fi; \
	\
	echo "Querying for active policy for $$TARGET_MSP..." >&2; \
	\
	export PATH=$(BIN_DIR):$$PATH; \
	export FABRIC_CFG_PATH=$(CONFIG_DIR); \
	export CORE_PEER_TLS_ENABLED=true; \
	\
	peer chaincode query -C $(CHANNEL_NAME) -n $(CC_NAME) \
		-c "{\"function\":\"GetActiveLockPolicy\",\"Args\":[\"$$TARGET_MSP\"]}" | jq
