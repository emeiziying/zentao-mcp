# 禅道 REST v1 兼容模式

本 fork 保留上游 v2 配置，新增适用于现有 REST v1 实例的只读配置。

## 启动

1. 编译：`go build -o zentao-mcp ./cmd/app`。
2. 复制 `config.v1.example.yaml`，将 `base_url` 改为自己的 `/zentao/api.php/v1` 地址；站点无子目录时去掉 `/zentao`。
3. 从仓库根目录运行 `./zentao-mcp -config config.v1.yaml`。
4. MCP 客户端连接 `http://127.0.0.1:9090/zentao/mcp`，在 `Token` 请求头中提供自己的禅道 Token。

Token 应由客户端安全配置提供，不要写入仓库。已有 REST v1 Token 可以直接复用；需要登录时由可信客户端调用 REST v1 `/tokens`。服务不收集用户密码，不提供网页登录/OAuth，不自动安装 glab 或其他客户端。

## 工具

| 工具 | 接口 | 说明 |
|---|---|---|
| zentao_current_user | GET /user | 当前用户，fields 可选 task、bug、task,bug，支持 page/limit |
| zentao_task_detail | GET /tasks/{id} | 任务详情 |
| zentao_products | GET /products | 产品列表，支持 page/limit |

保留原始响应结构，兼容嵌套任务分页，不将其强制转换为数组。此配置只注册以上 GET 接口，不包含编辑、删除、工时和状态修改。查询范围由禅道个人账号权限决定。

## 错误行为

上游失败状态现在标记 MCP isError。带 JSON 输出定义的接口若返回空白、HTML 或 null，即使 HTTP 200 也标记为失败；无 JSON 定义的接口和合法 204 响应保持原行为。GET/HEAD 工具带 readOnlyHint。客户端仍须检查业务错误字段，不应仅凭 HTTP 200 宣称业务成功。

## 验证

- `go test ./...`
- MCP initialize → notifications/initialized → tools/list → tools/call。
- 实例验收应至少包含当前用户、产品列表、任务详情，并确认无写工具。
- 不同禅道版本接口有差异，本文件只声明上述范围，不声称覆盖全部 REST v1 API。

此服务沿用上游请求头凭据转发，不等于完整多人 OAuth 服务。公开部署需要另行配置 TLS、入口身份认证和权限策略。

可重复验收脚本：在安全环境中提供 `ZENTAO_TOKEN`，运行 `python3 scripts/smoke-v1.py`。可选 `ZENTAO_MCP_URL` 指定 MCP 地址、`ZENTAO_TEST_TASK_ID` 指定当前账号可读任务。脚本只打印通过状态，不输出业务正文或凭据。
