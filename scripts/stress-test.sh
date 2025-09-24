#!/bin/bash

# Stress test script - 1000 req/s target

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
API_URL=${API_URL:-"http://localhost:8080"}
DURATION=${1:-60} # Test duration in seconds
TARGET_RPS=1000   # Target requests per second

echo -e "${BLUE}========================================${NC}"
echo -e "${GREEN}WhatsApp Media Converter Stress Test${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Target: ${YELLOW}1000 requests/second${NC}"
echo -e "Duration: ${YELLOW}${DURATION} seconds${NC}"
echo -e "Total requests: ${YELLOW}$((TARGET_RPS * DURATION))${NC}"
echo -e "${BLUE}========================================${NC}"

# Check if vegeta is installed
if ! command -v vegeta &> /dev/null; then
    echo -e "${RED}Vegeta is not installed.${NC}"
    echo "Install it with:"
    echo "  brew install vegeta (macOS)"
    echo "  go install github.com/tsenart/vegeta@latest"
    exit 1
fi

# Test data
AUDIO_REQUEST='{"data":"UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA=","is_url":false}'
IMAGE_REQUEST='{"data":"/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAP/bAEMAAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAv/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIRAxEAPwCwAA8A/9k=","is_url":false}'

# Create target files
echo "POST ${API_URL}/convert/audio" > audio_targets.txt
echo "Content-Type: application/json" >> audio_targets.txt
echo "" >> audio_targets.txt
echo "${AUDIO_REQUEST}" >> audio_targets.txt

echo "POST ${API_URL}/convert/image" > image_targets.txt
echo "Content-Type: application/json" >> image_targets.txt
echo "" >> image_targets.txt
echo "${IMAGE_REQUEST}" >> image_targets.txt

# Warm up
echo -e "${YELLOW}Warming up...${NC}"
vegeta attack -rate=100 -duration=5s -targets=audio_targets.txt | vegeta report

# Run stress test
echo -e "${GREEN}Starting stress test...${NC}"

# Mixed workload (60% audio, 40% image)
echo -e "\n${YELLOW}Phase 1: Mixed workload (60% audio, 40% image)${NC}"
(
    vegeta attack -rate=$((TARGET_RPS * 60 / 100)) -duration=${DURATION}s -targets=audio_targets.txt > audio_results.bin &
    vegeta attack -rate=$((TARGET_RPS * 40 / 100)) -duration=${DURATION}s -targets=image_targets.txt > image_results.bin &
    wait
)

# Generate reports
echo -e "\n${GREEN}Generating reports...${NC}"

echo -e "\n${BLUE}Audio Conversion Results:${NC}"
vegeta report < audio_results.bin

echo -e "\n${BLUE}Image Conversion Results:${NC}"
vegeta report < image_results.bin

echo -e "\n${BLUE}Combined Results:${NC}"
cat audio_results.bin image_results.bin | vegeta report

# Generate histograms
vegeta plot < audio_results.bin > audio_plot.html
vegeta plot < image_results.bin > image_plot.html
cat audio_results.bin image_results.bin | vegeta plot > combined_plot.html

# Check if target was met
ACTUAL_RPS=$(cat audio_results.bin image_results.bin | vegeta report -type=json | jq '.rate')
SUCCESS_RATE=$(cat audio_results.bin image_results.bin | vegeta report -type=json | jq '.success')

echo -e "\n${BLUE}========================================${NC}"
echo -e "${GREEN}Test Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Target RPS: ${YELLOW}${TARGET_RPS}${NC}"
echo -e "Actual RPS: ${YELLOW}${ACTUAL_RPS}${NC}"
echo -e "Success Rate: ${YELLOW}${SUCCESS_RATE}%${NC}"

if (( $(echo "$ACTUAL_RPS >= $((TARGET_RPS * 95 / 100))" | bc -l) )) && (( $(echo "$SUCCESS_RATE >= 0.99" | bc -l) )); then
    echo -e "${GREEN}✓ PASSED: System achieved target performance!${NC}"
else
    echo -e "${RED}✗ FAILED: System did not meet target performance${NC}"
fi

echo -e "${BLUE}========================================${NC}"
echo -e "Reports saved:"
echo -e "  - audio_plot.html"
echo -e "  - image_plot.html"
echo -e "  - combined_plot.html"

# Clean up
rm -f audio_targets.txt image_targets.txt audio_results.bin image_results.bin