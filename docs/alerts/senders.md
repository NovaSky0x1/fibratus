# Alert Senders

You can send alert notifications to your team through email, Slack, or incident response platforms. The notification can be sent to multiple alert senders. Alert senders configuration resides in the `alertsenders` section of the `yml` file.

- [Mail](/alerts/senders/mail)
- [Slack](/alerts/senders/slack)
- [Systray](/alerts/senders/systray)
- [Eventlog](/alerts/senders/eventlog)

In **Fleet mode**, the agent automatically enables the Fleet Server alert sender which forwards detection alerts via gRPC to the centralized dashboard. Detections appear on the [Detections page](/fleet/detections) with severity, process context, MITRE ATT&CK mapping, and process tree visualization. No configuration needed — the fleet alert sender is auto-enabled when enrollment is detected.

