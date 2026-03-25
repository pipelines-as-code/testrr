#!/usr/bin/env bash
set -euo pipefail

if [[ ! -x ./bin/testrr ]]; then
  echo "missing ./bin/testrr; run make build first" >&2
  exit 2
fi

workdir="$(mktemp -d)"
server_pid=""

cleanup() {
  if [[ -n "${server_pid}" ]] && kill -0 "${server_pid}" 2>/dev/null; then
    kill "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -rf "${workdir}"
}

on_exit() {
  local status="$1"
  if [[ "${status}" -ne 0 && -f "${workdir}/server.log" ]]; then
    echo "testrr e2e failed; server log follows:" >&2
    cat "${workdir}/server.log" >&2
  fi
  cleanup
  exit "${status}"
}

trap 'on_exit $?' EXIT

port="$(python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
)"

export TESTRR_DATA_DIR="${workdir}/data"
export TESTRR_ADDR="127.0.0.1:${port}"
export TESTRR_AUTO_MIGRATE="true"

base_url="http://${TESTRR_ADDR}"
upload_json="${workdir}/upload.json"
runs_json="${workdir}/runs.json"
run_json="${workdir}/run.json"
dashboard_html="${workdir}/dashboard.html"
run_html="${workdir}/run.html"
test_html="${workdir}/test.html"
chart_json="${workdir}/chart.json"

./bin/testrr serve >"${workdir}/server.log" 2>&1 &
server_pid="$!"

for _ in $(seq 1 60); do
  if curl --silent --show-error --fail "${base_url}/" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl --silent --show-error --fail "${base_url}/" >/dev/null

printf 'secret\n' | ./bin/testrr project create \
  --slug e2e \
  --name "CI E2E" \
  --username ci \
  --password-stdin

curl --silent --show-error --fail \
  --user "ci:secret" \
  -F "files=@testdata/junit-mixed.xml" \
  -F "files=@testdata/go-test-sample.json" \
  -F "branch=main" \
  -F "run_label=ci-e2e" \
  -F "commit_sha=e2e-sha" \
  -F "build_id=ci-e2e-build" \
  "${base_url}/api/v1/projects/e2e/runs" >"${upload_json}"

run_id="$(python3 - "${upload_json}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

assert payload["status"] == "complete", payload
assert payload["run_label"] == "ci-e2e", payload
assert payload["branch"] == "main", payload
assert payload["total_count"] == 6, payload
assert payload["passed_count"] == 2, payload
assert payload["failed_count"] == 3, payload
assert payload["skipped_count"] == 1, payload
print(payload["id"])
PY
)"

curl --silent --show-error --fail \
  --user "ci:secret" \
  "${base_url}/api/v1/projects/e2e/runs?branch=main" >"${runs_json}"

python3 - "${runs_json}" "${run_id}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    runs = json.load(handle)

assert len(runs) == 1, runs
assert runs[0]["id"] == sys.argv[2], runs
assert runs[0]["run_label"] == "ci-e2e", runs
assert runs[0]["failed_count"] == 3, runs
PY

curl --silent --show-error --fail \
  --user "ci:secret" \
  "${base_url}/api/v1/projects/e2e/runs/${run_id}" >"${run_json}"

test_key="$(python3 - "${run_json}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

results = payload["results"]
assert len(results) == 6, payload
assert any(result["status"] == "skipped" for result in results), results
assert any(result["test_name"] == "TestParseFail" and result["status"] == "failed" for result in results), results
assert any(result["test_name"] == "TestFail" and result["failure_message"] == "boom" for result in results), results
for result in results:
    if result["test_name"] == "TestFail":
        print(result["test_key"])
        break
else:
    raise AssertionError(results)
PY
)"

curl --silent --show-error --fail \
  "${base_url}/projects/e2e?branch=main" >"${dashboard_html}"
grep -q "ci-e2e" "${dashboard_html}"
grep -q "Top failing tests" "${dashboard_html}"

curl --silent --show-error --fail \
  "${base_url}/projects/e2e/runs/${run_id}" >"${run_html}"
grep -q "TestParseFail" "${run_html}"
grep -q "TestSkip" "${run_html}"

encoded_test_key="$(python3 - "${test_key}" <<'PY'
import sys
import urllib.parse

print(urllib.parse.quote(sys.argv[1], safe=""))
PY
)"

curl --silent --show-error --fail \
  "${base_url}/projects/e2e/tests?test_key=${encoded_test_key}" >"${test_html}"
grep -q 'data-chart-kind="test-duration"' "${test_html}"
grep -F -q "${test_key}" "${test_html}"

curl --silent --show-error --fail \
  "${base_url}/projects/e2e/tests/chart?test_key=${encoded_test_key}" >"${chart_json}"

python3 - "${chart_json}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

assert payload["labels"] == ["ci-e2e"], payload
assert payload["statuses"] == ["failed"], payload
assert payload["durations"] == [50], payload
PY
