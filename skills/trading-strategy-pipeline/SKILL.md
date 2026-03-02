---
name: trading-strategy-pipeline
description: 使用技能化流水线生成可上线交易策略包。适用于用户在“策略生成”板块输入交易习惯后，按 spec-builder -> strategy-draft -> optimizer -> risk-reviewer -> release-packager 产出结构化策略与硬边界。
---

# Trading Strategy Pipeline

## 何时使用

- 用户需要根据交易习惯（habit）生成可执行策略。
- 需要把 AI 输出约束为结构化 JSON，并绑定风控硬边界。
- 需要输出可回测、可审核、可上线的策略包。

## 固定流程

1. `spec-builder`
- 输入：`symbol`、`habit`、实盘执行参数（仓位模式、高低信心、杠杆）。
- 输出：硬边界（最大回撤、单笔风险、杠杆上限、是否允许加仓）。

2. `strategy-draft`
- 输入：市场快照 + spec 约束。
- 输出：仅结构化 DSL/JSON（市场状态、关键位、入场、出场、HOLD 条件）。

3. `optimizer`
- 输入：回测结果。
- 限制：只允许调参数或有限改规则，不允许改硬边界。

4. `risk-reviewer`
- 输入：策略草案 + 回测结果。
- 输出：过拟合风险、脆弱点、极端行情暴露，给出 pass/fail。

5. `release-packager`
- 输出：`strategy_version`、变更摘要、监控项、回滚条件、shadow 计划。

## 强约束

- 任一阶段失败，统一 `HOLD`，不得下单。
- 风控引擎拥有仓位与杠杆最终决定权。
- 仅输出严格 JSON（带 `version`），解析失败视为失败。
- 每阶段都要记录输入、输出、耗时、模型、token、执行结果。

## 参考

- 习惯画像：`references/habit-profiles.json`
- 策略包结构：`references/strategy-package-schema.json`
