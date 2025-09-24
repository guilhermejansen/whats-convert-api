#!/bin/bash

# Load test script for Media Converter API

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_URL=${API_URL:-"http://localhost:8080"}
TYPE=${1:-"audio"} # audio or image
CONCURRENT=${2:-100}
REQUESTS=${3:-1000}

# Test data
AUDIO_BASE64="UklGRiQAAABXQVZFZm10IBAAAAABAAEARKwAAIhYAQACABAAZGF0YQAAAAA="
IMAGE_BASE64="/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAP/bAEMAAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAv/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIRAxEAPwCwAA8A/9k="

echo -e "${GREEN}Starting load test for ${TYPE} conversion${NC}"
echo -e "Target: ${API_URL}/convert/${TYPE}"
echo -e "Concurrent requests: ${CONCURRENT}"
echo -e "Total requests: ${REQUESTS}"
echo ""

# Create temp file for request
TEMP_FILE=$(mktemp)
if [ "$TYPE" == "audio" ]; then
    echo "{\"data\":\"${AUDIO_BASE64}\",\"is_url\":false}" > ${TEMP_FILE}
else
    echo "{\"data\":\"${IMAGE_BASE64}\",\"is_url\":false}" > ${TEMP_FILE}
fi

# Run Apache Bench
echo -e "${YELLOW}Running test...${NC}"
ab -n ${REQUESTS} \
   -c ${CONCURRENT} \
   -T "application/json" \
   -p ${TEMP_FILE} \
   -g gnuplot.tsv \
   -e percentiles.csv \
   "${API_URL}/convert/${TYPE}"

# Clean up
rm ${TEMP_FILE}

echo -e "${GREEN}Test complete!${NC}"
echo -e "Results saved to:"
echo -e "  - gnuplot.tsv (for plotting)"
echo -e "  - percentiles.csv (response time percentiles)"