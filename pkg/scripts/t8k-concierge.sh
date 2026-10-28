#!/bin/bash

# TractStack v2 Build Concierge
# Processes build commands from /home/t8k/state/ directory
# Monitors for ULID-named CSV files and processes sequentially

set -e
set -o pipefail

# Basic logging function
log() {
  local line="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
  echo "$line"
  if [[ -n "${JOB_LOG:-}" ]]; then
    echo "$line" >>"$JOB_LOG"
  fi
}

log_error() {
  local line="[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*"
  echo "$line" >&2
  if [[ -n "${JOB_LOG:-}" ]]; then
    echo "$line" >>"$JOB_LOG"
  fi
}

# Run a command, teeing to JOB_LOG when set (rebuild-frontend)
run_logged() {
  if [[ -n "${JOB_LOG:-}" ]]; then
    "$@" 2>&1 | tee -a "$JOB_LOG"
    return "${PIPESTATUS[0]}"
  fi
  "$@"
}

# State directory (single location for all installations)
STATE_DIR="/home/t8k/state"
LOG_FILE="/home/t8k/log/concierge.log"

# Ensure log directory exists
mkdir -p "$(dirname "$LOG_FILE")"

# Function to parse CSV line
parse_csv() {
  local csv_line="$1"
  local type="" tenant="" site="" command="" job=""

  # Split CSV by commas and parse key=value pairs
  IFS=',' read -ra PAIRS <<<"$csv_line"
  for pair in "${PAIRS[@]}"; do
    IFS='=' read -r key value <<<"$pair"
    case "$key" in
    "type") type="$value" ;;
    "tenant") tenant="$value" ;;
    "site") site="$value" ;;
    "command") command="$value" ;;
    "job") job="$value" ;;
    esac
  done

  # Return parsed values via global variables
  CSV_TYPE="$type"
  CSV_TENANT="$tenant"
  CSV_SITE="$site"
  CSV_COMMAND="$command"
  CSV_JOB="$job"
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
  if ! go build -tags sqlite_fts5 -o "$go_binary" ./cmd/tractstack-go; then
    log_error "Go build failed"
    return 1
  fi

  # Make executable
  chmod +x "$go_binary"

  log "Go backend built successfully: $go_binary"
  return 0
}

iso_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

# Write job JSON (rebuild-frontend). Extra optional JSON object fields as raw suffix.
write_job_record() {
  local job_file="$1"
  local status="$2"
  shift 2
  local extras=("$@")
  local extra_json=""
  local e
  for e in "${extras[@]}"; do
    extra_json+=", ${e}"
  done
  local site_json="null"
  if [[ -n "$CSV_SITE" ]]; then
    site_json="\"$CSV_SITE\""
  fi
  cat >"$job_file" <<EOF
{"status":"$status","startedAt":"$JOB_STARTED_AT","type":"$CSV_TYPE","site":${site_json}${extra_json}}
EOF
}

prune_releases() {
  local astro_src_dir="$1"
  local releases_dir="$astro_src_dir/releases"

  if [[ ! -d "$releases_dir" ]]; then
    return 0
  fi

  local current=""
  if [[ -L "$astro_src_dir/dist" ]]; then
    current=$(readlink -f "$astro_src_dir/dist" 2>/dev/null || true)
  fi

  declare -A keep_set=()
  if [[ -n "$current" && "$current" == "$releases_dir"/* ]]; then
    keep_set["$current"]=1
  fi

  local dirs=()
  local line path
  while IFS= read -r line; do
    path="${line#* }"
    [[ -n "$path" ]] && dirs+=("$path")
  done < <(find "$releases_dir" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %p\n' 2>/dev/null | sort -rn)

  local kept=0
  local d
  for d in "${dirs[@]}"; do
    if [[ $kept -lt 3 ]]; then
      keep_set["$d"]=1
      kept=$((kept + 1))
    fi
  done

  for d in "${dirs[@]}"; do
    if [[ -z "${keep_set[$d]:-}" ]]; then
      log "Pruning old release: $d"
      rm -rf "$d"
    fi
  done
}

prune_jobs() {
  local jobs_dir="$1"
  if [[ ! -d "$jobs_dir" ]]; then
    return 0
  fi

  local jsons=()
  local line path
  while IFS= read -r line; do
    path="${line#* }"
    [[ -n "$path" ]] && jsons+=("$path")
  done < <(find "$jobs_dir" -maxdepth 1 -name '*.json' ! -name 'latest.json' -printf '%T@ %p\n' 2>/dev/null | sort -rn)

  local kept=0
  local f status base
  for f in "${jsons[@]}"; do
    status=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('status',''))" "$f" 2>/dev/null || echo "")
    case "$status" in
    started | building | swapping | reloading)
      continue
      ;;
    esac
    if [[ $kept -lt 20 ]]; then
      kept=$((kept + 1))
      continue
    fi
    base=$(basename "$f" .json)
    log "Pruning old job: $base"
    rm -f "$f" "$jobs_dir/$base.log"
  done
}

# Shared out-of-place Astro release build (both rebuild-frontend and build)
# Usage: build_astro_release <release-name> <stamp-id>
# Sets BUILD_WHITELIST_FAILED=1 on whitelist extract failure (non-fatal).
build_astro_release() {
  local release_name="$1"
  local stamp_id="$2"
  local astro_src_dir="$BUILD_SRC_DIR/my-tractstack"
  local release_dir="$astro_src_dir/releases/$release_name"
  BUILD_WHITELIST_FAILED=0

  log "Building Astro release $release_name (stamp $stamp_id) in $astro_src_dir"

  if [[ ! -d "$astro_src_dir" ]]; then
    log_error "Astro source directory not found: $astro_src_dir"
    return 1
  fi

  cd "$astro_src_dir"

  # First-run conversion: real dist directory → releases/legacy + symlink
  if [[ -d dist && ! -L dist ]]; then
    log "Converting dist directory to releases/legacy + symlink"
    mkdir -p releases && mv dist releases/legacy && ln -s releases/legacy dist && touch "$STATE_DIR/.layout-releases"
  fi

  mkdir -p src/generated releases
  local built_at
  built_at=$(iso_now)
  cat >src/generated/buildStamp.ts <<EOF
export const buildStamp = { jobId: '${stamp_id}', builtAt: '${built_at}' };
EOF

  if ! run_logged pnpm build --outDir "releases/${release_name}"; then
    log_error "Astro build failed (symlink left unchanged)"
    return 1
  fi

  if [[ -n "${JOB_JSON:-}" ]]; then
    write_job_record "$JOB_JSON" "swapping"
  fi

  if [[ ! -f "$release_dir/server/entry.mjs" ]]; then
    log_error "Release entry missing: $release_dir/server/entry.mjs (symlink left unchanged)"
    return 1
  fi

  ln -sfn "releases/${release_name}" dist.tmp && mv -Tf dist.tmp dist
  log "Swapped dist → releases/${release_name}"

  log "Extracting Tailwind whitelist from built assets..."
  local astro_dist_dir="$astro_src_dir/dist"
  local go_templates_dir="$BUILD_SRC_DIR/tractstack-go/internal/presentation/templates"
  local whitelist_output="$BUILD_DATA_DIR/config/default/tailwindWhitelist.json"

  if python3 /home/t8k/scripts/extractTailwindWhitelist.py "$astro_dist_dir" "$whitelist_output" "$go_templates_dir"; then
    log "Tailwind whitelist extracted successfully"
  else
    log_error "Failed to extract Tailwind whitelist"
    BUILD_WHITELIST_FAILED=1
  fi

  prune_releases "$astro_src_dir"
  log "Astro release built successfully: $release_dir"
  return 0
}

# Sysop Astro path: pull / pnpm up / install, then shared release layout
build_astro_frontend() {
  local astro_src_dir="$BUILD_SRC_DIR/my-tractstack"
  local release_name="build-$(date +%s)"

  log "Building Astro frontend from $astro_src_dir"

  if [[ ! -d "$astro_src_dir" ]]; then
    log_error "Astro source directory not found: $astro_src_dir"
    return 1
  fi

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

  if ! build_astro_release "$release_name" "$release_name"; then
    return 1
  fi

  return 0
}

# Author rebuild-frontend: no git pull, pnpm up, go build, or systemctl restart
process_rebuild_frontend() {
  local csv_file="$1"
  local jobs_dir="$STATE_DIR/jobs"
  local job_id="$CSV_JOB"
  local astro_src_dir

  if [[ -z "$job_id" ]]; then
    log_error "rebuild-frontend requires job parameter"
    rm -f "$csv_file"
    return 1
  fi

  if ! determine_build_paths "$CSV_TYPE" "$CSV_TENANT" "$CSV_SITE"; then
    return 1
  fi

  astro_src_dir="$BUILD_SRC_DIR/my-tractstack"
  mkdir -p "$jobs_dir"
  JOB_STARTED_AT=$(iso_now)
  JOB_JSON="$jobs_dir/$job_id.json"
  JOB_LOG="$jobs_dir/$job_id.log"
  : >"$JOB_LOG"

  write_job_record "$JOB_JSON" "started"
  log "rebuild-frontend job=$job_id type=$CSV_TYPE site=$CSV_SITE"

  cd "$astro_src_dir" || {
    log_error "Astro source directory not found: $astro_src_dir"
    write_job_record "$JOB_JSON" "failed" "\"log\":\"$JOB_LOG\""
    rm -f "$csv_file"
    prune_jobs "$jobs_dir"
    JOB_LOG=""
    JOB_JSON=""
    return 1
  }

  write_job_record "$JOB_JSON" "building"

  mkdir -p releases
  if [[ package.json -nt releases/.last-install ]] || [[ pnpm-lock.yaml -nt releases/.last-install ]]; then
    log "package.json or lockfile newer than releases/.last-install; running pnpm install --frozen-lockfile"
    if run_logged pnpm install --frozen-lockfile; then
      touch releases/.last-install
    else
      log_error "pnpm install --frozen-lockfile failed"
      write_job_record "$JOB_JSON" "failed" "\"log\":\"$JOB_LOG\""
      rm -f "$csv_file"
      prune_jobs "$jobs_dir"
      JOB_LOG=""
      JOB_JSON=""
      return 1
    fi
  else
    log "Skipping pnpm install (lockfile not newer than releases/.last-install)"
  fi

  if ! build_astro_release "$job_id" "$job_id"; then
    write_job_record "$JOB_JSON" "failed" "\"log\":\"$JOB_LOG\""
    rm -f "$csv_file"
    prune_jobs "$jobs_dir"
    JOB_LOG=""
    JOB_JSON=""
    return 1
  fi

  local extras=()
  if [[ "${BUILD_WHITELIST_FAILED:-0}" -eq 1 ]]; then
    extras+=("\"whitelist\":\"failed\"")
  fi

  write_job_record "$JOB_JSON" "reloading" "${extras[@]}"

  local astro_process_name="astro-main"
  if [[ "$CSV_TYPE" == "dedicated" ]]; then
    astro_process_name="astro-${BUILD_TENANT_ID}"
  fi

  if pm2 reload "$astro_process_name" >/dev/null 2>&1; then
    log "Reloaded $astro_process_name"
  else
    log "PM2 reload warning for $astro_process_name"
  fi

  local reloaded_at
  reloaded_at=$(iso_now)
  local succeed_extras=("\"reloadedAt\":\"$reloaded_at\"")
  if [[ "${BUILD_WHITELIST_FAILED:-0}" -eq 1 ]]; then
    succeed_extras+=("\"whitelist\":\"failed\"")
  fi
  write_job_record "$JOB_JSON" "succeeded" "${succeed_extras[@]}"
  cp -f "$JOB_JSON" "$jobs_dir/latest.json"
  rm -f "$csv_file"
  log "Removed processed file: $csv_file"

  prune_jobs "$jobs_dir"

  JOB_LOG=""
  JOB_JSON=""
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

  log "Parsed command - Type: $CSV_TYPE, Tenant: $CSV_TENANT, Site: $CSV_SITE, Command: $CSV_COMMAND, Job: $CSV_JOB"

  if [[ "$CSV_COMMAND" == "rebuild-frontend" ]]; then
    process_rebuild_frontend "$csv_file"
    return $?
  fi

  # Validate command
  if [[ "$CSV_COMMAND" != "build" ]]; then
    log_error "Unknown command: $CSV_COMMAND (only 'build' and 'rebuild-frontend' are supported)"
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

  # Build Astro frontend (release layout)
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
