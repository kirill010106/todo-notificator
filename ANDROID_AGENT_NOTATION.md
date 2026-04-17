# Android AI Agent Notation (Java + XML)

## Purpose
This instruction is for an AI agent that develops an Android client for ToDo Notificator using Java and XML, strictly following the API contract.

## Input Assumptions
- The agent has NO access to project source code.
- The agent receives only the OpenAPI file from the user.
- The OpenAPI contract is the single source of truth for endpoints, payloads, response bodies, status codes, and field names.

## Production Backend
- Use production backend address only: https://todoapi.chkpnk.ru/api/v1
- For Retrofit base URL use trailing slash form: https://todoapi.chkpnk.ru/api/v1/
- Do not use localhost or dev URLs.

## Hard Rules
1. Do not invent endpoints, fields, enums, or status codes.
2. Do not rename JSON fields from contract names.
3. Treat nullable fields as nullable in client models.
4. Respect 204 responses as empty-body responses.
5. If a request returns 401, use refresh flow exactly once and retry original request once.
6. If refresh fails, clear auth session and force login screen.
7. Do not break app flow on unknown JSON fields.
8. Always parse and serialize date-time as ISO-8601 with timezone support.

## Android Stack Constraints
- Language: Java
- UI: XML Views (no Compose)
- Architecture: MVVM + Repository
- Networking: Retrofit + OkHttp
- JSON: Gson or Moshi
- Local secure storage: EncryptedSharedPreferences for tokens
- Optional cache/offline: Room

## Network Layer Contract
- Add Authorization: Bearer <access_token> for protected endpoints.
- Keep public endpoints without Bearer token.
- Implement a single refresh coordinator to avoid multiple simultaneous refresh requests.
- Add request timeout and error mapping for network, timeout, and server validation errors.

## Auth and Session Behavior
- Login stores access_token, refresh_token, expires_in.
- Protected call with 401:
  - if request not retried yet -> call /refresh with refresh_token
  - if refresh success -> update tokens -> retry original request once
  - if refresh failed -> logout locally and navigate to auth
- Logout endpoint call is best effort; local token cleanup must happen even if network fails.

## API Compliance Workflow
For each endpoint from OpenAPI:
1. Create Retrofit method.
2. Create request DTO and response DTO exactly by schema.
3. Create mapper DTO -> Domain model.
4. Add repository method.
5. Add ViewModel action and UI state updates.
6. Add unit test and API test (MockWebServer) for success and key errors.

## Error Handling Contract
- Parse standard error envelope if defined in OpenAPI (status and error fields).
- Display user-facing messages from server error when available.
- Distinguish validation errors (400), unauthorized (401), not found (404), conflict (409), and server errors (500+).

## Feature Modules Required
1. Authentication
- Register
- Login
- Refresh flow
- Verify email (token link handling)
- Resend verification
- Logout

2. Tasks
- List with pagination (limit, offset)
- Create
- Update (partial patch)
- Delete
- Reminder and deadline editing

3. Categories
- List
- Create
- Update
- Delete

4. Stats
- Get user stats
- Patch stats if contract provides this endpoint

5. Pomodoro
- Start session
- Pause session
- Stop session with contract enum action values only

## UI Requirements
- Java + XML screens/fragments only.
- Loading, error, and empty states for each list screen.
- Optimistic UI only when server contract guarantees safe rollback behavior; otherwise use server-confirmed updates.
- Consistent date/time pickers for deadline and reminder fields.

## QA and Testing
Minimum test set per feature:
1. Success response parsing.
2. 400/401/404/409 error mapping.
3. 204 empty-body handling.
4. 401 -> refresh -> retry success.
5. 401 -> refresh fail -> logout path.
6. Mapper tests for nullable fields and enum values.

## Delivery Format From Agent
Agent must provide:
1. Generated API interface and DTOs.
2. Repository and ViewModel implementations.
3. XML layouts and UI wiring.
4. Token interceptor and authenticator logic.
5. Test coverage summary by endpoint.
6. Explicit list of assumptions (if any).

## Definition of Done
- All implemented endpoints match OpenAPI schema and status codes.
- No contract field mismatches in request/response JSON.
- Auth flow with refresh is stable and race-safe.
- App handles errors gracefully and remains usable offline for cached reads if Room is enabled.
- Build passes and tests pass.
