# testrr - Track every test. Catch every regression.

<img width="3034" height="2014" alt="image" src="https://github.com/user-attachments/assets/01c36568-ce91-4382-867f-dadfed6a869a" />

A self-hosted test report hub. Upload JUnit, TRX, or NUnit reports from your CI pipelines and browse pass/fail history, trends, and regressions in one place.

## Deployment

### Docker (recommended for VMs)

No Go or Node required on the host — only Docker.

```sh
docker build -t testrr .
docker run -d \
  -p 8080:8080 \
  -v ./data:/home/testrr/data \
  --name testrr \
  testrr
```

The `data` volume persists the SQLite database and uploaded artifacts across container restarts. Set `TESTRR_DATABASE_URL` to a `postgres://` URL if you prefer PostgreSQL.

After the container starts, create your first project:

```sh
docker exec -i testrr testrr project create \
  --slug my-project \
  --name "My Project" \
  --username ci \
  --password-stdin <<< "secret"
```

### systemd with Podman

The sample unit at [`systemd/testrr.service`](/Users/chmouel/git/work/testrr/systemd/testrr.service) stores persistent data in `/var/lib/testrr/data` and is meant to run as a system-wide Podman service.

The service intentionally avoids extra systemd sandbox directives because Podman needs access to its own runtime and storage paths under `/run` and `/var/lib/containers`.


Create the data directory before starting the service:

```sh
sudo mkdir -p /var/lib/testrr/data
sudo chown 1000:1000 /var/lib/testrr/data
sudo cp systemd/testrr.service /etc/systemd/system/testrr.service
sudo systemctl daemon-reload
sudo systemctl enable --now testrr.service
```

## Quick start (local dev)

```sh
make build
./bin/testrr migrate
./bin/testrr project create \
  --slug my-project \
  --name "My Project" \
  --username ci \
  --password-stdin <<< "secret"
./bin/testrr serve
```

Open `http://localhost:8080`.

## Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|---|---|---|
| `TESTRR_ADDR` | `:8080` | Listen address |
| `TESTRR_DATA_DIR` | `data` | Directory for SQLite database and uploaded artifacts |
| `TESTRR_DATABASE_URL` | `data/testrr.sqlite` | Database connection string. Use a `postgres://` URL for PostgreSQL |
| `TESTRR_MAX_UPLOAD_MB` | `100` | Maximum upload size in megabytes |
| `TESTRR_AUTO_MIGRATE` | `true` | Run database migrations automatically on startup |
| `TESTRR_OUTPUT_RETENTION_DAYS` | `30` | Keep compressed per-test output in the database for this many days. Run metadata and artifacts are kept indefinitely |

## Uploading test reports

Uploads use HTTP Basic Auth with the project credentials you created above.

```sh
curl -u ci:secret \
  -F "files=@report.xml" \
  http://localhost:8080/api/v1/projects/my-project/runs
```

### Optional metadata fields

Pass these as form fields alongside the file(s):

| Field | Description |
|---|---|
| `branch` | Branch name — used to scope regression detection to the same branch |
| `commit_sha` | Git commit SHA |
| `build_id` | CI build identifier |
| `build_url` | Link back to the CI build |
| `environment` | Deployment environment (e.g. `staging`) |
| `run_label` | Human-readable label for the run. Defaults to `build_id`, then `commit_sha`, then upload timestamp |
| `started_at` | RFC 3339 timestamp of when the run started |

### Upload multiple files

```sh
curl -u ci:secret \
  -F "files=@junit.xml" \
  -F "files=@cypress.xml" \
  -F "branch=main" \
  -F "commit_sha=abc123" \
  http://localhost:8080/api/v1/projects/my-project/runs
```

### GitHub Actions example

```yaml
- name: Upload test results
  if: always()
  run: |
    curl -u "${{ secrets.TESTRR_USER }}:${{ secrets.TESTRR_PASS }}" \
      -F "files=@test-results/junit.xml" \
      -F "branch=${{ github.ref_name }}" \
      -F "commit_sha=${{ github.sha }}" \
      -F "build_url=${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}" \
      ${{ vars.TESTRR_URL }}/api/v1/projects/my-project/runs
```

`go test -json` output can also be uploaded directly with a `.json` or `.jsonl`
file.

## Supported report formats

| Format | Extension | Notes |
|---|---|---|
| JUnit XML | `.xml` | Standard `<testsuite>` / `<testsuites>` format |
| Go test JSON | `.json` or `.jsonl` | Newline-delimited `go test -json` output |
| TRX | `.trx` or `.xml` | Visual Studio / `dotnet test` output |
| NUnit | `.xml` | NUnit 3 (`<test-run>`) and NUnit 2 (`<test-results>`) |

## Project management

```sh
# List all projects
./bin/testrr project list

# Rotate credentials
./bin/testrr project rotate-password \
  --slug my-project \
  --username ci \
  --password-stdin <<< "newsecret"

# Run migrations manually
./bin/testrr migrate

# Prune old heavy per-test output from the database and vacuum SQLite
./bin/testrr storage prune

# Override the retention window for a one-off maintenance run
./bin/testrr storage prune --retention-days 90 --vacuum=false
```

`storage prune` removes only heavy per-test payloads such as failure output and stdout/stderr from the database after the retention window. Runs, test summaries, and uploaded report artifacts remain available.

## Development

```sh
make deps      # install Go and Node dependencies
make generate  # regenerate templ templates
make assets    # build frontend assets
make test      # run tests
make build     # build binary
make check     # generate + assets + lint + test + build
```

## CI Uploads

GitHub Actions uploads this repo's raw `go test -json` results to
`https://testrr.pipelinesascode.com` using project `testrr` and username `pac`
when the `TESTRR_PASSWORD` repository secret is set.
