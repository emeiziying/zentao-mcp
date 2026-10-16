# 禅道本人历史查询

两个工具沿用调用者 Token，只执行固定 GET 路径，每次检查权限，并在查询前后复核本人身份和可见范围。身份变化时不返回已读业务数据。调用参数不能指定账号、凭据、任意 URL 或当前日期。

- `zentao_my_task_history`: `year`、`mode`（默认 `year`，可选 `all`），以及 `execution_ids` 或显式 `scan_all_visible:true`。年度参与关系包含当前指派、本人完成/关闭，以及结构化历史指派和操作记录。账号精确匹配，不使用姓名或描述子串。
- `zentao_my_bug_history`: `year`（0 表示不限年份），以及 `product_id` 或显式 `scan_all_visible:true`。列表同时指定 `status=all` 和 `browseType=all`，覆盖已解决、关闭的 Bug；详情按 openedBy、assignedTo、resolvedBy、closedBy 及对应事件日期识别本人关系。

任务 `year` 模式先按列表日期筛选，可能遗漏仅历史动作发生在该年的任务，因此始终 `complete=false`。`all` 检查选定可见执行的全部候选详情；完整性仅限当前权限及服务保留的结构化历史，不能推断不可见或已删除的历史。Bug 当前关系字段无法覆盖每一次过去转交，输出明确这一限制。

逾期按服务器上海当天判断，截止日期早于今天且状态 wait/doing/pause 才计入当前逾期。完成晚于截止单独统计；缺完成日期时使用关闭日期并标记 `closedDate_proxy`，不声称它是真实完成时间。详见 [任务规则](task-history.md)。

缺失必要字段、姓名歧义、分页异常、重复数据、范围变化和预算耗尽不会被报告为完整。查询预算 50 秒，另留 3 秒身份复查；Bug 最多 20 产品、每产品 20 页、2000 个详情。结果需读取 complete、failures、limitations、truncated，不能把部分结果当作全年总量。

这些工具仅产生证据，不生成回复、不执行审核、不发送消息。Hermes 原有生成与独立审核双方通过的要求保持不变。本次验证使用隔离 HTTP fixtures；未部署或进行真实业务历史全量验收。
