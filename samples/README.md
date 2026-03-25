# Sample Data

This directory contains complete sample timelines for local testing.

- `demo-app/` includes four historical runs for the same project, with multiple JUnit XML files per run.
- `widget-shop/` includes a smaller second project so you can verify multi-project behavior.
- `upload-samples.sh` creates missing projects and uploads every sample run in chronological order.

## Quick Start

Start the app:

```bash
TESTRR_ADDR=:18080 go run ./cmd/testrr serve
```

Load the samples:

```bash
BASE_URL=http://127.0.0.1:18080 bash samples/upload-samples.sh
```

Browse:

- `http://127.0.0.1:18080/`
- `http://127.0.0.1:18080/projects/demo-app`
- `http://127.0.0.1:18080/projects/widget-shop`

## Default Sample Credentials

- `demo-app`: `demo-uploader` / `demo-secret`
- `widget-shop`: `widget-uploader` / `widget-secret`

The dashboards are public. Uploads require the per-project credentials above.
