# teamsctl 🤖

Have you ever thougth to yourself... 🤔

> "Geez, wouldn't it be nice, if it was easier to send AI slop and responses to my coworker.."

or

> "Holy hell, I fucking hate talking to Mike... Fucking Mike, with his cool stories and short arms - I wish I could make him speak to Claude or GPT or some chinese model."

or

> "It's a super duper great idea to have an LLM use my name in Teams"

## WELL DO I HAVE THE SOLUTION FOR YOU! 🫵

Now with **40%** more slop, and **20x** more bullshit and a **100% vibe coded codebase** from top to fucking bottom - **Now you can**!

Introducing the **teamsctl** (_teams cuddle_ because it sounds cute).

This allows you the ability to use teams from the commandline.. **BUT WAIT THERE IS MORE**

For a limited time only, now you can use our new **MCP SERVER**! 🔌

Huh! Wadda you say!? Does **that** not sound like a great fucking idea?

For only **4.99$** you'll get a lifetime subscription to a vibe coded Go mess, with a shitty and hacky authenication method!

So if you want to outsource talking to Mike or any other colleagues for that matter, **this** is the solution for you!

---

**I HAVE PERSONALLY NOT REVIEWED A SINGLE FUCKING THING 💪**

> Me: "GPT5.6 Make this"

> GPT5.6: "_BEEP_ _BOOP_ OKAY" _Proceeds to puke out 800 lines of Go code to a single file_

See **that's** how the modern day man does it! 🚀

## Requirements

- A Microsoft Teams account.
- Google Chrome or another Chromium-based browser for authentication. Use `-chrome` or `CHROME_PATH` for Chromium, Helium, or a non-standard installation; macOS `.app` paths are accepted.
- Go 1.18 or newer only when building from source. Homebrew and release installs use a prebuilt or Homebrew-managed binary.


## Install

With Homebrew:

```sh
brew install TheSinding/tap/teamsctl
```

Or install the latest release directly:

```sh
wget -qO- https://raw.githubusercontent.com/TheSinding/teamsctl/main/scripts/install.sh | sh
```

On macOS without `wget`, use:

```sh
curl -fsSL https://raw.githubusercontent.com/TheSinding/teamsctl/main/scripts/install.sh | sh
```

This installs the binary to `${HOME}/.local/bin/teamsctl`. From a Git checkout,
build and install the current source instead:

```bash
make install
sudo make install PREFIX=/usr/local
```

Other targets: `make build`, `make test`, `make uninstall`.

Authenticate once before using the CLI or MCP server:

```bash
teamsctl auth
```

Authentication opens Chrome with a persistent profile and stores tokens in
your OS keyring (macOS Keychain, Windows Credential Manager, or the Linux
Secret Service) when available. If no keyring backend is available, tokens
fall back to plain files under `~/.config/teamsctl` (or
`$XDG_CONFIG_HOME/teamsctl` if set), created with `0600` permissions. Chrome
is required; Node and Electron are not.

Optional autofill:

```bash
TEAMS_EMAIL="$EMAIL" TEAMS_PASSWORD="$PASSWORD" TEAMS_OTP="$OTP" teamsctl auth
```

Use `CHROME_PATH` when Chrome is not installed at its platform-default path.

## CLI

| Command | Behavior |
|---|---|
| `teamsctl auth [-email ADDRESS] [-chrome PATH] [-timeout 5m]` | Refresh all required Teams tokens. |
| `teamsctl conversations` | Print every chat and channel as JSON, including usable conversation IDs. |
| `teamsctl messages [-limit 50] [-name TITLE] ID` | Print the newest messages in chronological order. `-limit 0` returns all fetched messages. |
| `teamsctl send [-format text\|html] [-mention NAME] ID [MESSAGE...]` | Send a message. Reads stdin when `MESSAGE` is omitted. Repeat `-mention` for multiple people. |
| `teamsctl mcp` | Run the MCP server over stdio. |
| `teamsctl version` | Print the version embedded from the Git tag at build time. |

Chat records can contain fallback IDs in `ids`. Commands accept comma-separated
candidate IDs and try them in order.

Examples:

```bash
teamsctl conversations | jq '.[] | select(.kind == "chat")'
teamsctl messages -limit 10 '19:conversation-id@thread.v2'
printf 'hello world' | teamsctl send '19:conversation-id@thread.v2'
teamsctl send -format html '19:conversation-id@thread.v2' '<strong>Hello</strong>'
teamsctl send -mention Mikkel '19:conversation-id@thread.v2' 'Hello @Mikkel'
```

## MCP

The MCP server checks token expiry during initialization and before every tool
call. Run `teamsctl auth` when connection fails with an authentication error.

| Tool | Purpose |
|---|---|
| `list_conversations` | Find chats/channels by `query`, `kind`, and `limit`. Results are cached for five minutes. |
| `get_latest_message` | Resolve the best matching one-to-one chat and return its latest message. |
| `get_messages` | Read messages using a conversation ID. |
| `send_message` | Send plain text or HTML, with optional real Teams mentions. |

Use `format: "html"` for formatted or multi-part messages. HTML such as
`<strong>@Mikkel</strong>` is only bold text: a real mention also requires
`"mentions": ["Mikkel"]`. Unlisted `@` text remains plain text.

Advanced consumers can bypass name/email lookup with `mention_entities`, using
`display_name` plus `mri` or `object_id`.

## Add To An Agent

These examples assume `teamsctl` is available on your `PATH` (for example,
when installed via Homebrew). If not, replace `teamsctl` with the full path to
your binary (for example, `PATH/TO/teamsctl`). Restart the agent harness after
changing its configuration.

<details>
<summary>OpenCode</summary>

Add to `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "teams": {
      "type": "local",
      "command": ["teamsctl", "mcp"],
      "enabled": true
    }
  }
}
```

</details>

<details>
<summary>Claude Code</summary>

```bash
claude mcp add --scope user teams -- teamsctl mcp
```

Verify with `claude mcp list`.

</details>

<details>
<summary>Codex</summary>

```bash
codex mcp add teams -- teamsctl mcp
```

Equivalent `~/.codex/config.toml`:

```toml
[mcp_servers.teams]
command = "/bin/sh"
args = ["-c", "exec teamsctl mcp"]
```

</details>

<details>
<summary>Pi</summary>

Install the adapter:

```bash
pi install npm:pi-mcp-adapter
```

Add to `~/.config/mcp/mcp.json`:

```json
{
  "mcpServers": {
    "teams": {
      "command": "/bin/sh",
      "args": ["-c", "exec teamsctl mcp"],
      "lifecycle": "lazy",
      "directTools": true
    }
  }
}
```

</details>

## Development

```bash
make test
make build
```
