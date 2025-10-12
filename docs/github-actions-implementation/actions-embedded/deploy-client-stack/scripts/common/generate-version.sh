#!/bin/bash
# CalVer Version Generation (replaces reecetech/version-increment@2023.10.2)

set -euo pipefail

source /scripts/common/logging.sh

generate_calver_version() {
    local version_suffix="${VERSION_SUFFIX:-}"
    
    # Generate CalVer format: YYYY.M.D.BUILD_NUMBER
    local year=$(date +%Y)
    local month=$(date +%-m)  # Remove leading zero
    local day=$(date +%-d)    # Remove leading zero
    local build_number="${GITHUB_RUN_NUMBER:-1}"
    
    # Base version
    local version="${year}.${month}.${day}.${build_number}"
    
    # Add suffix if provided
    if [[ -n "$version_suffix" ]]; then
        version="${version}${version_suffix}"
    fi
    
    echo "$version"
}

validate_version_via_api() {
    local version="$1"
    
    # Use GitHub API to validate version doesn't conflict
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        log_info "Validating version against GitHub releases"
        
        # Check if tag already exists
        if gh api "repos/${GITHUB_REPOSITORY}/releases/tags/v${version}" >/dev/null 2>&1; then
            log_warning "Version v${version} already exists, appending timestamp"
            local timestamp=$(date +%H%M%S)
            version="${version}.${timestamp}"
        fi
    fi
    
    echo "$version"
}

main() {
    log_info "Generating CalVer version"
    
    # Handle app-image-version override
    if [[ -n "${APP_IMAGE_VERSION:-}" ]]; then
        log_info "Using provided app-image-version: ${APP_IMAGE_VERSION}"
        echo "${APP_IMAGE_VERSION}" > /tmp/deploy_version
        export VERSION="${APP_IMAGE_VERSION}"
        return 0
    fi
    
    # Generate CalVer version
    local version
    version=$(generate_calver_version)
    
    # Validate against API if token available
    version=$(validate_version_via_api "$version")
    
    log_info "Generated version: ${version}"
    
    # Export for use by other scripts
    echo "$version" > /tmp/deploy_version
    export VERSION="$version"
    
    # Set GitHub output
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "version=$version" >> "$GITHUB_OUTPUT"
    fi
}

main "$@"
