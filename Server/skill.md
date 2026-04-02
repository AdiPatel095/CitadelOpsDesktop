---
name: citadel-game-api
description: Interact with the CitadelOps Game WebSockets through the local HTTP tool server (devtools). Use when you need to send commands to the game, read live packets, get game info, or analyze payloads.
---

# CitadelOps Game API

## Quick Start

This skill enables the agent to interact with the Goodgame Empire websocket through the `devtools` local HTTP bridge running on `http://localhost:8181`.

Whenever the user asks you to interact with the game, send a command, or read game state, you should use `curl` to talk to the local API.

### 1. Check Available Commands
To see the list of 3-letter SmartFoxServer/Game commands you can send or receive, along with their payload structures:
```bash
curl -s http://localhost:8181/commands
```
You can also look up the specific structure and notes for a single command to save context:
```bash
curl -s "http://localhost:8181/commands?cmd=gui"
```

### 2. Read Incoming Packets
To read the recent packet history from the game (returns up to the last 1000 messages):
```bash
curl -s http://localhost:8181/messages
```
*Note: The response is a JSON array of string arrays (each string array is a packet split by `%`).*

### 3. Send a Command
To just send a raw payload string (fire and forget) into the game websocket:
```bash
curl -X POST http://localhost:8181/send \
     -H "Content-Type: application/json" \
     -d '{"payload":"%xt%EmpireEx_21%pin%1%"}'
```
*(Replace the payload value with the actual formatted packet you wish to send).*

### 4. Query a Command (Synchronous Request/Response)
If you want to send a command and immediately get its corresponding response back (without having to fetch the whole message history), use the `/query` endpoint:
```bash
curl -X POST http://localhost:8181/query \
     -H "Content-Type: application/json" \
     -d '{"payload":"%xt%EmpireEx_21%gcu%1%{}%"}'
```
The server will automatically wait for the matching response packet (e.g. `gcu` in this case) and return it directly. You can also explicitly set the expected response type if it differs:
```bash
curl -X POST http://localhost:8181/query \
     -H "Content-Type: application/json" \
     -d '{"payload":"%xt%EmpireEx_21%sce%1%{}%", "expected_response":"irc"}'
```

**IMPORTANT**: If the command expects no specific data parameters, you still must append an empty JSON object `{}%` to the end of the packet, e.g., `%xt%EmpireEx_21%gcs%1%{}%` or `%xt%EmpireEx_21%gli%1%{}%`.

### 5. Clear Packet History
If you need a clean slate before performing an action to easily find the result manually:
```bash
curl -X POST http://localhost:8181/messages/clear
```

## Workflow Pattern for Game Interactions

When the user asks you to perform an action in the game and retrieve data:
1. Prefer using the `POST /query` endpoint. It sends your payload and safely waits to return the specific matching response packet.
2. **IMPORTANT**: If a `/query` request times out or returns no response, **do not retry**. Try only once. A timeout or empty response simply means that the game server does not send a response packet for that specific command.
3. Only use `POST /send` + `GET /messages` if you need to perform an action where the response type is unknown or you need to analyze the whole sequence of returned packets.
