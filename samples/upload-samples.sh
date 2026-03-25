#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SAMPLES_DIR="${ROOT_DIR}/samples"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
CLI_BIN="${CLI_BIN:-${ROOT_DIR}/bin/testrr}"

run_cli() {
  if [[ -x "${CLI_BIN}" ]]; then
    "${CLI_BIN}" "$@"
    return
  fi
  (
    cd "${ROOT_DIR}"
    go run ./cmd/testrr "$@"
  )
}

project_exists() {
  local slug="$1"
  run_cli project list | awk -F '\t' -v slug="${slug}" '$1 == slug { found = 1 } END { exit(found ? 0 : 1) }'
}

ensure_project() {
  local slug="$1"
  local name="$2"
  local username="$3"
  local password="$4"

  if project_exists "${slug}"; then
    printf 'project exists: %s\n' "${slug}"
    return
  fi

  printf 'creating project: %s\n' "${slug}"
  printf '%s\n' "${password}" | run_cli project create \
    --slug "${slug}" \
    --name "${name}" \
    --username "${username}" \
    --password-stdin
}

upload_run() {
  local slug="$1"
  local username="$2"
  local password="$3"
  local run_dir="$4"
  local meta_file="${run_dir}/meta.env"
  local xml_files=("${run_dir}"/*.xml)

  if [[ ! -f "${meta_file}" ]]; then
    printf 'missing metadata file: %s\n' "${meta_file}" >&2
    exit 1
  fi
  if [[ ! -e "${xml_files[0]}" ]]; then
    printf 'no xml files in: %s\n' "${run_dir}" >&2
    exit 1
  fi

  # shellcheck disable=SC1090
  source "${meta_file}"

  local curl_args=(
    --silent
    --show-error
    --fail
    --user "${username}:${password}"
    --request POST
    --form "branch=${BRANCH}"
    --form "environment=${ENVIRONMENT}"
    --form "build_id=${BUILD_ID}"
    --form "build_url=${BUILD_URL}"
    --form "commit_sha=${COMMIT_SHA}"
    --form "run_label=${RUN_LABEL}"
    --form "started_at=${STARTED_AT}"
  )

  local file
  for file in "${xml_files[@]}"; do
    curl_args+=(--form "files=@${file};type=text/xml")
  done

  printf 'uploading %s (%s)\n' "${slug}" "${RUN_LABEL}"
  curl "${curl_args[@]}" "${BASE_URL}/api/v1/projects/${slug}/runs" > /dev/null
}

load_project_timeline() {
  local slug="$1"
  local username="$2"
  local password="$3"
  local project_dir="${SAMPLES_DIR}/${slug}"
  local run_dir

  for run_dir in "${project_dir}"/run-*; do
    upload_run "${slug}" "${username}" "${password}" "${run_dir}"
  done
}

main() {
  ensure_project "demo-app" "Demo App" "demo-uploader" "demo-secret"
  ensure_project "widget-shop" "Widget Shop" "widget-uploader" "widget-secret"

  load_project_timeline "demo-app" "demo-uploader" "demo-secret"
  load_project_timeline "widget-shop" "widget-uploader" "widget-secret"

  printf '\nSamples loaded.\n'
  printf 'Browse: %s/\n' "${BASE_URL}"
  printf 'Demo App: %s/projects/demo-app\n' "${BASE_URL}"
  printf 'Widget Shop: %s/projects/widget-shop\n' "${BASE_URL}"
}

main "$@"
