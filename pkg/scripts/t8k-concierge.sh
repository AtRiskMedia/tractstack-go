#!/bin/bash

# TractStack v2 Build Concierge
# Processes build commands from /home/t8k/state/ directory
# Monitors for ULID-named CSV files and processes sequentially

set -e

# Basic logging function
log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

log_error() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# State directory (single location for all installations)
STATE_DIR="/home/t8k/state"
LOG_FILE="/home/t8k/log/concierge.log"

# Ensure log directory exists
mkdir -p "$(dirname "$LOG_FILE")"

# Function to parse CSV line
parse_csv() {
  local csv_line="$1"
  local type="" tenant="" site="" command=""

  # Split CSV by commas and parse key=value pairs
  IFS=',' read -ra PAIRS <<<"$csv_line"
  for pair in "${PAIRS[@]}"; do
    IFS='=' read -r key value <<<"$pair"
    case "$key" in
    "type") type="$value" ;;
    "tenant") tenant="$value" ;;
    "site") site="$value" ;;
    "command") command="$value" ;;
    esac
  done

  # Return parsed values via global variables
  CSV_TYPE="$type"
  CSV_TENANT="$tenant"
  CSV_SITE="$site"
  CSV_COMMAND="$command"
}

# Function to determine build paths based on installation type
determine_build_paths() {
  local type="$1"
  local tenant="$2"
  local site="$3"

  case "$type" in
  "main" | "prod" | "multi")
    BUILD_SRC_DIR="/home/t8k/src"
    BUILD_BIN_DIR="/home/t8k/bin"
    BUILD_DATA_DIR="/home/t8k/t8k-go-server"
    BUILD_TENANT_ID="${tenant:-default}"
    ;;
  "dedicated")
    if [[ -z "$site" ]]; then
      log_error "Dedicated build requires site parameter"
      return 1
    fi
    BUILD_SRC_DIR="/home/t8k/sites/$site/src"
    BUILD_BIN_DIR="/home/t8k/sites/$site/bin"
    BUILD_DATA_DIR="/home/t8k/sites/$site/t8k-go-server"
    BUILD_TENANT_ID="$site"
    ;;
  *)
    log_error "Unknown installation type: $type"
    return 1
    ;;
  esac

  return 0
}

# Function to build Go backend
build_go_backend() {
  local go_src_dir="$BUILD_SRC_DIR/tractstack-go"
  local go_binary="$BUILD_BIN_DIR/tractstack-go"

  log "Building Go backend from $go_src_dir"

  if [[ ! -d "$go_src_dir" ]]; then
    log_error "Go source directory not found: $go_src_dir"
    return 1
  fi

  # Ensure bin directory exists
  mkdir -p "$BUILD_BIN_DIR"

  # Build Go binary
  cd "$go_src_dir"
  if ! go build -o "$go_binary" ./cmd/tractstack-go; then
    log_error "Go build failed"
    return 1
  fi

  # Make executable
  chmod +x "$go_binary"

  log "Go backend built successfully: $go_binary"
  return 0
}

# Function to build Astro frontend
build_astro_frontend() {
  local astro_src_dir="$BUILD_SRC_DIR/my-tractstack"

  log "Building Astro frontend from $astro_src_dir"

  if [[ ! -d "$astro_src_dir" ]]; then
    log_error "Astro source directory not found: $astro_src_dir"
    return 1
  fi

  # Check if git repo exists and handle accordingly
  if [[ -d "$astro_src_dir/.git" ]]; then
    cd "$astro_src_dir"
    if [[ -z "$(git status --porcelain)" ]]; then
      log "Git repo is clean, pulling latest changes..."
      git pull || log_error "git pull failed for my-tractstack"
    else
      log "Git repo has unstaged changes, skipping pull"
    fi
  else
    log "No git repository found, skipping pull"
  fi

  # Build Astro project
  cd "$astro_src_dir"
  log "Updating astro-tractstack to the latest version..."
  if ! pnpm up astro-tractstack --latest; then
    log_error "Failed to update astro-tractstack package"
    return 1
  fi
  if ! pnpm install; then
    log_error "pnpm install failed"
    return 1
  fi
  if ! pnpm build; then
    log_error "Astro build failed"
    return 1
  fi

  # Verify build artifacts exist
  if [[ ! -d "$astro_src_dir/dist" ]]; then
    log_error "Astro build artifacts not found: $astro_src_dir/dist"
    return 1
  fi

  log "Astro frontend built successfully: $astro_src_dir/dist"

  # Extract Tailwind whitelist from built assets
  log "Extracting Tailwind whitelist from built assets..."
  local astro_dist_dir="$BUILD_SRC_DIR/my-tractstack/dist"
  local go_templates_dir="$BUILD_SRC_DIR/tractstack-go/internal/presentation/templates"
  local whitelist_output="$BUILD_DATA_DIR/config/default/tailwindWhitelist.json"

  if python3 /home/t8k/scripts/extractTailwindWhitelist.py "$astro_dist_dir" "$whitelist_output" "$go_templates_dir"; then
    log "Tailwind whitelist extracted successfully"
  else
    log_error "Failed to extract Tailwind whitelist"
  fi

  return 0
}

# Function to process a single build command
process_build_command() {
  local csv_file="$1"
  local csv_content

  log "Processing build command from: $csv_file"

  # Read CSV content
  if ! csv_content=$(cat "$csv_file" 2>/dev/null); then
    log_error "Failed to read CSV file: $csv_file"
    return 1
  fi

  # Parse CSV
  parse_csv "$csv_content"

  log "Parsed command - Type: $CSV_TYPE, Tenant: $CSV_TENANT, Site: $CSV_SITE, Command: $CSV_COMMAND"

  # Validate command
  if [[ "$CSV_COMMAND" != "build" ]]; then
    log_error "Unknown command: $CSV_COMMAND (only 'build' is supported)"
    return 1
  fi

  # Determine build paths
  if ! determine_build_paths "$CSV_TYPE" "$CSV_TENANT" "$CSV_SITE"; then
    return 1
  fi

  log "Build paths - Source: $BUILD_SRC_DIR, Binary: $BUILD_BIN_DIR, Data: $BUILD_DATA_DIR"

  # Pull latest code from Git repositories
  log "Pulling latest code..."
  cd "$BUILD_SRC_DIR/tractstack-go" || {
    log_error "Failed to change directory to tractstack-go"
    return 1
  }
  git pull || {
    log_error "git pull failed for tractstack-go"
    return 1
  }

  # Update scripts after pulling
  #log "Updating scripts from tractstack-go..."
  #cp -r "$BUILD_SRC_DIR/tractstack-go/pkg/scripts/"* /home/t8k/scripts/ || {
  #  log_error "Failed to update scripts from tractstack-go"
  #  return 1
  #}
  #chmod +x /home/t8k/scripts/*.sh

  log "Code pull and script update successful."

  # Execute build process
  local build_success=true

  # Build Go backend
  if ! build_go_backend; then
    build_success=false
  fi

  # Build Astro frontend
  if ! build_astro_frontend; then
    build_success=false
  fi

  if [[ "$build_success" == true ]]; then
    # Restart services
    log "Build successful. Restarting services..."
    local go_service_name="tractstack-go"
    local astro_process_name="astro-main"

    if [[ "$CSV_TYPE" == "dedicated" ]]; then
      go_service_name="tractstack-go@${BUILD_TENANT_ID}"
      astro_process_name="astro-${BUILD_TENANT_ID}"
    fi

    if ! sudo systemctl restart "$go_service_name"; then
      log_error "Failed to restart $go_service_name"
      return 1
    fi
    log "Restarted $go_service_name"

    if pm2 reload "$astro_process_name" >/dev/null 2>&1; then
      log "Reloaded $astro_process_name"
    else
      log "PM2 reload warning for $astro_process_name"
    fi

    log "Build and restart completed successfully for $CSV_TYPE installation"
    # Remove processed file
    rm -f "$csv_file" || true
    log "Removed processed file: $csv_file"
  else
    log_error "Build failed for $CSV_TYPE installation"
    return 1
  fi

  return 0
}

# Main function
main() {
  log "TractStack Build Concierge starting"

  # Ensure state directory exists
  if [[ ! -d "$STATE_DIR" ]]; then
    log_error "State directory not found: $STATE_DIR"
    exit 1
  fi

  # Find all build-*.csv files and sort by name (chronological order for ULIDs)
  local build_files=()
  while IFS= read -r -d '' file; do
    build_files+=("$file")
  done < <(find "$STATE_DIR" -name "build-*.csv" -type f -print0 | sort -z)

  if [[ ${#build_files[@]} -eq 0 ]]; then
    log "No build files found in $STATE_DIR"
    exit 0
  fi

  log "Found ${#build_files[@]} build file(s) to process"

  # Process each file sequentially
  local processed=0
  local failed=0

  for build_file in "${build_files[@]}"; do
    if process_build_command "$build_file"; then
      processed=$((processed + 1))
    else
      failed=$((failed + 1))
      log_error "Failed to process: $build_file"
    fi
  done

  log "Build processing complete - Processed: $processed, Failed: $failed"

  if [[ $failed -gt 0 ]]; then
    log_error "Some builds failed"
    exit 1
  fi

  log "All builds completed successfully"
  echo "About to exit with 0"
  exit 0

}

# Run main function
main "$@"
