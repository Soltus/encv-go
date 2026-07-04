# OpenList API Contract (agent perspective)

This document specifies the eight OpenList HTTP endpoints the
in-process Go agent (`/workspace/agent`) consumes. The
contract is intentionally narrow: only the request shape, the
success response, and the canonical error codes the agent
inspects.

All endpoints share these properties:

- **Base URL**: configurable via `agent_settings.openlist_base_url`
  in `~/.encv/config.user.json`. Default: `http://127.0.0.1:5244`.
- **Auth header**: `Authorization: <token>` (raw token, **not**
  `Bearer <token>` — OpenList convention).
- **Content-Type**: `application/json` for requests with bodies.
- **Response shape**: `{ "code": <int>, "message": "<str>",
  "data": <any> }`. `code == 200` is the only success
  indicator; the agent does not parse `message`.

## 1. `list_files`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/admin/fs/list` |
| **Tool name** | `list_files` |
| **NeedConfirm** | true |
| **Kind** | read-only |

**Request body**

```json
{
  "path": "/some/dir",
  "page": 1,
  "per_page": 1000
}
```

**Response**

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "content": [
      {
        "name": "a.txt",
        "size": 1234,
        "is_dir": false,
        "modified": "2025-01-01T12:00:00Z"
      }
    ],
    "total": 1
  }
}
```

**Error codes** the agent surfaces to the LLM:

| Code | Cause | Agent's tool_result |
|------|-------|---------------------|
| 401  | invalid token | `openlist_error: HTTP 401` |
| 404  | path not found | `openlist_error: HTTP 404` |
| 5xx  | server error | `openlist_error: HTTP 5xx` |

## 2. `read_file`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/admin/fs/get` |
| **Tool name** | `read_file` |
| **NeedConfirm** | true (large files) |
| **Kind** | read-only |

**Request body**

```json
{ "path": "/a.txt" }
```

**Response**

```json
{
  "code": 200,
  "message": "ok",
  "data": "<file contents as a string>"
}
```

## 3. `write_file`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/admin/fs/put` |
| **Tool name** | `write_file` |
| **NeedConfirm** | true (overwrites) |
| **Kind** | file-change |

**Request body**

```json
{
  "path": "/a.txt",
  "data": "<new contents>",
  "flag": "overwrite"
}
```

**Response**: `{ "code": 200, "message": "ok" }`

## 4. `delete_file`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/fs/remove` |
| **Tool name** | `delete_file` |
| **NeedConfirm** | true |
| **Kind** | file-change |

**Request body**

```json
{ "path": "/a.txt" }
```

**Response**: `{ "code": 200, "message": "ok" }`

## 5. `rename`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/fs/rename` |
| **Tool name** | `rename` |
| **NeedConfirm** | true |
| **Kind** | file-change |

**Request body**

```json
{ "src": "/a.txt", "dst": "/sub/b.txt" }
```

**Response**: `{ "code": 200, "message": "ok" }`

## 6. `exec_command`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/admin/command` |
| **Tool name** | `exec_command` |
| **NeedConfirm** | true |
| **Kind** | command |

**Request body**

```json
{ "command": "ls -la /" }
```

**Response**

```json
{
  "code": 200,
  "message": "ok",
  "data": "<captured stdout>"
}
```

> ⚠️ This endpoint is admin-only on most OpenList builds.
> Front-ends should disable the tool for non-admin tokens.

## 7. `get_storage_info`

| | |
|---|---|
| **HTTP method** | GET |
| **Path** | `/api/admin/storage` |
| **Tool name** | `get_storage_info` |
| **NeedConfirm** | false |
| **Kind** | read-only |

**Request body**: (empty)

**Response**

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "total": 1000000000,
    "used": 500000000,
    "free": 500000000
  }
}
```

## 8. `search_files`

| | |
|---|---|
| **HTTP method** | POST |
| **Path** | `/api/admin/fs/search` |
| **Tool name** | `search_files` |
| **NeedConfirm** | true |
| **Kind** | read-only |

**Request body**

```json
{ "parent": "/some/dir", "keywords": "report" }
```

**Response**

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "content": [
      {
        "name": "report-2025-01.pdf",
        "size": 1234,
        "is_dir": false,
        "modified": "2025-01-15T08:00:00Z"
      }
    ]
  }
}
```

## Auth & Errors (summary)

- **Header**: `Authorization: <token>` (no `Bearer` prefix).
- **Success**: `code == 200`. The agent does NOT inspect
  `message`.
- **Failure**: any other `code` value, or an HTTP transport
  error. The agent wraps these in a typed `*OpenListError` and
  surfaces the message in the tool_result JSON.

## Adding a new OpenList endpoint

1. Add the typed method to `openlist_client.go` in the
   `/workspace/agent` package. Keep the method small and
   focused on a single endpoint.
2. Add the corresponding `openai.FunctionDefinition` schema
   to `cmd/agent-demo/schemas.go`.
3. Register the tool in `cmd/agent-demo/main.go` →
   `registerOpenListTools`. Set `NeedConfirm` per the
   side-effect class:
   - **read-only / safe**: false
   - **mutates user data** (write/delete/rename): true
   - **destructive on system state** (exec_command): true
4. Add a fake response in `openlist_client_test.go` and an
   end-to-end test against the OpenList tool registration.
5. Update this document with the new endpoint.
