# Troubleshooting

Common issues and their resolutions for the Fleet Server and agent.

## Agent Issues

### Agent Not Appearing in Dashboard

**Symptoms**: Agent enrolled successfully but doesn't show as online.

**Check**:
1. Verify the service is running: `sc query fibratus`
2. Check agent logs: `Get-Content "C:\Program Files\Fibratus\Logs\fibratus.log" -Tail 20`
3. Look for gRPC connection errors (502, 405, connection refused)
4. Verify enrollment data exists: check `HKLM\SOFTWARE\Fibratus\Enrollment` in registry

**Common causes**:
- Nginx gRPC proxy not configured (`location /fleet.` block missing)
- Server not running (`systemctl status fibratus-fleet`)
- Firewall blocking port 443
- Enrollment data corrupted — re-enroll with `fibratus enroll --token TOKEN --server URL`

### Agent Shows "dev" Version

The agent binary was built without version ldflags. Rebuild with:
```powershell
$env:VERSION="3.0.0"
.\make.bat build
```
The `make.bat` script embeds the version via `-ldflags "-X .../app.version=$VERSION"`.

### Rules Not Compiling (Skipped)

**Symptoms**: Agent log shows `skipping rule "...": rule needs engine version [3.0.0] but current version is [X]`

**Cause**: The agent's embedded version is lower than the rule's `min-engine-version`.

**Fix**: Rebuild the agent with the correct version (3.0.0+) using `make.bat build` with `$env:VERSION="3.0.0"`.

### "Enrollment failed at DPAPI step"

This is a **cosmetic warning**, not a real failure. The enrollment data was stored successfully in the DPAPI-encrypted registry. The warning comes from the optional ACL hardening step (`ProtectRegistryKeys`) which may fail if the registry key doesn't exist yet or the process lacks sufficient privileges. The enrollment itself is complete — the agent will connect on the next service start.

### High Memory Usage

With ~2,100 compiled rules, expect ~250-300 MB working set. If memory exceeds this:
- Check if rules compiled successfully (invalid rules waste memory during parsing)
- The memory optimization zeroes compile-only fields after compilation — ensure you're running the latest binary
- Baseline without rules is ~50-80 MB; the rest is rule ASTs, ETW buffers, and process snapshotter state

### Detections Not Firing

**Check the full chain**:

1. **Are events flowing?** Check server log: `tail /var/log/fibratus-fleet/fleet-server.log` — should show "buffer flushed X events"
2. **Are CreateProcess events present?** Query ClickHouse for event types in last 5 minutes — if CreateProcess is zero, the StackwalkDecorator may be deadlocked (update to latest binary with 2-phase flush fix)
3. **Are rules compiled?** Agent log should show "rules compiled from encrypted memory — N rules active"
4. **Are alerts sending?** Agent log should show "sending alert: [Rule Name]" when a rule matches
5. **Is the alert sender connected?** Check for gRPC errors in agent log

### MSI Silent Install Fails (Error 1603)

**Common causes**:
- Previous MSI product registered with different product code: uninstall first with `msiexec /x {PRODUCT_CODE} /qn`
- Windows Installer mutex held by another install: wait or kill `msiexec.exe` processes
- The current binary (v3.0.0+) uses no WixUI, so LaunchConditions issues are resolved

## Server Issues

### Telemetry Not Reaching ClickHouse

**Check**:
1. Verify ClickHouse is running: `systemctl status clickhouse-server`
2. Check server logs for ingestion errors
3. Verify the per-org table exists: `clickhouse-client --query "SHOW TABLES FROM fibratus"`
4. Check Nginx gRPC proxy has `client_max_body_size 100m` (413 errors mean the body size limit is too small)

### Dashboard Shows "Dashboard needs to be built"

The Go binary embeds the dashboard. Rebuild in correct order:
```bash
cd web/dashboard && npm run build
cd ../.. && go build -o /opt/fibratus-fleet/bin/fleet-server ./cmd/fleet-server
```
Dashboard must be built BEFORE the Go binary.

### gRPC 405 Method Not Allowed

Nginx is not proxying gRPC traffic. Ensure the Nginx config has:
```nginx
location /fleet. {
    grpc_pass grpc://127.0.0.1:8444;
    client_max_body_size 100m;
    grpc_read_timeout 300s;
    grpc_send_timeout 300s;
}
```
And that this config is in BOTH `sites-available` and `sites-enabled`.

### gRPC 413 Request Entity Too Large

The gRPC location block needs `client_max_body_size 100m`. The default Nginx limit (1 MB) is too small for telemetry batches.

## Rule Issues

### SIGMA Rules Generating Too Many False Positives

Use the **Tuning** tab on the Rules page to identify noisy rules. Disable them individually or by bulk selection. Known-noisy SIGMA rules are disabled by default during conversion.

### Rules Show as "Invalid"

The server validates rules using the Fibratus QL parser. Common validation failures:
- Missing macro definitions (macros must be synced alongside rules)
- Invalid field names (SIGMA-converted rules may reference fields that don't exist in Fibratus)
- Syntax errors in conditions (check for unmatched parentheses, invalid operators)

## Useful Commands

### Agent Logs
```powershell
Get-Content "C:\Program Files\Fibratus\Logs\fibratus.log" -Tail 50
```

### Server Logs
```bash
tail -f /var/log/fibratus-fleet/fleet-server.log
```

### Agent Service Status
```powershell
sc query fibratus
```

### ClickHouse Event Count
```bash
clickhouse-client --query "SELECT event_name, count() FROM fibratus.<table> WHERE timestamp > now() - INTERVAL 5 MINUTE GROUP BY event_name ORDER BY count() DESC"
```

### Detection Count
```bash
sudo -u postgres psql -d fibratus_fleet -c "SELECT count(*) FROM detections;"
```

### Restart Agent
```powershell
Get-Process fibratus -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 5
Start-Service fibratus
```
