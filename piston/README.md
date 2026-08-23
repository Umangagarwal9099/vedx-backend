# Self-hosted Piston (code execution)

Replaces Glot.io, whose public API was fully removed in their site relaunch
(confirmed by testing every documented endpoint — all 404). Code execution
for the student portal's Coding Practice feature now runs through this.

## Deploy

On whatever server this runs on (needs Docker + Docker Compose, and cgroup v2
enabled with cgroup v1 disabled — true by default on any reasonably recent
Linux host):

```sh
cd piston
docker compose up -d
```

This starts the API on port 2000 with **no language runtimes installed yet**
— confirmed locally that `/api/v2/execute` and `/api/v2/runtimes` both work
correctly right away (execute cleanly returns a 400 "runtime is unknown" for
a language that isn't installed yet, rather than erroring or crashing).

## Install the languages we need

First, list what's installable and get the exact package names/versions —
don't guess these, they don't always match the language names `/execute`
uses (e.g. the JavaScript runtime's package is named `node`, not
`javascript`):

```sh
curl http://localhost:2000/api/v2/packages
```

Then install each one the platform supports — Python, JavaScript, TypeScript,
Java, C, C++, Go, Rust — by POSTing the exact `language`/`language_version`
pair from that list for each:

```sh
curl -X POST http://localhost:2000/api/v2/packages \
  -H "Content-Type: application/json" \
  -d '{"language": "python", "version": "<version from the list above>"}'
```

Repeat for each language. Re-run `GET /api/v2/packages` afterward and check
`"installed": true` on each to confirm.

If an install request hangs or the container needs restarting — during local
testing on one machine, installs intermittently failed to reach one of
GitHub's release-asset CDN IPs specifically (DNS-based requests to the same
hostname worked fine every time via curl directly; only that one pinned IP
baked into the signed download URL Piston received didn't route from that
particular Docker network path). If you hit the same thing, just retry the
install request — a fresh request gets a fresh signed URL and likely a
different IP. This never affected `/execute` or `/runtimes` — only the
one-time package install step — and is very likely specific to that one
machine's network path rather than something that'll recur here.

Once installed, verify actual execution works:

```sh
curl -X POST http://localhost:2000/api/v2/execute \
  -H "Content-Type: application/json" \
  -d '{"language":"python","version":"*","files":[{"content":"print(1+1)"}]}'
```

Expect `{"language":"python","version":"...","run":{"stdout":"2\n",...}}`.

## Point the student app at it

In `vedxlence-lms-student`'s environment (Vercel project settings), set:

```
PISTON_API_URL=http://<this-server's-address>:2000
```

(Not `NEXT_PUBLIC_` — this is only ever called server-side, from
`src/app/api/execute/route.ts`, so it shouldn't be exposed to the browser
bundle.)

If this runs on the same host as `vedx-backend`, prefer the internal/private
address over the public one where possible — this port has no auth in front
of it, since Piston's own API doesn't include one. Restricting inbound
access on 2000 to just the student app's servers (firewall rule / security
group / Docker network) is worth doing before this is live.
