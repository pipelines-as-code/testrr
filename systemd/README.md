# systemd + Podman deployment

## Install

```sh
# Create the data directory
sudo mkdir -p /var/lib/testrr/data
sudo chown 1000:1000 /var/lib/testrr/data

# Copy and enable the unit
sudo cp testrr.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now testrr
```

## Configure

Override defaults without editing the unit file:

```sh
sudo systemctl edit testrr
```

```ini
[Service]
Environment=TESTRR_IMAGE=ghcr.io/your-org/testrr:v1.2.3
Environment=TESTRR_ADDR=:9090
Environment=TESTRR_DATABASE_URL=postgres://user:pass@db/testrr
```

## First-time project setup

```sh
sudo podman exec -i testrr testrr project create \
  --slug my-project \
  --name "My Project" \
  --username ci \
  --password-stdin <<< "secret"
```

## Logs

```sh
journalctl -u testrr -f
```
