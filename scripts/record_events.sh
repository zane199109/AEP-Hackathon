#!/bin/bash
# Export on-chain events to mock_events.jsonl for offline replay.
# Red line: must export REAL chain events — no hand-written mocks.
#
# Usage:
#   export BASE_SEPOLIA_RPC="https://..."
#   bash scripts/record_events.sh <CONTRACT_ADDRESS> [FROM_BLOCK] [TO_BLOCK]

set -e

CONTRACT_ADDR=$1
FROM_BLOCK=${2:-latest}
TO_BLOCK=${3:-latest}

if [ -z "$CONTRACT_ADDR" ]; then
  echo "Usage: $0 <CONTRACT_ADDRESS> [FROM_BLOCK] [TO_BLOCK]"
  echo "Example: $0 0x1234...abcd 100000 100010"
  exit 1
fi

if [ -z "$BASE_SEPOLIA_RPC" ]; then
  echo "ERROR: BASE_SEPOLIA_RPC env var not set"
  exit 1
fi

OUTPUT_FILE="mock_events.jsonl"
echo "Exporting events from contract $CONTRACT_ADDR..."

# Export using cast (Foundry)
# BountyPosted event signature
cast logs \
  --rpc-url "$BASE_SEPOLIA_RPC" \
  --address "$CONTRACT_ADDR" \
  --from-block "$FROM_BLOCK" \
  --to-block "$TO_BLOCK" \
  --json \
  'BountyPosted(uint256,address,uint256,uint256)' \
  2>/dev/null | jq -c '.[] | {type: "BountyPosted", tx_hash: .transactionHash, log_index: .logIndex, job_id: .args[0], buyer: .args[1], amount: .args[2]}' > "$OUTPUT_FILE"

echo "Exported $(wc -l < "$OUTPUT_FILE") BountyPosted events to $OUTPUT_FILE"

# Append BountyClaimed events
cast logs \
  --rpc-url "$BASE_SEPOLIA_RPC" \
  --address "$CONTRACT_ADDR" \
  --from-block "$FROM_BLOCK" \
  --to-block "$TO_BLOCK" \
  --json \
  'BountyClaimed(uint256,address)' \
  2>/dev/null | jq -c '.[] | {type: "BountyClaimed", tx_hash: .transactionHash, log_index: .logIndex, job_id: .args[0], seller: .args[1]}' >> "$OUTPUT_FILE"

echo "Exported $(wc -l < "$OUTPUT_FILE") total events to $OUTPUT_FILE"
echo "Done. File: $OUTPUT_FILE"
