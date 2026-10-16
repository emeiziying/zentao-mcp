# Authenticated user's monthly task worklogs

`zentao_my_monthly_worklogs` accepts only `year` and `month`. It supports the current year in `Asia/Shanghai`. Past months cover every day, the current month covers day one through today, and future months or other years are rejected. There is no fallback to a full historical task scan.

## Read and identity boundary

The tool first authenticates `/user` using the caller's existing ZenTao token and records the account, numeric user ID and real name. A server-owned daily-report endpoint is then queried with the authenticated real name (or an explicit server account-to-name mapping). Four bounded workers read at most 31 days using `data=yesterdayEff`, `name` and `ex=MM-dd`. The report client is independent: it has no ZenTao Token, Authorization header or cookies and does not follow redirects. Chat input cannot choose the URL, name, account or task IDs.

The report discovers task IDs only. Its `id` is a **task ID**, not a worklog ID, and is never emitted as a log ID. The tool reads each discovered task's `GET /tasks/{id}/estimate` with the original user's token, validates ownership and dates, excludes deleted rows, and returns the original worklog IDs and text. Task status and current assignment do not filter discovery, so completed and transferred tasks can be included. Every concrete authenticated GET still passes the configured operation policy.

For each date/task pair, the report's row count and exact decimal amount multiset must match the canonical own-account logs. This detects same-name mixing, omissions within discovered tasks and equal-total-but-different-row discrepancies. Other users' canonical logs are never returned. A final `/user` check must match account, user ID and real name before any business result is returned. Identity-check failure fails closed.

## Configuration and result

The tool is disabled unless its server explicitly configures `monthly_report.url`:

```yaml
servers:
  - name: zentao
    # Existing schema_url, base_url, allow and block settings remain required.
    monthly_report:
      url: http://report-service:8080/dataViewData
      account_names:
        example-account: Example Name
```

`account_names` is optional. Without a mapping, the authenticated profile's real name is used. A mapping never changes the token account used for canonical log verification. Configuration rejects URLs with embedded credentials, queries, fragments or non-HTTP schemes. The tool also requires `/user` and `/tasks/{id}/estimate` GET definitions in the loaded schema and permission to use them.

The output contains `account`, `user_id`, `realname`, `year`, `month`, `timezone`, `entries`, `count`, exact-decimal `total_hours`, `complete`, `coverage` and `limitations`. Canonical entries contain `id`, `task_id`, `task_name`, `account`, `date`, `consumed` and `work`.

`coverage.scope` is `daily_report_verified_tasks`, with `from_date`, `to_date`, `days_requested`, `days_scanned`, `failed_days`, `tasks`, `tasks_scanned`, `failures` and `truncated`. Completeness means all requested days were read and every discovered task was independently verified and reconciled. It is **not** proof that the reporting service includes every effort module or inaccessible/deleted task. In particular, a task omitted entirely by that service cannot be detected through this discovery route. The current month's result is explicitly month-to-date.

A failed day or task, malformed canonical record, repeated page, inconsistent total, reconciliation mismatch or deadline produces `complete=false`. Canonical logs on a failed report day may still be returned when independently verified from a task discovered on another day. The result stays partial. A 50-second overall query budget plus a three-second identity recheck replaces the abandoned long-running full-scan approach. No caches, persisted credentials or business writes are added.

The official [REST v1 task worklog endpoint](https://www.zentao.net/book/api/1199.html) defines an unpaginated `effort` collection. Its documented ID-keyed object and deployed array forms are accepted. If the upstream provides `total`, pagination is validated rather than silently truncating.

## Maintenance and verification

The read-only aggregate implementation is synchronized with the maintained downstream copy. Repository copies do not synchronize automatically. This PR changes source code only; it does not deploy or reconfigure shared services.

`go test ./...` uses isolated fake upstreams. Tests cover completed/transferred candidates without task enumeration, canonical original IDs, another user's logs, deleted logs, month boundaries, Shanghai date boundaries, unsupported years, future months, exact decimal sums and multiset comparison, malformed reports/logs, failed days, name collisions, permission blocks, identity changes, default-off registration, wrong schemas and credentials absent from report requests. Real read-only replay is a separate acceptance step and must not send group messages.
