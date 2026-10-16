# 禅道 REST v1 兼容模式

本 fork 为 REST v1 提供独立接口定义，保留上游 v2 模式。v1 文档覆盖、参数来源和疑点修正见 [覆盖清单](v1-coverage.md)；覆盖不代表目标实例拥有所有商业模块或当前账号有全部权限。

## 配置

1. `go build -o bin/zentao-mcp ./cmd/app`。
2. 复制配置并填写实例的 `/zentao/api.php/v1` 地址（站点无子目录则去掉 `/zentao`）。
3. 从仓库根目录执行 `./bin/zentao-mcp -config config.v1.yaml`。
4. MCP 地址为 `http://127.0.0.1:9090/zentao/mcp`，每个请求通过 `Token` 头传入调用者自己的凭据。

| 配置 | 能力 |
|---|---|
| config.v1.example.yaml | 全量 GET 查询 |
| config.v1.full.example.yaml | 查询、创建、修改、删除及状态操作；禁止 /tokens 工具 |

`payload` 是 JSON 请求体；必填字段在工具 schema 中声明，PUT 只传希望修改的字段。分页按接口 page/limit 参数逐页查询，保留服务端结构和总数，不默认只取第一页后声称结果完整。数组按文档放在 payload 中，不拼接成字符串。不同版本任务日志参数由接口定义给出兼容分支。

## 登录与凭据

`python3 scripts/login-v1.py --url https://zentao.example.com/zentao/api.php/v1`

由用户在可信本机终端输入账号和隐藏密码。登录脚本调用 POST /tokens，再用 GET /user 验证，只保存 Token 到用户配置，权限为 0600，不保存密码。已有配置覆盖前确认；失败保留旧配置。可用 `--config` 指定路径。

服务本身不读此配置：客户端从用户自己的安全配置读取 Token，放入 MCP `Token` 请求头。Token 过期后重新登录；不自动重放业务写请求，不因认证失败切换为共享账号。

登录端点存在于完整 OpenAPI 文档，但示例运行配置禁止将密码登录暴露为模型工具；模型不应接收或输出密码。已有 Token 可直接使用，无需再次登录。

## 错误与权限

- HTTP 非 2xx 标记 isError；JSON 接口返回空、HTML、null 也标记失败，合法 204 保留。
- JSON 的 error 非空、status/result 为 fail/failed/error、result=false 标记业务失败。正常 code 字段不被当作 HTTP 状态码。
- GET/HEAD 标记 readOnlyHint。标注仅供客户端决策，服务端能力由 allow/block 和禅道权限控制。
- 不跟随上游重定向，避免凭据被转发或写请求重放；默认请求超时 30 秒。
- 日志递归隐藏 payload 内密码和 Token；凭据不存进仓库。
- 请求可能已经写入但响应丢失时，先查询实际状态再决定处理，不盲目重试。

## 验证

- `go test -race ./...`：隔离请求、方法、参数、凭据、错误与日志测试。
- `python3 scripts/test-login-v1.py`：本机假服务验证登录和安全保存。
- `python3 scripts/test-v1-contract.py /absolute/path/to/zentao-mcp`：用假上游逐一调用所有业务工具，不连接真实禅道。
- `ZENTAO_TOKEN` 由安全环境注入后，`python3 scripts/smoke-v1.py` 验证真实查询。可选 `ZENTAO_MCP_URL`、`ZENTAO_TEST_TASK_ID`。

完整契约测试不等于真实实例所有接口验收。生产实测仅覆盖只读查询；真实任务修改、删除、用户管理、反馈/工单需有对应权限、版本和独立测试数据再验收。

本服务沿用每请求凭据转发，不提供完整多人网页登录/OAuth；远程部署还需 TLS 和入口身份认证。

## 本次验收结果

- 官方手册 97 个接口定义：38 GET、33 POST、13 PUT、13 DELETE；其中认证独立处理，完整 MCP 配置注册 96 个业务工具。
- 全量 Go 竞态测试、登录假服务测试、96 工具逐一 MCP 契约调用通过。
- 真实只读验证：当前用户及任务、任务详情、产品、部门、项目集、项目、测试单通过。
- 用户列表返回 400（no company-browse priv），反馈与工单返回 403（Access not allowed）。这些是当前账号/实例的访问限制，适配器正确标记失败，不绕过权限。
- 未对真实实例执行任何业务写入。写接口仅完成文档、schema及隔离传输验证；上线前仍需在对应版本测试环境验收真实业务规则。
