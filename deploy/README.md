# Deployment

Target: small always-on Linux VM (Ubuntu 22.04+ / Debian 12+).

## One-time setup

1. Install binary:

   ```
   sudo install -m 0755 bin/mr /usr/local/bin/mr
   ```

2. Create system user and data dir:

   ```
   sudo useradd --system --home /var/lib/mr --shell /usr/sbin/nologin mr
   sudo install -d -o mr -g mr -m 0750 /var/lib/mr
   ```

3. Create env file (holds API keys):

   ```
   sudo install -d -o mr -g mr -m 0750 /etc/mr
   sudo tee /etc/mr/env > /dev/null <<'EOF'
   REDDIT_CLIENT_ID=...
   REDDIT_CLIENT_SECRET=...
   REDDIT_USER_AGENT=market-research/0.1 (by u/yourname)
   STACKEXCHANGE_KEY=...
   ANTHROPIC_API_KEY=...
   MR_DB_PATH=/var/lib/mr/mr.db
   EOF
   sudo chown mr:mr /etc/mr/env
   sudo chmod 0600 /etc/mr/env
   ```

4. Install systemd units:

   ```
   sudo cp deploy/mr-fetch.service deploy/mr-fetch.timer \
           deploy/mr-rediscover.service deploy/mr-rediscover.timer \
           /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now mr-fetch.timer mr-rediscover.timer
   ```

5. Add first topic:

   ```
   sudo -u mr /usr/local/bin/mr topic add "soc2 compliance tool" --description "SOC2 audit pain"
   ```

## Observability

- Logs: `journalctl -u mr-fetch -o json` / `journalctl -u mr-rediscover -o json`
- Timer status: `systemctl list-timers mr-*`
- Self-diagnostic: `sudo -u mr /usr/local/bin/mr doctor`
