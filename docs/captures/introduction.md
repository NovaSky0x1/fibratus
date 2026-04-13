# Immortalizing The Event Flux

Captures or `kcap` files aim for the capture-once replay-anywhere workflow. Captures contain the full state of processes at the time capture was taken as well as the originated event flux. This makes them a great companion in post-mortem investigations - generate the capture in the honeypot machine, grab the `.kcap` file, and you're ready to dive into the attacker kill chain by replaying the capture file on your laptop.

With captures you "freeze" the shape of the event flux at a certain point in time. Do you need to troubleshoot an network issue and surface the root cause? Or maybe you need to determine what files were written by a malicious process? Replay the capture at any given time and drill down into the event flow to start investigating.

You can harness the power of the filtering engine when replaying captures or even execute a filament on top of captured events.

## Remote Captures via Fleet Server

In [Fleet mode](/fleet/overview), you can initiate live kernel captures on remote endpoints directly from the dashboard. The Agent Detail page provides a **Captures** tab where you can:

- Start a capture with custom duration, event type filters, or Fibratus QL expressions
- View events in real time with terminal-style color-coded output
- Export captured events as JSON, CSV, or native `.kcap` format
- Download and replay captures locally with `fibratus replay`

See [Fleet Live Captures](/fleet/captures) for details.
