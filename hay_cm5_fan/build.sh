#!/bin/bash
# Build script for HAY CM5 Fan Controller addon

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== HAY CM5 Fan Controller Addon Builder ===${NC}"

# Check if we're in the right directory
if [ ! -f "config.yaml" ] || [ ! -f "build.yaml" ] || [ ! -f "Dockerfile" ]; then
    echo -e "${RED}Error: This script must be run from the addon directory!${NC}"
    exit 1
fi

# Get addon slug and version
ADDON_SLUG=$(grep "^slug:" config.yaml | cut -d'"' -f2 | tr -d ' ')
ADDON_VERSION=$(grep "^version:" config.yaml | cut -d'"' -f2)

echo -e "Building ${GREEN}${ADDON_SLUG}${NC} version ${YELLOW}${ADDON_VERSION}${NC}"

# This addon only supports aarch64 (CM5 hardware)
ARCH=$(uname -m)
case $ARCH in
    aarch64)
        BUILD_ARCH="aarch64"
        ;;
    x86_64)
        # Allow building on amd64 for testing (won't have GPIO hardware)
        BUILD_ARCH="aarch64"
        echo -e "${YELLOW}Warning: Building aarch64 image on x86_64 — will need QEMU/buildx for cross-compilation${NC}"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        echo -e "${YELLOW}This addon only supports aarch64 (Raspberry Pi CM5)${NC}"
        exit 1
        ;;
esac

echo -e "Architecture: ${YELLOW}${BUILD_ARCH}${NC}"

# Get base image from build.yaml
BASE_IMAGE=$(grep -A1 "^build_from:" build.yaml | grep "  ${BUILD_ARCH}:" | awk '{print $2}')
if [ -z "$BASE_IMAGE" ]; then
    echo -e "${RED}Error: No base image found for architecture ${BUILD_ARCH}${NC}"
    exit 1
fi

echo -e "Base image: ${YELLOW}${BASE_IMAGE}${NC}"

# Build image name
IMAGE_NAME="local/${BUILD_ARCH}-addon-local_${ADDON_SLUG}"
IMAGE_TAG="${ADDON_VERSION}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo ""
echo -e "${BLUE}Building Docker image...${NC}"
echo "Image: ${FULL_IMAGE}"
echo ""

# Build the Docker image
docker build \
    --build-arg BUILD_FROM="${BASE_IMAGE}" \
    --build-arg BUILD_ARCH="${BUILD_ARCH}" \
    --build-arg BUILD_DATE="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --build-arg BUILD_DESCRIPTION="HAY CM5 Fan Controller for Home Assistant" \
    --build-arg BUILD_NAME="${ADDON_SLUG}" \
    --build-arg BUILD_REF="$(git rev-parse --short HEAD 2>/dev/null || echo 'local')" \
    --build-arg BUILD_REPOSITORY="local" \
    --build-arg BUILD_VERSION="${ADDON_VERSION}" \
    -t "${FULL_IMAGE}" \
    .

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}Build successful!${NC}"
    echo -e "Image: ${GREEN}${FULL_IMAGE}${NC}"
    echo ""
    echo "To test locally (limited without GPIO hardware):"
    echo -e "  ${YELLOW}docker run --rm -it --privileged ${FULL_IMAGE}${NC}"
    echo ""
    echo "To deploy to Home Assistant Yellow:"
    echo -e "  1. Ensure the addon folder is in ${YELLOW}/addons/${ADDON_SLUG}${NC}"
    echo -e "  2. Go to Supervisor -> Add-on Store -> Check for updates"
    echo -e "  3. Install/Update the addon"
else
    echo ""
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi
