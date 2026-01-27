#!/bin/bash
# tests/e2e/read_commands_test.sh
#
# End-to-end tests for all read-only tfccli commands.
# Tests both table and json output formats.
#
# Usage: ./tests/e2e/read_commands_test.sh <org-name>
# Example: ./tests/e2e/read_commands_test.sh Acme

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASS=0
FAIL=0
SKIP=0

# Check for org argument
if [ $# -lt 1 ]; then
    echo "Usage: $0 <org-name>"
    echo "Example: $0 Acme"
    exit 1
fi

ORG="$1"

echo -e "${BLUE}========================================"
echo "TFC CLI Read Commands E2E Test"
echo "Organization: $ORG"
echo -e "========================================${NC}"
echo ""

# Ensure tfccli binary is available
if ! command -v tfccli &> /dev/null; then
    # Try local bin
    if [ -f "./bin/tfccli" ]; then
        TFC="./bin/tfccli"
    else
        echo -e "${RED}Error: tfccli binary not found. Run 'make' first.${NC}"
        exit 1
    fi
else
    TFC="tfccli"
fi

echo "Using: $TFC"
echo ""

# --- Test Functions ---

test_command() {
    local desc="$1"
    local cmd="$2"
    local allow_fail="${3:-false}"
    
    printf "%-60s " "$desc"
    
    if output=$(eval "$cmd" 2>&1); then
        echo -e "${GREEN}✓ PASS${NC}"
        PASS=$((PASS + 1))
        return 0
    else
        exit_code=$?
        if [ "$allow_fail" = "true" ]; then
            echo -e "${YELLOW}⊘ SKIP${NC} (expected: may not be available)"
            SKIP=$((SKIP + 1))
            return 0
        else
            echo -e "${RED}✗ FAIL${NC} (exit: $exit_code)"
            echo "  Output: $output"
            FAIL=$((FAIL + 1))
            return 1
        fi
    fi
}

test_json_has_data() {
    local desc="$1"
    local cmd="$2"
    local allow_fail="${3:-false}"
    
    printf "%-60s " "$desc"
    
    if output=$(eval "$cmd" 2>&1); then
        # Check if output has "data" key (including null or empty array)
        if echo "$output" | jq -e 'has("data")' > /dev/null 2>&1; then
            echo -e "${GREEN}✓ PASS${NC}"
            PASS=$((PASS + 1))
            return 0
        else
            echo -e "${RED}✗ FAIL${NC} (no 'data' key in JSON)"
            FAIL=$((FAIL + 1))
            return 1
        fi
    else
        exit_code=$?
        if [ "$allow_fail" = "true" ]; then
            echo -e "${YELLOW}⊘ SKIP${NC}"
            SKIP=$((SKIP + 1))
            return 0
        else
            echo -e "${RED}✗ FAIL${NC} (exit: $exit_code)"
            FAIL=$((FAIL + 1))
            return 1
        fi
    fi
}

# --- Collect Resource IDs ---

echo -e "${BLUE}--- Collecting Resource IDs ---${NC}"
echo ""

# Verify org exists
echo -n "Verifying organization '$ORG'... "
if ! $TFC organizations get "$ORG" --output-format=json > /dev/null 2>&1; then
    echo -e "${RED}FAILED${NC}"
    echo "Organization '$ORG' not found or not accessible."
    exit 1
fi
echo -e "${GREEN}OK${NC}"

# Get first project ID
echo -n "Getting project ID... "
PROJECT_ID=$($TFC projects list --org="$ORG" --output-format=json 2>/dev/null | jq -r '.data[0].id // empty')
if [ -n "$PROJECT_ID" ]; then
    echo -e "${GREEN}$PROJECT_ID${NC}"
else
    echo -e "${YELLOW}none found${NC}"
    PROJECT_ID=""
fi

# Get first workspace ID
echo -n "Getting workspace ID... "
WORKSPACE_ID=$($TFC workspaces list --org="$ORG" --output-format=json 2>/dev/null | jq -r '.data[0].id // empty')
if [ -n "$WORKSPACE_ID" ]; then
    echo -e "${GREEN}$WORKSPACE_ID${NC}"
else
    echo -e "${YELLOW}none found${NC}"
    WORKSPACE_ID=""
fi

# Get run, plan, apply, cv IDs from workspace
RUN_ID=""
PLAN_ID=""
APPLY_ID=""
CV_ID=""
VAR_ID=""

if [ -n "$WORKSPACE_ID" ]; then
    # Get first run
    echo -n "Getting run ID... "
    RUN_ID=$($TFC runs list --workspace-id="$WORKSPACE_ID" --limit=1 --output-format=json 2>/dev/null | jq -r '.data[0].id // empty')
    if [ -n "$RUN_ID" ]; then
        echo -e "${GREEN}$RUN_ID${NC}"
        
        # Get plan/apply IDs from run details
        echo -n "Getting plan ID from run... "
        RUN_DETAILS=$($TFC runs get "$RUN_ID" --output-format=json 2>/dev/null)
        PLAN_ID=$(echo "$RUN_DETAILS" | jq -r '.data.plan_id // empty')
        if [ -n "$PLAN_ID" ]; then
            echo -e "${GREEN}$PLAN_ID${NC}"
        else
            echo -e "${YELLOW}none found${NC}"
        fi
        
        echo -n "Getting apply ID from run... "
        APPLY_ID=$(echo "$RUN_DETAILS" | jq -r '.data.apply_id // empty')
        if [ -n "$APPLY_ID" ]; then
            echo -e "${GREEN}$APPLY_ID${NC}"
        else
            echo -e "${YELLOW}none found${NC}"
        fi
    else
        echo -e "${YELLOW}none found${NC}"
    fi
    
    # Get first configuration version
    echo -n "Getting configuration version ID... "
    CV_ID=$($TFC configuration-versions list --workspace-id="$WORKSPACE_ID" --output-format=json 2>/dev/null | jq -r '.data[0].id // empty')
    if [ -n "$CV_ID" ]; then
        echo -e "${GREEN}$CV_ID${NC}"
    else
        echo -e "${YELLOW}none found${NC}"
    fi
    
    # Get first variable
    echo -n "Getting variable ID... "
    VAR_ID=$($TFC workspace-variables list --workspace-id="$WORKSPACE_ID" --output-format=json 2>/dev/null | jq -r '.data[0].id // empty')
    if [ -n "$VAR_ID" ]; then
        echo -e "${GREEN}$VAR_ID${NC}"
    else
        echo -e "${YELLOW}none found${NC}"
    fi
fi

echo ""
echo -e "${BLUE}--- Running Tests ---${NC}"
echo ""

# --- Organizations ---
echo -e "${BLUE}[Organizations]${NC}"
test_command "orgs list (table)" "$TFC organizations list --output-format=table"
test_json_has_data "orgs list (json)" "$TFC organizations list --output-format=json"
test_command "orgs get (table)" "$TFC organizations get '$ORG' --output-format=table"
test_json_has_data "orgs get (json)" "$TFC organizations get '$ORG' --output-format=json"
echo ""

# --- Projects ---
echo -e "${BLUE}[Projects]${NC}"
test_command "projects list (table)" "$TFC projects list --org='$ORG' --output-format=table"
test_json_has_data "projects list (json)" "$TFC projects list --org='$ORG' --output-format=json"
if [ -n "$PROJECT_ID" ]; then
    test_command "projects get (table)" "$TFC projects get '$PROJECT_ID' --output-format=table"
    test_json_has_data "projects get (json)" "$TFC projects get '$PROJECT_ID' --output-format=json"
else
    echo -e "  ${YELLOW}⊘ SKIP projects get - no project found${NC}"
    SKIP=$((SKIP + 2))
fi
echo ""

# --- Workspaces ---
echo -e "${BLUE}[Workspaces]${NC}"
test_command "workspaces list (table)" "$TFC workspaces list --org='$ORG' --output-format=table"
test_json_has_data "workspaces list (json)" "$TFC workspaces list --org='$ORG' --output-format=json"
if [ -n "$WORKSPACE_ID" ]; then
    test_command "workspaces get (table)" "$TFC workspaces get '$WORKSPACE_ID' --output-format=table"
    test_json_has_data "workspaces get (json)" "$TFC workspaces get '$WORKSPACE_ID' --output-format=json"
else
    echo -e "  ${YELLOW}⊘ SKIP workspaces get - no workspace found${NC}"
    SKIP=$((SKIP + 2))
fi
echo ""

# --- Workspace Variables ---
echo -e "${BLUE}[Workspace Variables]${NC}"
if [ -n "$WORKSPACE_ID" ]; then
    test_command "ws-variables list (table)" "$TFC workspace-variables list --workspace-id='$WORKSPACE_ID' --output-format=table"
    test_json_has_data "ws-variables list (json)" "$TFC workspace-variables list --workspace-id='$WORKSPACE_ID' --output-format=json"
    if [ -n "$VAR_ID" ]; then
        test_command "ws-variables get (table)" "$TFC workspace-variables get '$VAR_ID' --workspace-id='$WORKSPACE_ID' --output-format=table"
        test_json_has_data "ws-variables get (json)" "$TFC workspace-variables get '$VAR_ID' --workspace-id='$WORKSPACE_ID' --output-format=json"
    else
        echo -e "  ${YELLOW}⊘ SKIP ws-variables get - no variable found${NC}"
        SKIP=$((SKIP + 2))
    fi
else
    echo -e "  ${YELLOW}⊘ SKIP workspace-variables - no workspace found${NC}"
    SKIP=$((SKIP + 4))
fi
echo ""

# --- Workspace Resources ---
echo -e "${BLUE}[Workspace Resources]${NC}"
if [ -n "$WORKSPACE_ID" ]; then
    test_command "ws-resources list (table)" "$TFC workspace-resources list --workspace-id='$WORKSPACE_ID' --output-format=table"
    test_json_has_data "ws-resources list (json)" "$TFC workspace-resources list --workspace-id='$WORKSPACE_ID' --output-format=json"
else
    echo -e "  ${YELLOW}⊘ SKIP workspace-resources - no workspace found${NC}"
    SKIP=$((SKIP + 2))
fi
echo ""

# --- Runs ---
echo -e "${BLUE}[Runs]${NC}"
if [ -n "$WORKSPACE_ID" ]; then
    test_command "runs list (table)" "$TFC runs list --workspace-id='$WORKSPACE_ID' --output-format=table"
    test_json_has_data "runs list (json)" "$TFC runs list --workspace-id='$WORKSPACE_ID' --output-format=json"
    test_command "runs list with --limit (json)" "$TFC runs list --workspace-id='$WORKSPACE_ID' --limit=5 --output-format=json"
    if [ -n "$RUN_ID" ]; then
        test_command "runs get (table)" "$TFC runs get '$RUN_ID' --output-format=table"
        test_json_has_data "runs get (json)" "$TFC runs get '$RUN_ID' --output-format=json"
    else
        echo -e "  ${YELLOW}⊘ SKIP runs get - no run found${NC}"
        SKIP=$((SKIP + 2))
    fi
else
    echo -e "  ${YELLOW}⊘ SKIP runs - no workspace found${NC}"
    SKIP=$((SKIP + 5))
fi
echo ""

# --- Plans ---
echo -e "${BLUE}[Plans]${NC}"
if [ -n "$PLAN_ID" ]; then
    test_command "plans get (table)" "$TFC plans get '$PLAN_ID' --output-format=table" "true"
    test_json_has_data "plans get (json)" "$TFC plans get '$PLAN_ID' --output-format=json" "true"
    test_command "plans json-output (stdout)" "$TFC plans json-output '$PLAN_ID' | jq -e '.format_version'" "true"
    test_command "plans json-output (--out)" "$TFC plans json-output '$PLAN_ID' --out=/tmp/tfc-plan-test.json && jq -e '.format_version' /tmp/tfc-plan-test.json" "true"
else
    echo -e "  ${YELLOW}⊘ SKIP plans - no run/plan found${NC}"
    SKIP=$((SKIP + 4))
fi
echo ""

# --- Applies ---
echo -e "${BLUE}[Applies]${NC}"
if [ -n "$APPLY_ID" ]; then
    test_command "applies get (table)" "$TFC applies get '$APPLY_ID' --output-format=table" "true"
    test_json_has_data "applies get (json)" "$TFC applies get '$APPLY_ID' --output-format=json" "true"
else
    echo -e "  ${YELLOW}⊘ SKIP applies - no run/apply found${NC}"
    SKIP=$((SKIP + 2))
fi
echo ""

# --- Configuration Versions ---
echo -e "${BLUE}[Configuration Versions]${NC}"
if [ -n "$WORKSPACE_ID" ]; then
    test_command "cv list (table)" "$TFC configuration-versions list --workspace-id='$WORKSPACE_ID' --output-format=table"
    test_json_has_data "cv list (json)" "$TFC configuration-versions list --workspace-id='$WORKSPACE_ID' --output-format=json"
    if [ -n "$CV_ID" ]; then
        test_command "cv get (table)" "$TFC configuration-versions get '$CV_ID' --output-format=table"
        test_json_has_data "cv get (json)" "$TFC configuration-versions get '$CV_ID' --output-format=json"
        test_command "cv download (--out)" "$TFC configuration-versions download '$CV_ID' --out=/tmp/tfc-cv-test.tar.gz && ls -la /tmp/tfc-cv-test.tar.gz" "true"
    else
        echo -e "  ${YELLOW}⊘ SKIP cv get/download - no configuration version found${NC}"
        SKIP=$((SKIP + 3))
    fi
else
    echo -e "  ${YELLOW}⊘ SKIP configuration-versions - no workspace found${NC}"
    SKIP=$((SKIP + 5))
fi
echo ""

# --- Users ---
echo -e "${BLUE}[Users]${NC}"
test_command "users me (table)" "$TFC users me --output-format=table"
test_json_has_data "users me (json)" "$TFC users me --output-format=json"
# Get user ID from users me for testing users get
USER_ID=$($TFC users me --output-format=json 2>/dev/null | jq -r '.data.id // empty')
if [ -n "$USER_ID" ]; then
    test_command "users get (table)" "$TFC users get '$USER_ID' --output-format=table"
    test_json_has_data "users get (json)" "$TFC users get '$USER_ID' --output-format=json"
else
    echo -e "  ${YELLOW}⊘ SKIP users get - could not get user ID from users me${NC}"
    SKIP=$((SKIP + 2))
fi
echo ""

# --- Invoices (HCP Terraform Cloud only) ---
echo -e "${BLUE}[Invoices]${NC}"
test_command "invoices list (table)" "$TFC invoices list --org='$ORG' --output-format=table" "true"
test_json_has_data "invoices list (json)" "$TFC invoices list --org='$ORG' --output-format=json" "true"
test_command "invoices next (table)" "$TFC invoices next --org='$ORG' --output-format=table" "true"
test_json_has_data "invoices next (json)" "$TFC invoices next --org='$ORG' --output-format=json" "true"
echo ""

# --- Summary ---
echo -e "${BLUE}========================================"
echo "Test Summary"
echo -e "========================================${NC}"
echo -e "${GREEN}PASS: $PASS${NC}"
echo -e "${RED}FAIL: $FAIL${NC}"
echo -e "${YELLOW}SKIP: $SKIP${NC}"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
